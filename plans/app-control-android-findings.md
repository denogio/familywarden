# App control — Android capability findings (P2)

Round 1: 2026-10-21, Android 17 emulator with Google Play (`android-37.0`,
`google_apis_playstore`), FamilyWarden **debug** build as Device Owner via adb. Probes:
`android/app/src/debug/.../DebugProbeReceiver.kt` (debug only, never in release). The
son's phone was not used; the user's Pixel 10 was not touched. Emulator results are
evidence for AOSP/Android behaviour, not for OEM, battery or real-hardware behaviour.

## Usage data (UsageStatsManager)

| Question | Result | Consequence |
|---|---|---|
| Access without grant | `queryEvents` returns **0 events, no error** (app-op mode 3/default) | The app must check `OPSTR_GET_USAGE_STATS` itself and report "usage access missing", never "no usage" |
| Grant via adb | `appops set <pkg> GET_USAGE_STATS allow` works immediately | Setup already uses adb, so no user tap is needed |
| Persistence across reboot | Package-mode grant was **lost** after a quick reboot (also after `write-settings`), kept after 90 s; **UID-mode** `appops set --uid <pkg> GET_USAGE_STATS allow` survived quick reboots twice | Setup uses the UID-mode command; the app still re-checks access at boot and reports loss |
| Device Owner self-grant | No documented API (docs: the user grants usage access in Settings) | Not relied on |
| Before first unlock | Documented: `queryEvents` returns `null` while the user is locked | Count nothing then; resume after unlock |
| Event retention | Documented: "only kept by the system for a few days" | The phone keeps its own running totals; never re-derives a day long after the fact |
| Screen off | `SCREEN_NON_INTERACTIVE` (16) arrives ~0.7 s **before** the app's `ACTIVITY_PAUSED`/`STOPPED` | Cut counting at type 16, not at the pause |
| Screen on | `SCREEN_INTERACTIVE` (15) then the app's `ACTIVITY_RESUMED` | Resume counting at whichever is later |
| Shutdown | Documented: `DEVICE_SHUTDOWN` closes all open activities without stop events | Close open intervals at shutdown |
| Picture-in-picture | Could not force Chrome into PiP via adb on the emulator | Proposed: count `RESUMED` → `STOPPED`, cut by screen-off. Whether this accurately tracks PiP/split-screen visibility remains unverified; test with YouTube on the Pixel 10 |
| Background audio, screen off | Emulator has no audio device; playback could not be confirmed | By the screen-off cut it cannot count (D5); verify with Spotify on the Pixel 10 |

## App suspension (DevicePolicyManager)

| Question | Result | Consequence |
|---|---|---|
| Suspend an ordinary app | `setPackagesSuspended` works; launching shows the system dialog | Core mechanism confirmed |
| Dialog text | Title "Blocked by work policy" is fixed; body is replaced by `setShortSupportMessage` ("Paused by FamilyWarden. Ask a parent for more time in the FamilyWarden app.") | Set a FamilyWarden support message (EN/DA by locale); title stays Android's |
| Not suspendable | FamilyWarden itself, **Play Store**, default dialer, active launcher, Settings — returned in the failed list | Never rely on suspending them; the failed list must be reported |
| Play Store | Cannot be suspended, but `setApplicationHidden` hides it (no launcher entry) | Hide/unhide Play Store or use the install restriction below |
| Block installs | `no_install_apps` refuses Play installs **and updates** (see round 2 below); round 1 also reported an adb refusal, not retested here | Cannot use this as a permanent new-app gate while promising automatic updates |
| New-app window | This debug manifest receiver observed **no** `PACKAGE_ADDED` delivery; a freshly installed app launched unsuspended. This does not prove package events are universally unavailable | Test dynamic registration / the supported delivery mechanism; periodic reconciliation alone leaves a launch window |
| Uninstall | `setUninstallBlocked` refused for a user app (`DELETE_FAILED_OWNER_BLOCKED`); not possible for system apps | Block uninstall of managed user apps; uninstalling a suspended app would otherwise drop its suspension |
| Reboot | Suspensions survive reboot, **unless** the reboot happens within seconds of the change (not yet persisted) | Re-apply desired state at boot (P7 reconcile) |

## Play Store controls for a Device Owner

Source: https://developer.android.com/work/dpc/security (via the user's pointer to an
r/Intune thread on blocking personal accounts on fully managed devices).

- `setApplicationRestrictions(admin, "com.android.vending", bundle)` with
  **`allowed_accounts`** = comma-separated account emails that may use Google Play; an
  **empty string blocks every account**. Play 80970100+.
