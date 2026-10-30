# CI2 emulator diagnosis

## Approved-minute failure: product race, not slow KVM accounting

Run `40490235715` on `41f309b` did not consume the approved minute. Its synthetic
`app-usage.json` records `used=33009`, `uncertain=2050`, and fixture foreground
usage of exactly `33009` ms. The screen was interactive/unlocked, but the visible
activity was FamilyWarden, not the fixture. `activity.log` confirms this.

The more decisive `android-system.log` and `usage-events.log` show:

- 11:03:15: fixture resumes.
- 11:03:41: the test kills FamilyWarden's process.
- 11:03:44: `DEBUG_SYNC` starts the process again.
- 11:03:45: the fixture is suspended; Android closes its task and resumes
  FamilyWarden.
- 11:03:47: the fixture is unsuspended, but its foreground task does not return.

Waiting longer would not fix this: usage correctly stops when the fixture is no
longer visible. The prior diagnosis and fix are already recorded in
[core-migration.md](core-migration.md) under **Root cause (2026-10-28, from those
diagnostics of CI run 40490235715)**. Commit `7e600bb` prevents enforcement of a
policy snapshot replaced during grant-input reads. It is absent from `41f309b`
and present in the CI1 artifacts. That record also documents three passing P8
runs after the fix. No new product change is warranted by these diagnostics.

## Post-CI1 local reproduction

Used the retained `4e2cf639f9f19eec14d49a3e015c22ca3bde8a45` artifacts from green
CI1 run `40966230574`, attempt 1. `scripts/ci-artifacts.py verify` verified all
three manifests and package SHA-256 hashes before testing. No APK rebuild,
GitHub dispatch, live router, physical phone or QEMU pact was involved. Both
scripts use disposable Docker guests; container-internal `adb devices` showed
only `emulator-5554`.

`scripts/test-mast-emulator.sh` passed completely (exit 0), including:

- cached-pact restrictions and UI after reboot with the disposable router paused;
- fresh signed release and `Free sailing`;
- blocked-app suspension, running `AppControlService`, and signed acknowledgement
  `32388d03a546d454`;
- daily-limit pause, bonus behavior, block without usage access, and removal.

Stage snapshots include logcat, activity/service dumps and the UI hierarchy.
There was no null-root, fatal exception or ANR in those captured logs.

The focused `scripts/test-requests-emulator.sh` rerun passed completely (exit 0),
including parent approval/decline, history, inventory, signed rule CAS and scope
revocation. `requests/result.json` records `checks=passed`, `spent_millis=60000`
and `parent_device_owner=false`. The **unchanged** 115-second exhaustion/re-pause
wait and both exact `60000` ms assertions passed. During the grant, persisted
usage samples were:

| Journal cursor (wall ms) | Grant used (ms) | Visible activity |
| --- | --- | --- |
| 1793338622851 | 11760 | fixture |
| 1793338637899 | 26808 | fixture |
| 1793338642056 | 30965 | fixture |
| 1793338657125 | 46034 | fixture |
| 1793338675608 | 60000 | FamilyWarden, after suspension |

Between the first and fourth samples, usage and journal wall time both advance
by exactly 34274 ms. The fixture remains visible across process restart. This
provides no evidence for systematically slow foreground accrual or a larger
exhaustion timeout.

Local synthetic evidence (not committed): `/tmp/ci2-evidence/` contains the old
CI bundles/logs; `/tmp/ci2-local/` contains `mast.log`, `mast.exit`,
`mast-diag-*/`, `requests.log`, `requests.exit`, `requests/result.json`,
`requests-accrual.log`,
`requests-grant-usage.json`, `requests-grant-android.log`, and
`requests-usage-events.log`. These local paths are not durable CI artifacts.

## Separate reboot/session failure: not reproduced, cause still unproven

Run `40964996537` on `036b960` had a null-root UiAutomator error, then missing
release UI, no service/acknowledgement and cascading app-limit failures. It has
no failure diagnostics bundle. A successful current rerun does **not** establish
whether that historical failure was a degraded emulator/UI session or a genuine
service/policy failure. Keep this classification open; do not weaken assertions
or introduce product changes based only on the null-root symptom.

The harness now reports `no node for <text>` / `cannot tap <text>` or
`no checkable node for switch` / `cannot tap switch` and returns nonzero before
issuing an input tap. Previously these paths raised `IndexError` or
`StopIteration`. Five fast tests exercise the actual shell functions with fake
adb, covering missing nodes and preserving successful selector behavior.

Fast validation passed:

- `python3 -m unittest discover -s test/android -p 'test_mast_emulator.py'` (five tests).
- `bash scripts/test-syntax.sh`.
- `go test ./...` in `core/`.
- `git diff --check`.

No Android source changed, so Android JVM tests were not rerun. Both disposable
emulator/router pairs and their networks were removed after evidence collection.

CI2 should not be represented as fully closed while the separate historical
reboot/session cause remains unproven. A future reproduction needs diagnostics
at the first UI/service failure, not only the later requests-stage bundle.

## First-failure capture

`scripts/test-mast-emulator.sh` now snapshots the device at its first failed check
when `FW_MAST_DIAG_DIR` is set: the failing label, logcat, FamilyWarden service and
activity dumps, window state, UI hierarchy and a screenshot. CI sets it for the
emulator job and uploads the directory with the existing failure-only diagnostics
artifact (7-day retention). The next null-root/session failure can then be
classified from the first failure instead of its cascade.
