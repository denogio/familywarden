# App approval / Play updates — continued research

Research and disposable-emulator experiments: **2026-10-21–22**.
Status: **promising activity-control alternative, not a production decision**.
No physical phone, production router, owner migration or account removal.

## Separate the requirements

1. Parent approval **before installation**.
2. Parent approval before the child can open/use an app's **activities**.
3. No unapproved app **code whatsoever**, including exported providers/background work.
4. Already-approved apps continue receiving updates.

These are not equivalent. Neither `PACKAGE_ADDED` suspension nor lock task establishes all
four. Do not turn one failed approach into a claim that every Android option is exhausted.

## What we can currently substantiate

| Route | Evidence | Remaining limitation |
|---|---|---|
| `no_install_apps` | Existing emulator evidence: blocks new Play installs **and updates** | Not a permanent update-safe gate |
| Play `allowed_accounts` | Restricts which account uses Play, not which app is installed | Not an app catalog/approval policy |
| Post-install suspend/hide | Dynamic delivery works with correct package visibility; suspension race reproduced | Callback is after installation; cannot promise no first execution |
| Installer session observer | Public `SessionCallback` is lifecycle observation. `openSession`/`abandonSession` require session ownership | Not a documented global parent veto over Play's sessions |
| Installer `requestUserPreapproval` | Installer-owned session API; public docs explicitly reject non-owners | Not a DPC hook for intercepting Play's approvals |
| Managed Google Play | Managed catalog/app-update policy exists | No new custom-DPC Play EMM registrations; AMAPI has eligibility and owner-model constraints |
| Native multi-app lock task | OS activity allowlist, tested below; updates can proceed with Play UI excluded | Not a background-code barrier; bootstrap and ordinary-phone UX need work |
| DPC-owned updater / maintenance windows | A different installer can control its own approved APK inputs | Authentic Play APK acquisition, licenses, splits, update ownership and safe windows are not solved; no unofficial APK service chosen |

### Installer references