- `DISALLOW_MODIFY_ACCOUNTS` stops adding other Google accounts (e.g. a friend's).
- Device-wide unknown-source block: `verify_apps:device_wide_unknown_source_block` in the
  same bundle + `ENSURE_VERIFY_APPS` + `DISALLOW_INSTALL_UNKNOWN_SOURCES`.
- The page itself names scheduled `setPackagesSuspended` as the way to block games or
  YouTube at certain times — the supported mechanism for our use case.

### Round 2: signed-in Play Store (2026-10-21)

Same disposable Android 17 emulator, Play Store version **85073430**, debug Device
Owner. The user created and signed into an ordinary Google test account. No physical
phone or production router was changed. Account addresses are omitted; the debug probe
logs only `unset`, `deny-all`, or `allowlist (redacted)`.

| Control | Observation |
|---|---|
| Unrestricted baseline | Authenticator installed (`7002011`); Keep installed (`220679618`) after an earlier blocked attempt |
| `no_install_apps=true`, new installation | Keep remained absent on the first short attempt. A separate KDE Connect attempt remained absent after 70 s and added an installer log rejection, `User restriction prevents installing` |
| `no_install_apps=true`, existing-app update | Photos stayed at `51413605` through 120 s after tapping Update. With the restriction cleared, the same update completed to `52549343` within 45 s. Gmail similarly stayed at `65615106` under restriction, then updated to `66089035` unrestricted |
| `allowed_accounts=<signed-in test account>` | Tasks installed (`2100325`); YouTube Music updated from `91052280` to `94152280` within 45 s. This is an account allowlist, **not an app allowlist** |
| `allowed_accounts=""` | After restarting Play Store, app-detail links led to the **Sign in** screen instead of an install/update page. Manual updates were therefore inaccessible; unattended/background update behaviour was **not independently tested** |
| `no_modify_accounts=true` | Tapping **Add account** from Android's account settings produced **Blocked by work policy / For more info, contact your IT admin**. A direct Add-account intent was not reliable evidence (it can finish back onto an older chooser); use the normal Settings path for this check |

All three test restrictions were cleared afterwards (`no_install_apps=false`,
`no_modify_accounts=false`, `allowed_accounts` removed). Emulator and test account were
preserved for follow-up; installed test apps were left in place.

**Design consequence:** an account allowlist plus account-modification restriction can
keep Play visible and prevent switching to an unapproved account, but does not approve
individual apps. Permanent `no_install_apps` also prevents updates; a deny-all account
list removes manual Play access. Neither is a demonstrated solution to **automatic
updates plus parent-approved new apps**. Keep this acceptance gate open. If installations
remain enabled, the new-app launch race must be solved/tested rather than claiming a
periodic suspension pass closes it. No production policy decision follows from this
emulator experiment alone.

### Round 3: package visibility, dynamic events and first-launch race

Reproducer: `test/android/install-probe/{build.sh,test.py}` and debug-only
`PackageWatchProbe`. The synthetic APK logs `Activity.onCreate`; no child app or account
is involved. Only this fixture package is suspended by the experiment.

- Without explicit package visibility the foreground service also saw **no package
  events**. Adding `<queries><package ...fixture.../></queries>` made dynamic delivery
  work. Round 1 therefore did **not** establish that Android cannot deliver these events.
- A running `systemExempted` foreground service (eligible as Device Owner) received
  `PACKAGE_ADDED`, `PACKAGE_REMOVED` and `PACKAGE_REPLACED`, including replacement flags.
- The first five fresh-install/launch races had no observed successful first start;
  suspension completed about **42–100 ms after callback entry**. This measures callback
  handling, **not installation-to-suspension latency**.
- A second five-race run after reboot observed **three successful `onCreate` calls**.
  One preceded the added callback; two happened even after the suspension call returned,
  consistent with an already-dispatched launch. A successful suspension return is not
  proof that no app code ran.
- Deliberately pausing the watcher process also let the fixture start; resuming it applied
  the queued suspension. This controlled delay is a separate test, not natural latency.
- In-place APK replacement delivered the expected events and stayed suspended. Reboot
  restarted the watcher and reconciled suspension. The test uses a changed boot ID and
  waits for unlock/boot reconciliation rather than trusting an old `sys.boot_completed`.
- The runnable test passed and cleaned up its fixture APK and watcher. The existing
  Google account was preserved. No physical phone or router was touched.

**Conclusion:** this is useful fast detection and eventual enforcement, but **not
parental approval before any first execution**. Keeping public Play installs enabled
cannot be described as a closed installation gap on this evidence. Update-safe strict
approval remains a separate unresolved gate; neither a foreground service nor a periodic
reconcile is a pre-install authorization hook.

Official references: [broadcast registration](https://developer.android.com/reference/android/content/Context#registerReceiver(android.content.BroadcastReceiver,android.content.IntentFilter)),
[package-added semantics](https://developer.android.com/reference/android/content/Intent#ACTION_PACKAGE_ADDED),
[foreground service types](https://developer.android.com/develop/background-work/services/fgs/service-types).

## Still to verify on the user's Pixel 10 (needs a separate OK each time)

Battery/Doze cost of counting and syncing, Spotify with the screen off, YouTube
picture-in-picture, OEM behaviour of the suspension dialog, and that the UID-mode usage
grant survives a real reboot. This requires installing a build with the same release
signing key as the pilot app — a production change on the user's own phone.
