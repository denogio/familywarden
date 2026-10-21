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
docker run --rm -v "$PWD:/src:ro" -v /tmp/fw-install-probe:/out \
  -v mast-android-debugkey:/root/.android ghcr.io/cirruslabs/android-sdk:35 \
  sh /src/test/android/install-probe/build.sh
```

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
