# Bounded Android app-control experiments

Only a disposable **Android 17 Play emulator**, already enrolled with the FamilyWarden
**debug** Device Owner. Never use a child's phone or a production owner. Neither script
wipes the emulator, removes accounts, changes pairing, nor connects to a router.
The scripts refuse serials not starting with `emulator-`.

Build the debug app using the existing persistent debug-key volume (do not replace the
keystore between installs):

```sh
docker run --rm --memory=2g \
  -v "$PWD/android:/src" -v mast-gradle:/root/.gradle \
  -v mast-android-debugkey:/root/.android -w /src \
  ghcr.io/cirruslabs/android-sdk:35 \
  ./gradlew --no-daemon --max-workers=2 '-Dorg.gradle.jvmargs=-Xmx768m' \
  -Pkotlin.compiler.execution.strategy=in-process testDebugUnitTest assembleDebug lintDebug
adb -s emulator-5556 install -r android/app/build/outputs/apk/debug/app-debug.apk
```

Build only the synthetic target APK (two versions for replacement tests):

```sh
mkdir -p /tmp/fw-install-probe
docker run --rm --memory=1g --cpus=2 -v "$PWD:/src:ro" -v /tmp/fw-install-probe:/out \
  -v mast-android-debugkey:/root/.android ghcr.io/cirruslabs/android-sdk:35 \
  sh /src/test/android/install-probe/build.sh
```

The fixture builder creates a standard **test-only** debug key if the mounted volume
is empty (for example, CI downloaded the FamilyWarden APK without running Gradle).
It preserves an existing key, including refusing a corrupt key rather than silently
replacing it. Both fixture versions use that same signer; it does not have to match
the FamilyWarden APK's certificate. Production signing keys are never mounted here.

Then run **one experiment at a time**:

```sh
python3 test/android/install-probe/test.py /tmp/fw-install-probe --reboot
python3 test/android/install-probe/usage_test.py /tmp/fw-install-probe --reboot
```

- `test.py`: observes dynamic package events, races first launch against suspension,
  replaces the APK, demonstrates a controlled watcher-process pause, and optionally
  reboots. Successful first execution is a **finding**, not an assertion failure. This
  experiment is not a proof that all possible first-launch races are blocked.
- `usage_test.py`: grants UID-mode usage access, measures foreground/screen-off/background
  time, restarts the collector, exhausts a local quota, replaces the APK, revokes/restores
  access, and optionally reboots. It leaves usage access allowed and retains the private
  usage checkpoint for inspection. Cumulative counters are not reset for repeated runs.
- Both stop their own experimental service and remove only
  `io.github.denogio.familywarden.fixture` in `finally` cleanup. Do not run either while
  the other watcher is active. The code never blanket-suspends device packages.

The real reducer, atomic checkpoint, collector and safe enforcer are in `src/main`;
activation is **debug-only**. No v2 signed policy, enrollment key, DNS setting, pact,
router state or approved-removal guard is changed. The release manifest contains no
probe service/receiver; the existing release app does not activate this collector.
This is a tested local foundation, not completion of P5/P6/P7 or a deployable shared budget.
See [capability findings](../../../plans/app-control-android-findings.md) for limitations.

## Native activity / bootstrap research (also debug-only)

Rebuild the fixture APKs after changes to its Activity/Provider sources, then:

```sh
python3 test/android/install-probe/lock_task_test.py /tmp/fw-install-probe
# Optional: requires an available Maps update; queues it BEFORE entering native mode.
python3 test/android/install-probe/lock_task_test.py /tmp/fw-install-probe \
  --play-update com.google.android.apps.maps
python3 test/android/install-probe/home_boot_test.py /tmp/fw-install-probe \
  --with-pin --expect-preunlock-lock
# Observation only: may legitimately end INCONCLUSIVE; does not queue any Play update.
python3 test/android/install-probe/auto_update_observe.py --seconds 600
```

- These probes are hardware-guarded to the emulator. No release receiver, activity, Home
  intent filter or pre-unlock policy activation is added.
- `lock_task_test.py` checks five first-launch races, same-task activity blocking, Gmail
  as a positive control, and operation while the owner process is paused. It also shows
  that an exported ContentProvider **can execute** while activities are blocked.
- `auto_update_observe.py` keeps native lock task active, sleeps the screen and watches
  Calendar/Messages versions for a bounded interval. It changes no update preference.
  No change is inconclusive, not a failed deterministic test. It restores policy/display.
- The optional Play check proves **queued update completion**, not autonomous scheduling.
  Already-updated targets make this test inconclusive; do not downgrade production apps
  to manufacture an update.
- `home_boot_test.py` temporarily changes the default Home and restores the exact old
  component. `--with-pin` first verifies there is no existing credential; only then does
  it set its public synthetic PIN and clear it in cleanup. Never reuse this on a phone.
- `--expect-preunlock-lock` checks native readiness before the script unlocks. The first
  observed boot mode can still be `NONE`; passing this check is **not** proof of absence
  of an earlier readiness window or robustness to delayed/crashed owner startup.
- Cleanup stops the original locked root and verifies both credential/device-protected
  experiment markers are cleared. The native probe includes last-resort emulator-only
  task removal; the Home test can use a non-wiping recovery reboot after restoring Home.
  Needing either recovery is a finding, not evidence of a production-safe exit path.

Read [continued research](../../../plans/app-control-install-research.md) before choosing
this as a product architecture. Quick Settings and other daily-phone behavior differ.
No actual telephone or emergency call is placed by these scripts.
