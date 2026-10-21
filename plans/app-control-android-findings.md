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
| Picture-in-picture | Could not force Chrome into PiP via adb on the emulator | Count **visible** time (`RESUMED` → `STOPPED`, cut by screen-off): covers PiP and split screen by construction; verify with YouTube on the Pixel 10 |
| Background audio, screen off | Emulator has no audio device; playback could not be confirmed | By the screen-off cut it cannot count (D5); verify with Spotify on the Pixel 10 |

## App suspension (DevicePolicyManager)

| Question | Result | Consequence |
|---|---|---|
| Suspend an ordinary app | `setPackagesSuspended` works; launching shows the system dialog | Core mechanism confirmed |
| Dialog text | Title "Blocked by work policy" is fixed; body is replaced by `setShortSupportMessage` ("Paused by FamilyWarden. Ask a parent for more time in the FamilyWarden app.") | Set a FamilyWarden support message (EN/DA by locale); title stays Android's |
| Not suspendable | FamilyWarden itself, **Play Store**, default dialer, active launcher, Settings — returned in the failed list | Never rely on suspending them; the failed list must be reported |
| Play Store | Cannot be suspended, but `setApplicationHidden` hides it (no launcher entry) | Hide/unhide Play Store or use the install restriction below |
| Block installs | `no_install_apps` user restriction refuses installs (adb included) | Default for children: installs blocked; lifted only for a parent-approved install |
| New-app window | `PACKAGE_ADDED` is **not delivered** to a manifest receiver (implicit-broadcast limits): a freshly installed app launched unsuspended | With installs blocked this window does not open; a periodic reconcile suspends anything unapproved as a backstop |
| Uninstall | `setUninstallBlocked` refused for a user app (`DELETE_FAILED_OWNER_BLOCKED`); not possible for system apps | Block uninstall of managed user apps; uninstalling a suspended app would otherwise drop its suspension |
| Reboot | Suspensions survive reboot, **unless** the reboot happens within seconds of the change (not yet persisted) | Re-apply desired state at boot (P7 reconcile) |

## Still to verify on the user's Pixel 10 (needs a separate OK each time)

Battery/Doze cost of counting and syncing, Spotify with the screen off, YouTube
picture-in-picture, OEM behaviour of the suspension dialog, and that the UID-mode usage
grant survives a real reboot. This requires installing a build with the same release
signing key as the pilot app — a production change on the user's own phone.
