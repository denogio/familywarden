# CI4 — Combined CI and Android verification optimization plan

Status: plan (2026-10-30). Backlog item: CI4 in `plans/app-control-todo.md`.
Check-by-check evidence: [ci4-android-test-map.md](ci4-android-test-map.md).

## Goal and guardrails

Make the pipeline cheaper and faster without losing a single guarantee.

- Every guarantee stays proven: either it remains in an emulator stage or a named,
  lighter test proves it with equal strength. Update the map's "moved to" column for
  each move; remove an emulator check only after its replacement is green in CI.
- No assertion, timeout or product behavior is weakened to gain speed.
- Caches are accelerators, never authority: a cache miss must still produce the same
  result, cache keys include every input that changes the output, and released
  artifacts stay SHA-bound with provenance (`docs/ci-artifacts.md`).
- Measure before and after each step from CI step timings and record the numbers here.
- One step per commit; CI2 first-failure diagnostics (`FW_MAST_DIAG_DIR`) stay on.

## Baseline (measured)

Push run 41014185856 (`a7a4d80`) and emulator dispatch 41010097715 (`7fc31e3`):

| Job | Time | Where the time goes |
|---|---|---|
| Build apks (x86_64) | 5m24s | SDK image pull + feeds ~1.5m; dependency compile ~2.5m (lua, libubox, ubus, uci, ucode, rpcd, csstidy, luci-base); our packages <5s; smoke 9s; legacy migration 45s |
| Core apk (aarch64) | 5m02s | same SDK pattern |
| Ulysses Mast (Android) | 5m42s | Gradle without any cache: 323s for unit tests, lint, release + debug |
| LuCI e2e | 1m53s | runs after Build apks |
| Go core / Test | ~1m50s each | already parallel |
| Emulator dispatch | 21m16s | SDK image pull ~40s + `sdkmanager` emulator/system image ~45s (fresh volume every run); two cold boots; ~25 `uiautomator` dumps at 10–25s; two fixed 115s waits; one in-suite reboot ~75s |

Push gate critical path: Build apks → LuCI e2e ≈ 7.5m (Mast 5.7m in parallel).
The OpenWrt router container itself is cheap: ~7s in CI including the image pull
(~2s locally); it is not a bottleneck and stays as the emulator's router.

## Phase A — caching (no test changes)

A1. **Gradle** (Mast job): add Gradle caching (`setup-java` `cache: gradle` or
    `gradle/actions/setup-gradle`). Expect 323s → ~2–3m. Release signing and
    versionCode inputs are unaffected (they are not cached outputs).

A2. **Android SDK for the emulator**: replace the named `mast-sdk` volume in CI with a
    bind directory (`FW_MAST_SDK_DIR`, local default unchanged) restored by
    `actions/cache`, keyed on system image id + emulator package revision. Removes the
    ~45s `sdkmanager` download. Measure restore time; keep only if faster.

A3. **OpenWrt SDK builds** (x86_64 and aarch64): stop rebuilding unchanged dependencies.
    Two candidates, decided by measurement:
    - cache the SDK's `dl/`, `feeds/`, `staging_dir/` and dependency `build_dir/`
      outputs, keyed on SDK image digest + feeds revision + build script hash; or
    - package our three packages without compiling their dependencies (if the SDK
      supports it for these packages).
    **Accept only if** the produced `.apk` files are content-identical to a clean build
    (compare file lists, control metadata and payload hashes; package signatures and
    timestamps excepted) and smoke/upgrade tests stay green. Expect ~2.5m saved per job.

A4. **SDK container image pull** (~40s in the emulator job): evaluate after A2. Options:
    run the emulator on the runner host with the cached SDK (the runner has Java and
    KVM), keeping the container path for local runs; or accept the pull. Decide by
    measured gain versus local/CI parity.

## Phase B — slimmer emulator verification (CI4 steps 2–3)

B1. **Fake-clock Robolectric test** for the approved-minute guarantee (design in the
    map, section (c)): seams `AppControlService.tick()` → `internal` and an injectable
    clock on `AppControl`; no logic change. Asserts exact 60,000 ms exhaustion and
    re-pause, no refill on repeated delivery, retained spend across process death, and
    the CI2 mid-read replacement race. The emulator keeps one real foreground-accrual
    check (~30s) instead of two 115s waits.

B2. **Move pure state/UI checks to Robolectric** per the map: dark/200% text, Danish/
    English, navigation extras, address-repair rejection cases. Each moved check names
    its replacement test in the map.

B3. **One emulator for both suites**: one network, one boot, a fresh router per suite.
    Switch to `system-images;android-35;google_atd;x86_64` (verified available; keeps
    dialer/clock/contacts needed by candidate-app checks). Boot from a snapshot taken at
    "booted, animations off, no apps installed"; evaluate caching that snapshot across
    runs (A2 cache) after measuring restore time.

B4. **Two emulator modes**:
    - dispatch smoke (`mast_emulator=true`): the minimal set in the map, section (a);
    - full milestone run (`mast_emulator_full=true`): everything, including reboot
      without the router, DNS-not-served, repair-back and the moved UI variants.
    The full run is required before any release tag and before physical rollout.

## Order and targets

| Step | Effort | Expected effect |
|---|---|---|
| A1 Gradle cache | small | Mast job 5.7m → ~3m |
| A2 emulator SDK cache | small | −45s per dispatch |
| A3 OpenWrt dependency reuse | medium, verify identical output | Build apks 5.4m → ~3m; aarch64 5m → ~2.5m; push gate ~7.5m → ~5m |
| B1 Robolectric minute test | medium | guarantee proven in seconds; enables dropping 2×115s waits |
| B3 shared ATD emulator + snapshot | medium | −1.5–2m boot, lower CPU/RAM |
| B2 move UI/state checks | medium, incremental | −several minutes of UI dumps |
| B4 smoke/full split | small | dispatch smoke < 5m; full ~10m |
| A4 SDK image pull | measure first | up to −40s |

Targets: push gate ≈ 5 minutes; emulator smoke under 5 minutes; full emulator run
about 10 minutes; zero guarantees lost (map fully accounted for).

## Out of scope

- A native-core "test router" replacing the OpenWrt container: measured saving ~5s per
  suite; the real router is cheap and more realistic.
- Maestro: same emulator cost; does not address where the time goes.