- [UserManager](https://developer.android.com/reference/android/os/UserManager#DISALLOW_INSTALL_APPS)
  and [AOSP source](https://raw.githubusercontent.com/aosp-mirror/platform_frameworks_base/main/core/java/android/os/UserManager.java)
  describe `DISALLOW_INSTALL_APPS` as also preventing device/profile owners from installing.
  The fact that Play **updates** fail is separately runtime-tested; do not attribute an
  explicit update exception to documentation that does not state one.
- [SessionCallback](https://developer.android.com/reference/android/content/pm/PackageInstaller.SessionCallback)
  defines `onCreated`/`onFinished` as `void` notifications, not authorization decisions.
- [PackageInstaller](https://developer.android.com/reference/android/content/pm/PackageInstaller#abandonSession(int))
  documents `SecurityException` when abandoning/opening someone else's session.
- [Session preapproval](https://developer.android.com/reference/android/content/pm/PackageInstaller.Session#requestUserPreapproval(android.content.pm.PackageInstaller.PreapprovalDetails,android.content.IntentSender))
  is not transferable to another installer's session.

This is a review of these APIs, not proof that no future/vendor-specific API can help.
For example, the latest reference also lists registered-app-store restrictions (37.2);
those must not be assumed to distinguish fresh installs from updates or to exist on the
37.0 test image. OTA/system-update policy is a different subject entirely.

## Managed Google Play / AMAPI feasibility

Google's [EMM registration page](https://developers.google.com/android/work/play/emm-api/register)
(last updated 2025-11-13 in the fetched source) explicitly says new registrations for
custom DPCs using Play EMM API are no longer accepted. Existing validated EMMs remain
supported. New EMMs are directed to AMAPI and Google's **Android Device Policy** DPC.

[AMAPI permissible usage](https://developers.google.com/android/management/permissible-usage)
limits eligibility to the stated commercial EMM/Device Trust/OEM use cases and provides an
inquiry form for usage outside its guidelines. A self-hosted household project is **not
shown to qualify**. The page does not explicitly name all parental-control products; do
not misquote it as a blanket Android ban on parental control or on native Device Owner APIs.

Using Google's DPC would also change the present owner/enforcement architecture; it is
not an API token we can simply add to FamilyWarden's existing owner. No such migration
is proposed or authorized here. The separate [approved-DPC enrollment page](https://support.google.com/work/android/answer/16694822)
concerns enterprise provisioning restrictions; it should not be stretched into a claim
that every ADB/test/native-DPC path has identical rules.

## Native activity allowlist: meaningful positive evidence

Sources: [lock-task guide](https://developer.android.com/work/dpc/dedicated-devices/lock-task-mode),
[`BLOCK_ACTIVITY_START_IN_TASK`](https://developer.android.com/reference/android/app/admin/DevicePolicyManager#LOCK_TASK_FEATURE_BLOCK_ACTIVITY_START_IN_TASK),
[activity manifest](https://developer.android.com/guide/topics/manifest/activity-element#locktaskmode).

The debug research controller uses `setLockTaskPackages` plus
`LOCK_TASK_FEATURE_BLOCK_ACTIVITY_START_IN_TASK`; the latter matters because an allowed
app can otherwise launch an unallowlisted activity inside its existing task. It enables
Home, Recents, notifications, system info, power menu and keyguard feature flags.

`lock_task_test.py` passed on the existing Android 17 Play emulator:

- Five fresh-install / immediate-activity-launch races: **zero fixture Activity.onCreate**.
  The fixture remained **unsuspended**; this is not an old suspension masquerading as a
  native-lock-task result.
- A non-allowlisted activity requested **inside an allowlisted task** was blocked.
- Approved Gmail opened as a positive control, including its first-run redirect.
- The activity block held while the FamilyWarden process was deliberately paused.
- Play stayed installed and unsuspended, but **not activity-allowlisted**.
- A queued Google Play Maps update completed while native mode remained `LOCKED`:
  **1068516954 → 1068832648**.

**Update scope:** this update was requested in Play before entering lock task, and was
still pending on entry. Completion establishes that this native activity policy does not
necessarily kill an ongoing background Play update. It does **not** yet establish normal
periodic automatic-update scheduling. No auto-update preference was changed in this round.

A separate **601-second** observation sent no foreground Play update request. Native
mode stayed locked, AC power was present and the screen was asleep. Calendar
(`2018015783`) and Messages (`302567063`) did not change version. This is
**inconclusive**: the auto-update preference and qualifying schedule/network state were
not established. It is not evidence that lock task blocks automatic updates.
[Consumer Play help](https://support.google.com/googleplay/answer/113412) also notes
sign-in/permission/restart conditions and user-controlled update preferences; it does
not provide a Device Owner-specific timing guarantee.

### Not a code-execution allowlist

The synthetic APK now includes an exported `ContentProvider`. While native activity
launch remained blocked, a normal content-query interface caused **PROVIDER_CREATED** and
returned synthetic data. No fixture Activity.onCreate was observed.

Therefore this is an **activity/use barrier**, not a guarantee that unapproved package
code cannot execute. The test used `adb shell content query`; the provider deliberately
has no read permission, but this is not a demonstrated complete child-facing exploit
through an approved app. Do not generalize either direction beyond what was measured.

## Boot / first-unlock research

1. Ordinary lock task started by an activity was **NONE after reboot** without re-entry.
2. A temporary default Home with `android:lockTaskMode="if_whitelisted"` re-entered native
   mode after boot, but one scripted first-unlock race still started the fixture once.
3. A separate direct-boot-aware receiver was then added for research. It uses only a
   device-protected experiment flag and existing DPC allowlisting, not CE policy/account
   keys. It requests the direct-boot-aware Home on `LOCKED_BOOT_COMPLETED`.
4. The latest PIN-locked experiment observed `RUNNING_LOCKED`. The **first mode sample was
   NONE**, followed by `LOCKED` before the script typed the PIN. The first-unlock race then
   observed **zero** fixture starts, and steady-state launch remained blocked.

This is progress, **not proof of a continuously active barrier from earliest boot**.
There was an observed readiness interval; earliest unlock, delayed/crashed owner startup,
notification/deep-link unlock routes and repeated/load tests remain open. The script's
`--expect-preunlock-lock` checks readiness before it actually unlocks, not the absence of
any earlier unguarded interval. Do not call the bootstrap gap solved.

Sources: [Home/default-launcher cookbook](https://developer.android.com/work/dpc/dedicated-devices/cookbook),
[Direct Boot](https://developer.android.com/privacy-and-security/direct-boot).
The experiment temporarily changes the emulator's Home role; a real managed-home design
would need an approved persistent-preferred policy and recovery design, not just this
shell test. No private enrollment keys or full policy are moved into device-protected storage.

## UX and recovery are acceptance gates, not cosmetic details

- Android documents that **Quick Settings remains disabled** in lock task even with
  notifications enabled. Re-enabling Home/Recents/notifications does not equal a stock phone.
- Notifications, incoming/outgoing calls, SMS, alarms, camera, Bluetooth/Wi-Fi changes,
  biometrics, wallets, PiP, accessibility and parent-approved settings need targeted checks.
- [AOSP LockTaskController](https://raw.githubusercontent.com/aosp-mirror/platform_frameworks_base/main/services/core/java/com/android/server/wm/LockTaskController.java)
  contains emergency-intent exceptions when the keyguard feature is allowed. This is
  source evidence, **not a successful emergency-call test**, and the mirror branch need
  not match every OEM/image. No real call was placed and no call/emergency-call
  activity intent was started.
- Initial cleanup by simply removing the allowlist left a privileged BlockedAppActivity
  locked task. The harness now stops its original root and restores the prior Home before
  clearing the saved policy. Tests also verify the saved experiment marker is gone; a
  transient `NONE` mode alone was not sufficient proof of cleanup.
- A real phone must have parent-authorized, offline-safe recovery and preserve current
  signed removal/pact guards. Debug stop extras/receivers in this harness are intentionally
  test controls and **must never ship as release authority**.

## Reproducers and state left behind

See `test/android/install-probe/README.md`. Main research components are debug-only:
`LockTaskProbe`, `LockTaskHomeProbe`, `LockTaskBootProbe`; no release activation was added.

The Home experiment verifies an empty credential before setting its public synthetic PIN,
then restores the exact prior Home, clears CE/DE experiment markers, exits lock task,
removes its fixture APK and clears its own PIN. No wipe/account removal. Current test
account and original Nexus Launcher are retained. Physical Pixel and production untouched.

## Continue from here

1. Independently verify **scheduled** Play auto-updates with no foreground Play session,
   under unchanged/native policy, charging/network/idle conditions recorded.
2. Bound the bootstrap/readiness interval and first-unlock notification/deep-link cases;
   consider the documented bound DPC service (`ACTION_DEVICE_ADMIN_SERVICE`) for lifecycle
   support, but do not confuse process availability with a native launch guarantee.
3. Test ordinary-phone UX/recovery in the emulator before considering any physical pilot.
4. Decide whether an activity/use allowlist and its UI tradeoffs meet the product goal.
   If the requirement is approval before **installation/all code**, keep that gate open
   rather than rebranding this activity-control result as equivalent.
