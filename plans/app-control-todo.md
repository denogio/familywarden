# App control — implementation backlog

Branch: `feature/app-control`, based on `v0.9.1` (`dce92f4`).
Status (2026-10-27): LAN app control, requests/grants, usage and new-app approval are
implemented and deployed. Optional Go configuration/pact authority, LuCI CAS writers
and AGH reconciliation are implemented on this branch, not deployed; broader migration
validation remains open. Off-home transport remains undecided. Phase evidence below includes historical checkpoints;
see [core-migration.md](core-migration.md) for current ownership and deployment.

## Current work and remaining scope (reconciled 2026-10-27)

Checkboxes track the stated implementation or validation item, not the whole product.
An implemented feature is not automatically physically verified, deployed or released.
Historical test counts are evidence at that checkpoint, not current suite totals.

| Workstream | Implemented | Still open |
|---|---|---|
| P2 Android capabilities | Usage collection, suspension, package lifecycle, signed activation | Physical PiP/split-screen accuracy, Doze/battery/latency and remaining lifecycle gates |
| P3–P5 Go/OpenWrt identity and policy | Packaged daemon, enrollment/roles/scopes, signing, capabilities, applied revisions; deployed | Minimum resource guidance, remaining address-repair/compatibility validation; not full product migration |
| P6 usage | Durable local counters, signed sequenced uploads, router-local 30-day app summaries, scoped views | Physical accuracy/battery validation, domain history; shared cached offsets do not bound offline overspend |
| P7/P7b child enforcement | Signed app/total limits, bonus, downtime, warnings, safe exceptions and new-app approval | Remaining physical/lifecycle validation; full future policy editors are separate UI work |
| P8 LAN requests | Child/parent/admin flow, device/key-bound grants, consumption, first-answer-wins and new-app decisions; deployed | Focused physical request/approval run and bounded multi-phone allocation |
| P9 off-home | LAN notifications only | Transport decision, encryption/key distribution and off-home implementation/testing |
| P10 Android | Child/adult status/history, scoped parent decisions and usage | Parent presentation split is implemented and preview-tested (#94); scoped child selection is implemented and preview-tested (#97); full parent controls remain |
| P10 shared web | Existing LuCI pages and shared Requests module, now hardened/tested in neutral and LuCI hosts; control prototype reviewed | Production-wide shared presentation layer, core API integration, host adapter and EN/DA; prototype is not shipped code |
| Core migration | Identity/requests/usage/new-apps; opt-in authoritative configuration/pacts, CAS/confirm/rollback, Settings/Profiles CAS; owned phone/app policy from the document; Go AGH reconciliation/catalogue | **Not adapter-ready:** owned bonus, manual-network/grant, pact-permit revocation, status/expiry, runtime facts and portable network intent/exact apply receipts migrated/tested; DNS/release orchestration, platform construction/shared UI and P12 safety gates; owned configuration cutover not deployed (#91; [R0–R7](adapter-portability.md)) |
| P11 adult pool | Existing pact enforcement, not the proposed pool | Final consumption/reset rules, ledger and pact-safe self-service |
| P13 release validation | Same-key rc3/rc4 router+phone pilot and rc5 phone update | Release-wide security/compatibility/lifecycle acceptance for remaining scope; no blanket production rollout approval |

Current session task titles should name the **remaining** work rather than imply
that an implemented feature has not started. A legacy phase task remains pending
when its remaining validation or scope is incomplete; it must not block unrelated
implementation merely because the original broad task was never closed.
Product decisions and research: [app-control.md](app-control.md).

This is the durable checklist. Complete a box only after its acceptance evidence is
recorded. Work in the order below, with small reviewable commits. Open choices are
not permission to silently decide product behavior.

## Scope and boundaries

- Go core, OpenWrt first; preserve a boundary for future OPNsense/pfSense adapters.
  Do not implement speculative FreeBSD adapters now.
- Router-agnostic by design (A18): new product logic goes in the Go core, never in the
  shell plugin or behind OpenWrt-only assumptions; the shell plugin only shrinks.
- One Android app: managed child, parent approver, and adult self-limiter roles.
  Being an administrator and being subject to a pact remain independent.
- Separate ordinary Google accounts on children's phones; preserve the README warning
  about Google's age/account rules. No false-birthdate instructions or recovery hacks.
- Children: app limits, usage summaries, new-app approval, and one-off extra-time requests.
- Adults: social/browser limits, DNS protection, pact restrictions and a small daily
  discretionary pool; no self-approved escape beyond that pool during a pact.
- Reuse enrollment, keys, policy validation, Compose, DNS and pact behavior already built.
- No deployment, owner migration, account removal or live-pact modification without
  separate authorization. The son's daily Pixel 7 Pro is not a test device.
- Do not restart the compatibility emulator by default. Use fast tests; group emulator
  checks into one run per major milestone. Physical tests require approval.
- English documentation/default UI, full Danish Android localization, fictional fixtures.

## Test each slice narrowly

Use a focused Go unit test for core logic, a targeted adapter/subprocess or disposable-
container check for the platform bridge, and module/component tests for UI changes.
Full browser e2e and Android emulator suites run at integrated milestones, not after
every function. Widen the test only for a concrete changed boundary or observed failure.
OpenWrt is the only current platform target; preserve future options without building
another router adapter or treating cross-platform builds as routine milestones.
The adapter must retain native enforcement freedom (schedules, priorities, deadlines),
while core owns their product semantics. No new schedule-polling daemon is implied.

## Adapter-ready core completion gate (A18, clarified 2026-10-27)

Migration is done when a new router supplies facts, applies core-owned enforcement
intent and provides native service/auth/UI integration **without reimplementing
product rules**. Passing OpenWrt tests or owning UCI's replacement data alone is
not enough. Source evidence and dependencies: [adapter portability audit](adapter-portability.md).

- [x] R0: audit active owned-mode and legacy dependencies; separate implementation,
      portability, validation and deployment status without a speculative percentage.
- [ ] R1: migrate bonus, manual overrides and additive network-ticket product state,
      writers/readers and expiry/eligibility into core.
      - [x] R1a: owned bonus migrated with one-time import/upgrade, core policy/list
        readers and legacy-writer isolation (core 0.13.0-r10/backend rc10-r7).
        Go/race and 13 focused OpenWrt bridge checks pass; no full e2e/emulator run.
      - [x] R1b: complete network state/expiry/status consumers.
        - [x] Owned block/allow/reset/ticket and grant-repair decisions in Go, with
          shared RAM/native apply transaction and rollback (core r11/backend rc10-r8).
          Focused Go/race, affected adapter/configuration tests and 34 real OpenWrt
          network checks pass; no browser e2e/emulator run or deployment.
        - [x] Owned pact-permit revocation moved to core (core r12/backend rc10-r9),
          retaining blocks/other profiles and undoing RAM changes on failed apply.
          Legacy post-lock cutover guard and versioned apply handshake are covered by
          focused Go/race and 42 real OpenWrt pact-network checks; no full e2e/emulator.
        - [x] Owned status/expiry report in core (core r13/backend rc10-r10): owned
          `list`/`status` forward to `configuration_report`; precedence, allow windows,
          read-time expiry, pact details and bonus come from one locked runtime snapshot.
          Focused Go/race, adapter snapshot checks and 22 real OpenWrt status checks
          pass; native expiry cleanup stays kernel/adapter-owned; no full e2e/emulator.
- [x] R2: replace owned policy-shaped `core-context`/`core-policy` reads with explicit
      runtime/application facts; isolate legacy import in OpenWrt compatibility code.
      - [x] Owned `Context`/`Policy`/`WithContext` and bonus reads now use the
        versioned `core-facts` envelope (`runtime-facts-v1`): handshakes, timezone,
        `pact_captain`/`mast_captain` drift checks, profile presence, per-profile
        volatile override and verified DNS. No policy content crosses the boundary.
        Focused Go/race, adapter facts checks and 11 real OpenWrt `core-facts` checks
        pass; legacy `core-context`/`core-policy` remains the pre-import policy owner;
        no full e2e/emulator run or deployment.
- [x] R3: portable ordered network intent and exact applied-state/error receipts
      (core r15/backend rc10-r12). Core compiles effective profiles, precedence,
      windows, tickets, device/default policy and lockdown; Go OpenWrt renders nft,
      shell only discovers LAN and validates/applies. Configuration status distinguishes
      acknowledgement from live verification; failed/missing/foreign intent never falls
      back to legacy policy. Go/race/vet, syntax, 495 full integration checks, focused
      network/pact/status/bonus checks, fresh-package smoke and 65+4 browser tests pass.
      LuCI r5 fixes the browser-discovered shared action busy-state race with a
      deterministic held-response regression. No emulator or production deployment.
- [ ] R4: complete portable DNS lifecycle/state and protected-release orchestration;
      retain native secret/auth/lifecycle integration and verification-before-ack.
- [ ] R5: separate platform construction and finish shared core-backed administration
      with thin host bridges (P10), not a new framework or standalone login.
- [ ] R6: run the production service and shared UI against a recording/failing adapter
      with no shell/UCI/ubus/fw4/OpenWrt paths, then preserve real OpenWrt parity.
- [ ] R7: close P12 upgrade/import/interruption, compatibility and safe restore gates
      before removing legacy paths or claiming production-cutover readiness.

No pfSense/OPNsense adapter is implemented yet. The FreeBSD cross-build succeeds,
but still selects OpenWrt and is not runtime support. Existing fake-adapter unit tests
are useful evidence, not the completed R6 production-composition gate. Actual second-
platform implementation and production deployment remain separate scope/authorization.

## Baseline and migration inventory

Existing implementation to read before moving behavior:

| Responsibility | Current source / state |
|---|---|
| Firewall/platform application, legacy pact/configuration | `familywarden/usr/lib/rpcd/familywarden` |
| Phone identity and owned configuration/pact/app policy | `core/internal/identity`, `core/internal/configuration` |
| AGH reconciliation and service catalogue | `core/internal/agh` (shell retains platform storage/credentials) |
| Boot, reload, pact/DNS tick | `familywarden/etc/init.d/familywarden` |
| Signed phone endpoint | `familywarden/www-familywarden/cgi-bin/mast` |
| Device identity and policy validation | Android `PhoneKey.kt`, `Payload.kt`, `Sync.kt`, `Store.kt` |
| Current Android enforcement | `Mast.kt`, `PrivateDns.kt`, `AppliedDns.kt`, `DotProbe.kt` |
| Parent administration | LuCI `devices.js`, `profiles.js`, `settings.js` |
| Persistent secrets and pact state | `/etc/familywarden/`, root-only where appropriate |
| Kernel enforcement regression suite | `scripts/test-integration.sh` |
| Real disposable AGH tests | `scripts/test-agh.sh` |
| Browser / Android lifecycle tests | `test/e2e/`, Android unit tests, `scripts/test-mast-emulator.sh` |

## Design workstream — align LuCI with Android

Selected before feature implementation; visual changes must preserve current behavior.

- [x] Research current LuCI pages, Android identity and implementation constraints.
- [x] Prepare [the continuation prompt](../docs/design/luci-design-prompt.md) for the
      existing Claude Design thread; [research notes](../docs/design/luci-design-research.md).
- [x] Submit the prompt to Claude Design and obtain the design export (user handoff).
- [x] Finish the full multi-page review of exported states, factual claims, accessibility
      and native LuCI compatibility; Overview-specific findings are addressed.
- [x] After design approval, implement Overview first and test in disposable LuCI.
      [Implementation evidence and preserved exports](../docs/design/luci/README.md):
      24 browser tests pass, scoped light/dark styling, no production deployment.
- [x] After Overview acceptance, apply the shared visual language to other existing pages
      (all pages; released in LuCI 0.7.0 / v0.9.2 and deployed on rivendell).

The legacy LuCI visual refresh is complete and deployed on all pages. Its original
Overview-only notes are historical. App control is implemented; the separate shared
control-interface prototype is reviewed but not yet a production-wide replacement.
See [the prototype review](../docs/design/control-prototype-review.md).

Writer checkpoint (#103): partial identity updates resolve omitted fields
under the core's final validation/save mutex, preventing stale patches from restoring
old administrator/profile/name values. Deterministic regression, Go vet/race and real
OpenWrt integration pass. Core 0.10.0-r2 is prepared, not deployed; this does not finish
the separate cross-process configuration/pact transaction or CAS cutover.

Runtime gate follow-up: core 0.11.0/backend rc8 serialize runtime identity, grant,
new-app and authenticated-upload mutations with the platform pact/state gate through
persistence, not just the context read. Go vet/race and real OpenWrt checks pass,
including actual inherited-fd lock lifetime and a negative regression probe. Install
these development packages together (`go-apps-v3`); no phone wire/store schema change
or deployment. At that checkpoint full household ownership/CAS/apply-confirm/rollback
and direct UCI forms were still open. The subsequent optional authority and LuCI
writer cutover are implemented; see the current checkpoint below.

## P0 — Establish the implementation baseline

- [x] Create the branch from the merged v0.9.1 commit.
- [x] Write this backlog and link it from the product plan and roadmap.
- [x] Update stale current-status sections in `CLAUDE.md`: DNS and enrollment are
      implemented, v0.9.2 released and deployed on rivendell (2026-10-21).
- [x] Inventory shell APIs, policy wire format, keys, UCI options and persistent files with
      current/proposed owners: [app-control-baseline.md](app-control-baseline.md).
- [x] Record baseline versions, test results (integration 192, Playwright 29 in CI) and
      available test environments (same file).
- [x] Mark the knowledge graph as stale; refresh after the Go architecture settles.

**Accept:** a reproducible baseline, no claim that old planning text describes current code.

## P1 — Resolve contracts and product semantics

Depends on P0. Discuss unresolved choices before building their enforcement.

- [x] Specify signed device roles, parent-to-child scope, revocation and multiple parents.
      (Decided: router **and** parent phones may sign grants — app-control.md decision 2.)
      A normal parent phone must not need Device Owner merely to approve requests.
- [x] Define precedence: essential apps, explicit blocks, downtime, per-app/category
      quotas, one-off grants, adult pool, and active pacts.
- [x] Define whether grants bypass downtime/explicit blocks, and when unused minutes expire.
      (Decided: grants work after bedtime, valid 2 h, also open router internet.)
- [x] Define per-device vs shared multi-device budgets; do not claim global offline
      budgets without an allocation strategy that prevents double spending.
      (Decided: shared per child; bounded offline overspend, reconciled — decision 4.)
- [x] Define usage: foreground time, screen lock, split screen, picture-in-picture,
      background audio, overlapping activities and incomplete Android event history.
      (Decided: screen-on foreground incl. PiP; screen-off background audio excluded.)
- [x] Define household timezone/day boundaries, DST, clock rollback and travel behavior.
      (Decided: router-timezone midnight; signed server_time + monotonic clock.)
- [x] Set request rate limits, outstanding-request bounds, expiry and repeated-denial UX (A7).
      (Decided: no rate limit, one open request per child, expires after 1 hour.)
- [x] Choose usage retention/deletion and visibility for child, parent and adult (A8).
      (Decided: 30 days, app activity and website domains. Visibility per role still to detail.)
- [x] Decide initial Go/shell responsibility split and later migration boundary (A6).
- [ ] Choose off-home transport (A2): compare direct/VPN, relay and FCM wake-up options
      against self-hosting, delivery reliability, Doze, privacy and setup cost.
      FCM is not inherently incompatible with self-hosting; document actual credentials
      and service dependencies. Signing is not encryption.
- [x] Specify who authorizes offline grants: router only, or delegated parent keys;
      cover revocation, trust expiry and offline replay/double-spend limits.
      (Decided: delegated parent keys; mechanics go in the protocol document.)
- [x] Record schema examples, error codes and compatibility rules in a protocol document:
      [app-control-protocol.md](app-control-protocol.md) (draft; fields final once tested).

**Accept:** explicit decisions for the first vertical slice; deferred choices stay labeled.

## P2 — Verify Android capabilities with bounded experiments

Depends on the relevant P1 semantics. No child-phone experiments.

- [x] Check current official Android API behavior for UsageStats, package suspension,
      package visibility, special access, foreground execution and install restrictions.
- [x] Prove usage access grant/setup, detection of revocation and recovery. Treat adb
      app-ops setup as a candidate, not proof that Device Owner can self-grant access.
- [x] Test suspend/unsuspend of one disposable app, failure returns, active-app behavior
      and non-suspendable/system packages; never blanket-suspend critical packages.
- [x] Identify launcher, dialer/emergency access, input method, Settings recovery and
      FamilyWarden itself as safety cases with explicit allowlist rules.
- [x] Verify package-added/replaced/removed events and the install-to-suspend race.
      Round 3: explicit package visibility + dynamic foreground receiver works; a second
      five-race run allowed three first executions before enforcement took effect.
      Replacement and reboot reconciliation passed. Strict first-use approval is **not**
      achieved by post-install suspension. Continued native lock-task/Home research is
      tracked in `app-control-install-research.md`: activity blocking is promising,
      queued Play update completion passes, but background code/readiness/UX gates remain.
- [ ] Measure counting/enforcement latency and battery cost with screen on/off and Doze.
- [x] Verify ordinary-account sign-in on an authorized disposable setup. Round 2:
      user signed into a test account; Play installs/updates succeeded without restrictions.
      `no_install_apps` also blocked updates; account allowlisting alone did not gate apps.
- [ ] Record results, device/API/build and known bypasses; remove test state afterwards.

**Accept:** a documented supported approach and honest limitations, without production changes.

Rounds 1–2 (emulator, Android 17) recorded 2026-10-21:
[app-control-android-findings.md](app-control-android-findings.md). Round 2 restrictions
were cleared; emulator/account are retained for follow-up (no wipe).
Open: update-safe strict new-app approval, battery/Doze, Spotify
screen-off, YouTube PiP, real-device usage-grant persistence. The user approved proceeding
with a real counting/enforcement build and Pixel 10 test; describe the exact same-key APK,
changes and rollback before touching the phone. No physical-device changes made yet.

## P3 — Bootstrap the Go core

Depends on P0/P1. Use the standard library first; add dependencies only for concrete needs.

- [x] Create Go module, executable, configuration validation and clean shutdown (`core/`).
- [x] Separate pure domain rules from storage, API transport and the OpenWrt boundary;
      no adapter framework or speculative interfaces with no actual consumer.
- [x] Choose durable storage based on atomicity, router flash use, backups and build size.
      (Plain JSON files, atomic rename + fsync, written only on change; no dependencies.)
      Define schema versioning, crash recovery and bounded queues before writing data.
- [x] Add restricted local service transport, health/version/capability reporting and
      redacted logs; do not expose an unauthenticated WAN listener.
- [x] Add deterministic clocks and fixtures for policy, expiry and day-boundary tests.
- [x] Add Go formatting, unit tests, race checks where supported and build jobs to CI.

**Accept:** a small runnable core with durable-state tests; legacy behavior still works.

Done 2026-10-21: `core/` with `usign` (phone/router key format), `protocol` (signed
request/grant verification, error codes), `rules` (precedence evaluator), `store` (usage
ledger, one open request, 30-day prune) and `familywardend` (root-only unix socket,
health/version, tzdata embedded). Static linux/arm64 binary: 6.4 MB (rivendell has
~35 MB free flash). Schema versioning/migrations wait until the first stored format ships.

## P4 — Integrate OpenWrt without a big-bang rewrite

Depends on P3 and ownership decisions from P1.

- [x] Add architecture-specific Go package builds, procd lifecycle and configuration.
      Separate package `familywarden-core` (OpenWrt `golang-package.mk`, Go from the
      packages feed bootstrapped from the official golang image); the shell package stays
      `PKGARCH:=all` and unchanged. `scripts/build-apk-docker.sh` builds it (`WITH_CORE=0` skips).
- [x] Connect authenticated LuCI/ubus calls through a narrow local bridge and least-privilege ACLs.
      Identity, requests, usage and new-app decisions use the root-only socket bridge.
      Shared Requests UI uses LuCI sessions/ACLs. Real unmocked HTTP approval regression
      added 2026-10-25; strict JSON remains strict after transport session cleanup.
- [x] Keep the existing fw4 compiler and sail endpoint authoritative initially.
      The core touches neither; stopping, crashing or removing it leaves fw4 untouched (tested).
- [x] Define reload ordering, failures, timeouts and single-writer ownership per state file —
      see "Core on OpenWrt: ownership" below.
- [x] Add clean install, upgrade, restart and uninstall tests with preserved configuration.
      `scripts/test-apk.sh` (container) and `scripts/test-core-lifecycle.sh` (real procd).
- [ ] Measure package/RAM/storage cost on target architectures; document minimum requirements.
      Measured on x86_64 only (below). aarch64_cortex-a53 package build/measurement open.

**Accept:** disposable OpenWrt installation and reboot work without DNS/pact/firewall regressions.

**Migration checkpoint (2026-10-25):** signed app-policy compilation/revision/features
and raw app-rule normalization now live in Go. Latest integration suite: 404 checks,
including a frozen legacy-normalizer oracle and fail-closed pact snapshot compilation.
Profiles, schedules, authoritative app configuration and pacts are still stored in
UCI/shell. See `core-migration.md` steps 3a/3b for evidence and the traced cutover callers.
The raw-normalization and pact-lock checkpoints also pass 36 Playwright tests and
forced-package reinstall checks of daemon PID/version. The lock suite has 409 integration
checks, with the pre-fix snapshot failing deterministic lock/concurrency probes.
Phones now reads the profile directory through the core API (including disabled profiles),
without creating a parallel store: Go/race and 412 integration checks pass; full package/
browser checks are running on an immutable snapshot. These migration changes are not
deployed; production remains rc4 / core 0.6.0.

**Current migration checkpoint (2026-10-26):** opt-in import now gives the core one
revision-sealed configured/frozen document, with CAS/apply/confirm and restart rollback.
Settings and Profiles save through CAS in owned mode. Phone pact/rules and app compiler
input come from the document; app revisions agree across phone, context and locked
mutation paths (runtime bonus/timezone remain adapter facts). A divergent runtime
projection cannot change the frozen phone app limit. Go also owns AGH reconciliation
and the service catalogue, including empty-list/concurrent-change safeguards; owned
DNS profiles/domain/endpoint come from the document. Evidence: Go vet/race, 461 real
OpenWrt integration checks and 72 real AGH checks; per-slice CI and remaining boundaries
are recorded in `core-migration.md`. No deployment or legacy-path removal is implied.

### Core on OpenWrt: ownership and lifecycle (historical baseline, 2026-10-22)

| Path | Owner (single writer) | Notes |
|---|---|---|
| `/etc/familywarden/core/` (0700) | `familywardend` | Durable core state; kept on upgrade **and removal** (user data) |
| `/var/run/familywarden-core/core.sock` (0600, dir 0700) | `familywardend` | Root-only; not shared with the shell's `/var/run/familywarden` |
| everything else under `/etc/familywarden`, `/tmp/familywarden.*`, `/var/etc/familywarden.include`, UCI | shell plugin | Unchanged; the core reads none of it yet |

- **Ordering:** independent services. `familywarden` (START=95) never waits for or calls
  the core, so a broken core cannot delay or break firewall rules. The core (START=94)
  needs nothing from the shell.
- **Failures:** procd respawns with back-off (`respawn 3600 5 5`). Startup refuses a
  `-socket` path that is not a socket and refuses to start a second instance on a live
  socket; a stale socket after a crash is taken over. The status bridge times out after 3 s.
- **rpcd:** exec plugins are only scanned at (re)load, so postinst/postrm reload rpcd —
  without it the ubus object was missing after install and lingered after removal (found
  in the QEMU test).
- **Release:** CI builds and tests the x86_64 core but does not attach it to releases yet.

Measured 2026-10-22 on OpenWrt 25.12.4 x86_64 (container + disposable QEMU, real procd):
package 2.9 MB (apk), binary 7.3 MB installed, idle RSS 7.4–8.1 MB. Build adds about
8 min the first time (Go toolchain). Production rivendell had ~35 MB overlay and
~118 MB RAM free at last read: fits, but re-measure on aarch64 before any deployment.
Tests passed: `test-apk.sh` (install, ubus while down/up, 0600/0700 modes, second
instance refused, crash → stale socket → takeover, reinstall/removal keep data) and
`test-core-lifecycle.sh` (enabled + ubus right after a plain `apk add`, procd respawn,
reboot, stop/start, removal; fw4 chain and shell backend unchanged throughout).

## P5 — Extend identity and signed policy

Depends on P3/P4 and P1 protocol decisions.

- [x] Reuse current full-key fingerprint approval and per-device request authentication.
      v3 adds a second signature by the same enrolled phone key; nothing new to approve.
- [x] Preserve pinned router identity and existing phone keys during migration.
      No key, pin or enrollment change; the Go core now owns reply signing (core migration step 1).
- [x] Add scoped roles, app rules, schema/capability negotiation and policy revisions.
      Done: app rules + revision in wire v3, version negotiation by dual signature.
      Added 2026-10-24: authenticated `caps`/`sigcaps`, strict `requires` and signed refusals;
      no false current ack from incompatible/legacy clients (protocol §2b). 285 integration
      checks, 149 JVM tests, lint/debug build/release compilation and 13 browser tests pass;
      no emulator was needed at that checkpoint. Parent role/`approves`/`parents` keys
      subsequently shipped with P8; see protocol §3–4 and core identity tests.
- [x] Bound payloads, verify signed fields before parsing into trusted state, and reject
      unknown unsafe versions, mismatched devices, replay and downgrade.
      Signature over exact bytes before parsing (unchanged); strict apps validation;
      once v3 was accepted an older reply is refused ("downgraded reply").
- [x] Persist last trusted policy atomically; retain it offline without a 24/48-hour
      escalation. Unchanged Store behaviour; payload, enrollment latch and highest wire
      version are one preference commit. Removal still needs a fresh signed reply.
- [x] Add applied-state acknowledgements; fetching policy is not evidence of enforcement.
      Signed `applied` revision per phone, shown as `apps_applied` vs `apps_rev`. The app
      sends `-` until enforcement (P7) sets `Store.appsApplied`.
- [x] Address same-key router-address repair without accepting redirects or replacing pins.
      An enrolled phone may scan/enter its router again (also managed or pact-bound): same
      pinned key -> only the address changes, after a fresh request-bound signed reply from
      the new address (`Mast.repairAddress`). Another key still needs removal authority.
      Unit-tested and verified on Android 15 against two real container endpoints while
      pact-bound (#99): foreign-key, unreachable and unsigned replies refused; pin and
      device identity preserved. Physical network/address transitions remain a gate.
- [x] Test pending enrollment, changed pending keys, revoked roles and old Android clients.
      Done: pending/foreign-key v3, v2-only phones, router-rollback downgrade (integration
      + JVM), and the real debug app against a real v3 router on the Android 15 emulator
      (CI run 39820424952: pairing, approval, pact, DNS, offline cache, signed removal).
      Subsequently: current parent scope/requester state is rechecked in core request
      security and usage tests; parent roles are implemented, not deferred. This does
      not close every physical-device lifecycle/compatibility gate.

Wire v3 as built, with the compatibility/rollback matrix: `app-control-protocol.md` §2a.
Evidence 2026-10-22: 224 integration checks (32 new, pact-freeze tests fail when the
snapshot omits app rules), 121 JVM tests incl. a real router-signed v3 fixture.

**Accept:** mixed-version integration tests and a documented rollback/compatibility matrix.

## P6 — Collect usage privately and durably

Depends on P2/P5 and chosen accounting rules.

- [x] Implement Android usage-event ingestion with an atomic cursor/counter checkpoint
      and half-open event intervals; tested process-death/restart and duplicate handling.
- [x] Track per-app/day locally, reconcile active sessions and handle missing history
      honestly; missing permission is not zero usage.
- [x] Distinguish missing administrator weekly reports from measured zero (#102):
      core-owned `reported` metadata, conservative legacy fallback and EN/DA chart
      labels. Go vet/race, six Node tests and 53 browser tests pass; core 0.10.0 is
      prepared, not deployed. Follow-up CI 40279978074 also passes package smoke,
      aarch64 build, packaged LuCI e2e and Android checks.
- [x] Keep local counters/enforcement usable offline with the last trusted policy.
- [ ] Measure/review checkpoint write frequency and battery cost; throttle safely without
      making persisted grant consumption refill after a crash.
- [x] Upload signed, sequenced summary updates idempotently; prevent duplicates and
      out-of-order retries from increasing/decreasing totals incorrectly.
- [x] Enforce per-device ownership and reject impossible or oversized usage submissions.
      A signed report authenticates its source, not truth on a compromised device.
- [x] Bound app summaries to 30 router-local days, with authenticated/scoped access;
      do not upload account identifiers or raw usage events.
- [ ] Implement domain history and its retention/visibility rules (A8/A14); verify
      explicit deletion/backup behavior separately from automatic app-data retention.
- [ ] Finish the complete lifecycle matrix: synthetic midnight/DST, clock/gap and
      bounded-input tests exist; physical event accuracy, offline/reinstall coverage
      and battery behavior are not a blanket completed gate.

**Accept:** known synthetic event traces yield exact expected summaries and retries do not drift.

Initial evidence (2026-10-21): `UsageCounter`, `UsageJournal`, `UsageMonitor` and
110 JVM tests plus emulator usage/restart/update/revocation/reboot checks. Subsequent
production-path evidence: `a2f4e30`, `dca5fc2`, `e8670f9`, core `usage_test.go`, Android
usage/grant tests and `scripts/test-requests-emulator.sh`. Signed activation, uploads,
router retention and shared offsets are now implemented. Public activity-class identity,
physical PiP/split-screen accuracy, checkpoint cost and the remaining lifecycle matrix
stay open. Shared offsets are reconciliation, not bounded offline allocation.

## P7 — Enforce child app rules locally

Depends on P2/P5/P6.

- [x] Add app inventory and stable package identity; treat labels/icons as untrusted UI data.
- [x] Implement a pure evaluator for schedules, quotas and precedence from P1.
- [x] Suspend/unsuspend only managed packages, record failures and reconcile at boot,
      package changes, policy updates and budget/schedule boundaries.
- [x] Handle package replacement/uninstall, newly installed apps and essential-app exceptions.
      Model decided (A12): suspend new user apps on the dynamic package event (needs
      package visibility) plus boot/periodic reconcile; Play installs/updates stay on.
      UI and docs must state the brief first-launch window, never "blocked before use".
- [x] Protect relevant configuration, time, uninstall and user/profile changes where supported.
- [x] Restore only FamilyWarden-owned restrictions on approved removal; never erase another
      administrator's policy. Preserve the existing signed removal and pact guards.
- [x] Show exhausted quota, reason, remaining time and unavailable usage access honestly.
- [ ] Finish physical essential-app, package/OEM and lifecycle safety validation; an
      accepted post-install first-launch window is not pre-execution protection.

**Accept:** targeted runtime evidence for exhaustion, offline restart and essential-app safety;
unit tests cover precedence. No claim of root/recovery-proof enforcement.

The initial 2026-10-21 debug foundation is historical. The production path now uses
router-signed policy, pure `appAction`, tracked suspension ownership, persisted grants
and shared offsets; removal retains the signed/pact guards. Evidence includes the
P7/P8 emulator runs, the rc3/rc4 same-key pilot and `73b921b` for new-app approval.
No claim is made that shared offline overspend is bounded or physical safety validation
is complete. Post-install suspension retains the accepted first-launch race (A12).

## P7b — Total screen time, bonus and warnings (decisions D9/D10)

Implemented and included in the later rc3/rc4 pilot. The original `0.10.0-pilot2`
label is historical; remaining physical validation is tracked separately.

- [x] UCI: profile `screen_minutes` (all days) or `screen_weekday` + `screen_weekend`;
      list `always_allowed` packages. Router expands to a per-weekday map in wire v3
      (`apps.screen.{mon..sun}`), part of the revision and the pact snapshot.
- [x] Bonus: `ubus call familywarden bonus '{"profile":..,"minutes":30[,"package":..]}'`
      adds to today's total (or one app); router-local day, gone at midnight; in the signed
      policy as `apps.bonus` with its day; never changes the standing limit. Bounded and
      additive like tickets; refused for an adult profile bound by a pact.
- [x] Phone: count the union of visible ordinary apps (not essential/always-allowed); when
      total (+ bonus) is used up, pause every ordinary app; lift at midnight or on bonus.
- [x] Notifications: 5 minutes left (per limited app and total, once per day) and paused.
- [x] Pause dialog body via `setShortSupportMessage` (EN/DA), cleared on removal.
- [x] LuCI: one place to set all days / weekdays + weekend and always-allowed apps; bonus
      buttons (+15/+30/+60) on the overview card. HA via the same ubus method.
- [ ] Tests: router expansion/revision/pact freeze/bonus expiry; JVM union counting,
      weekday lookup in router zone, midnight with bonus; emulator total exhaustion keeps
      dialer/launcher/Settings usable exist. Remaining: close the full physical safety
      matrix; the original pilot2 target was superseded by the rc3/rc4 deployments.
- [x] Downtime pauses ordinary apps; essential and configured always-allowed apps remain
      usable. SMS is not a built-in essential exception.
- [ ] Reconcile the original separate “allowed in downtime” option with the current
      single `alwaysAllowed` set before claiming separate per-app downtime controls.
- [ ] Finish Family Link-aligned parent presentation and editors (P10); do not treat
      the implemented child status/request flow as completion of all design screens.

## P8 — Deliver the first end-to-end extra-time slice

Depends on P5–P7. Start on LAN before adding the selected off-home transport.

**LAN slice implemented end to end (2026-10-25, emulator-verified and subsequently deployed).** Go:
signed LAN requests/decisions, scoped
parent/admin approval, durable first-answer-wins ledger, device/key-bound policy grants,
capability gating and retryable OpenWrt internet tickets. Strict parser, concurrent-parent,
replay, stale requester, clock rollback, expiry, capacity and failed-write tests run with
the fake adapter; real CGI/ubus/firewall tests cover transport and application. The old
unexposed request/grant verifier and request-store prototypes were removed, not exposed
as competing authorities. Current contract: `app-control-protocol.md` §3–4.
Android: strict grant parsing, consumption in the usage checkpoint, grant-aware
enforcement, `PolicyClock`, exact-byte durable outbox, child request and parent approval
screens (EN/DA), parent-only mode without Device Owner, LAN request notifications.
Evidence: 185 JVM tests; `scripts/test-requests-emulator.sh` (Android 15 + OpenWrt) and
`scripts/test-requests-ui.sh` (previews, dark, Danish 200%). Contract: protocol §3–4.

- [x] Child requests minutes for one app/day with a unique request ID and expiry.
- [x] Parent sees pending requests for authorized children and approves/denies in Android.
- [x] Verify role, scope, limits and signed authorization; never widen the weekday budget.
- [x] Persist grants and consumption; make double taps, retries, concurrent parents and
      duplicate delivery idempotent. Expired/revoked requests must not revive.
- [x] Apply one grant exactly once on the intended phone; report received vs applied states.
      (Received = signed ack of the grant revision; spending stays on the phone.)
- [x] Enforce request abuse limits and show expired/denied/offline states clearly.
      (One open request per child; storage bounds; no product rate limit, per A15.)
- [x] Test wrong child/parent/day/app, replay, tampering, restart and grant exhaustion.
- [ ] Physical-device run (Pixel test phone only, never a child's daily phone).
- [x] Authenticated usage and grant-consumption upload (protocol §5, 2026-10-25).
- [ ] Shared multi-phone allocation and router-side budgets (P6/P7 follow-up).

**Accept:** child exhausts an app, requests extra time, parent approves, only the approved
minutes become usable, and the standing rule remains unchanged.

## P9 — Add off-home delivery and notifications

Depends on P1 transport decision and P8.

- [ ] Implement the selected transport with bounded queues, reconnect/backoff and expiry.
- [ ] Separate wake-up/delivery from authorization; relay possession never grants approval.
- [ ] If relaying policy or usage, implement reviewed encryption/key distribution as well
      as signatures; document metadata exposure and key rotation/revocation.
- [ ] Handle Android notification permission/channels, Doze, process death and reboot.
- [ ] Verify Wi-Fi/mobile transitions, router/relay outage, duplicate/out-of-order messages
      and multiple parent phones; report stale reachability rather than silent success.
- [ ] Measure delivery latency and battery cost; document external-service dependencies
      and self-hosted configuration. Do not require a separate push app.

**Accept:** the P8 flow works off-home in a controlled test with verified security and outage behavior.

## P10 — Build parent, child and LuCI management views

Develop incrementally with P5–P9; no separate web dashboard framework.

First child/adult UI slice (2026-10-24): Today, reliable local counts, daily/app limits,
bonus-aware remaining time, downtime, always-allowed apps, local history totals and
captain pact status implemented. EN/DA, 139 JVM tests and focused light/dark/200%-text
emulator render checks pass at that checkpoint. Parent roles, requests, new-app
approval and uploaded usage views subsequently shipped. The parent navigation split
is implemented and preview-tested; pools, full parent editors and extended history remain open.
Details and deliberate design deviations: `docs/design/family-views/UI-SPEC.md`.

- [x] Parent Android: scoped time/new-app decisions and reported per-child usage summaries.
- [x] Parent Android: Screen time / Controls / Requests and My phone / Family split
      (#94); 197 JVM tests, lint/debug/release compilation, navigation-validator tests
      and grouped Android 15 EN/DA/200%-font preview checks pass. No policy-edit API
      or new deployment is implied.
- [x] Parent Android: scoped child selection on Screen time/Controls, combined Requests
      inbox and selection across pages/modes (#97). 199 JVM tests, lint/builds and
      Android 15 EN/DA-200% interactions pass, including scope revocation.
- [ ] Parent Android: full app inventory/rule editors and truthful per-device
      connection/applied-policy views.
- [x] Child Android: remaining budgets, pause reasons, request status/remaining extra
      and local usage/history totals. Domain history remains P6 work.
- [x] Existing LuCI: configure rules/roles, view usage/requests and inspect app revisions,
      capabilities and application failures.
      (2026-10-25: Requests page = first shared administrator UI module (A20): answer
      requests, see today per phone/app, 7-day totals, reported grant use. Parent Android
      shows reported usage per scoped child; #94 moves it from Requests to Screen time.)
- [x] Include new-app approval (2026-10-25, protocol §5b; LuCI Requests page and parent phone).
- [ ] Essential-app warnings and removal/revocation workflows.
- [x] Existing Android views: EN/DA strings/plurals, accessibility labels, large-text,
      dark/light themes and fictional previews/screenshots. New screens must preserve this.
- [x] Existing slices: mapper/unit tests and focused Playwright coverage of real backend
      requests/responses (`b3ded32`, `37a0146` and current Android verification).
- [x] Harden the existing shared Requests foundation (#101): persistent action errors,
      single-flight operations, stale-action guards, draft/focus preservation and late-
      reply disposal. Same `.fwa` module passes neutral-host and real-LuCI tests;
      [implementation evidence](../docs/design/control-interface-implementation.md).
- [ ] Implement the complete control prototype on that shared foundation, remaining
      core-backed views/editors, host integration and EN/DA (#96). This is not yet a
      production-wide replacement of the legacy LuCI pages.

**Accept:** UI reflects server authority and applied state, and has loading/error/offline states.

## P11 — Add adult limits and the daily discretionary pool

Depends on P6–P8 and existing pact compatibility.

- [ ] Define pool size/reset/consumption semantics with the user before implementation.
- [ ] Implement persistent pool ledger; retries/reboots/clock rollback cannot refill it.
- [ ] Bind app limits, pool and DNS restrictions to active pact state; only tightening
      is allowed outside amendment/end rules, including through parent/admin APIs.
- [ ] Prevent a captain from self-approving extra grants beyond the pool or escaping by
      role/profile/device rebinding; retain the supported sail cooling-off flow.
- [ ] Test cumulative tightening: removing a restriction added after pact creation must
      not loosen it. Owned DNS now has an explicit DNS-only transaction that persists
      the cumulative minimum: Go/restart tests, real AGH removal attempts and unmocked
      LuCI deferred-disable checks pass (core 0.13.0-r9/backend rc10-r6/LuCI r4).
      Legacy-shell/app/pool parity remains open; the old snapshot-plus-live union alone
      is not sufficient to retain a restriction added after pact creation.
- [ ] Test amendment windows, expiry, scheduled boundaries, offline use and approved removal.

**Accept:** integration tests demonstrate no new self-approval or DNS/app escape path.

## P12 — Migrate existing DNS/identity responsibilities safely

Depends on P4/P5/P11. Move only responsibilities assigned to Go; this is not a rewrite mandate.
P12 closes migration safety, not the whole router-agnostic architecture. The separate
[adapter-ready R0–R7 gate](adapter-portability.md) also requires migrating runtime
product decisions and proving a production service that does not depend on OpenWrt.

- [ ] Preserve root-private secrets, approved devices, router keys, ClientIDs, ownership
      records, DNS acknowledgements and all pact snapshots.
- [x] Port AGH reconciliation with the existing real-container tests: own clients only,
      preserve foreign settings, no empty-allowlist lockout, verify changes before ack.
      (`core/internal/agh`, `familywardend agh-sync`; owned routers take the desired set
      from the core document, legacy routers from UCI; client restrictions and replaced
      access lists are read back before acknowledging the policy.)
- [ ] Preserve strict DNS/no automatic unfiltered fallback and handset applied/probe status.
- [ ] Add idempotent migration, backup and interruption tests; stop dual writers.
- [ ] Define rollback limits after schema changes; never restore a backup that silently
      loosens an active pact or replays already-spent grants. Safety limits are recorded
      in `core-migration.md` (no ownership-marker deletion, no partial-ledger restore,
      no unvalidated downgrade); automated safe restoration/mixed-version validation
      is still missing.
- [ ] Remove legacy paths only after parity and mixed-version tests pass.

**Accept:** v0.9.1 fixtures upgrade without changing effective restrictions or identities.

## P13 — Harden, validate and prepare a pilot

Depends on the implemented feature paths; no production rollout implied.

- [ ] Review authentication, authorization, key storage, malformed inputs, replay,
      counters, revocation, log privacy, transport encryption and resource exhaustion.
- [ ] Run fast Go/Kotlin tests routinely; run relevant fw4/AGH/Playwright suites when
      touching those layers. Run one grouped emulator suite at the milestone.
- [ ] Exercise crash/power loss, offline reboot, before-first-unlock, startup clock,
      hotspot under pact, corrupt cache and unavailable usage access on approved devices.
- [ ] Test old router/new app and new router/old app combinations or explicitly refuse
      incompatible versions without erasing last trusted protection.
- [ ] Document setup, ordinary-account warning, supported Android versions, permission
      recovery, backup/restore, known limitations and a safe uninstall path.
- [x] Produce same-key signed candidates for the implemented LAN slice (rc3/rc4/rc5).
      This does not approve a final release of all remaining product scope.
- [x] Obtain explicit approval/backups and perform the recorded own-phone/router pilot
      (rc3/rc4; rc5 phone-only). See core-migration.md. This is historical authorization,
      not permission to deploy newer development packages or future UI changes.
- [ ] Record pilot evidence, remaining blockers and acceptance before merge/tag/release.

**Accept:** no unresolved safety/security failures, green required CI and explicit rollout consent.

## Deferred (not blockers for the first strict Android release)

- Accessibility/light tier without Device Owner; never describe it as equally strong.
- Wireless-debugging self-provisioning, verified QR provisioning and distribution-policy work.
- OPNsense/pfSense adapters, iPhone, Home Assistant and unrelated traffic/i18n backlog.
- A maintainer-hosted service and an unrelated parallel web-dashboard implementation.
  Domain history remains planned under A8/A14. The shared administrator web interface
  under A20 is current scope, not a deferred second dashboard.

## Progress evidence

For each completed phase, append commit IDs, test commands/results, environment and
known limitations here. Never include real children's names, account addresses or secrets.

- Baseline: branch created from v0.9.1. Documentation only; no new feature tests run yet.
