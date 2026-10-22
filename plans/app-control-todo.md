# App control — implementation backlog

Branch: `feature/app-control`, based on `v0.9.1` (`dce92f4`).
Status: implementation authorized; no app-control feature code written yet.
Product decisions and research: [app-control.md](app-control.md).

This is the durable checklist. Complete a box only after its acceptance evidence is
recorded. Work in the order below, with small reviewable commits. Open choices are
not permission to silently decide product behavior.

## Scope and boundaries

- Go core, OpenWrt first; preserve a boundary for future OPNsense/pfSense adapters.
  Do not implement speculative FreeBSD adapters now.
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

## Baseline and migration inventory

Existing implementation to read before moving behavior:

| Responsibility | Current source / state |
|---|---|
| Firewall, enrollment, pact, AGH sync | `familywarden/usr/lib/rpcd/familywarden` |
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

User-supplied design is archived; Overview and its follow-up refinements are implemented
and visually accepted. Next: Profiles, then Phones, Settings and Info. Playwright is
paused at the user's request; latest refinements have syntax checks and manual preview,
not a completed final browser suite. App-control enforcement is still not implemented.

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

- [ ] Add architecture-specific Go package builds, procd lifecycle and configuration.
      The existing shell package is `PKGARCH:=all`; a compiled core cannot be.
- [ ] Connect authenticated LuCI/ubus calls through a narrow local bridge and least-privilege ACLs.
- [ ] Keep the existing fw4 compiler and sail endpoint authoritative initially unless
      the migration contract explicitly assigns them elsewhere.
- [ ] Define reload ordering, failures, timeouts and single-writer ownership per state file.
- [ ] Add clean install, upgrade, restart and uninstall tests with preserved configuration.
- [ ] Measure package/RAM/storage cost on target architectures; document minimum requirements.

**Accept:** disposable OpenWrt installation and reboot work without DNS/pact/firewall regressions.

## P5 — Extend identity and signed policy

Depends on P3/P4 and P1 protocol decisions.

- [ ] Reuse current full-key fingerprint approval and per-device request authentication.
- [ ] Preserve pinned router identity and existing phone keys during migration.
- [ ] Add scoped roles, app rules, schema/capability negotiation and policy revisions.
- [ ] Bound payloads, verify signed fields before parsing into trusted state, and reject
      unknown unsafe versions, mismatched devices, replay and downgrade.
- [ ] Persist last trusted policy atomically; retain it offline without a 24/48-hour
      escalation. Separate policy freshness from fresh authorization for removal/grants.
- [ ] Add applied-state acknowledgements; fetching policy is not evidence of enforcement.
- [ ] Address same-key router-address repair without accepting redirects or replacing pins.
- [ ] Test pending enrollment, changed pending keys, revoked roles and old Android clients.

**Accept:** mixed-version integration tests and a documented rollback/compatibility matrix.

## P6 — Collect usage privately and durably

Depends on P2/P5 and chosen accounting rules.

- [ ] Implement Android usage-event ingestion with persisted cursor and bounded overlap
      to handle duplicate events, process death and reboot without double counting.
- [ ] Track per-app/day locally, reconcile active sessions and handle missing history
      honestly; missing permission is not zero usage.
- [ ] Keep local counters/enforcement usable offline; throttle writes for flash/battery.
- [ ] Upload signed, sequenced summary updates idempotently; prevent duplicates and
      out-of-order retries from increasing/decreasing totals incorrectly.
- [ ] Enforce per-device ownership and reject impossible or oversized usage submissions.
      A signed report authenticates its source, not truth on a compromised device.
- [ ] Apply chosen retention, deletion and access controls on phone and router.
      No browsing history, account identifiers or raw event streams by default.
- [ ] Test midnight/DST, clock changes, offline days, reinstall, gaps and queue overflow.

**Accept:** known synthetic event traces yield exact expected summaries and retries do not drift.

Local foundation implemented/tested 2026-10-21: `UsageCounter`, `UsageJournal` and
`UsageMonitor` in Android main sources; debug-only fixture activation. 110 JVM tests and
the emulator usage/restart/update/revocation/reboot smoke test pass. This does **not**
complete the checkboxes: public class identity cannot distinguish same-class instances,
PiP remains unverified, clock/history recovery and throttling need work, and signed
activation/uploads/router retention await P5. See capability findings and
`test/android/install-probe/README.md`.

## P7 — Enforce child app rules locally

Depends on P2/P5/P6.

- [ ] Add app inventory and stable package identity; treat labels/icons as untrusted UI data.
- [ ] Implement a pure evaluator for schedules, quotas and precedence from P1.
- [ ] Suspend/unsuspend only managed packages, record failures and reconcile at boot,
      package changes, policy updates and budget/schedule boundaries.
- [ ] Handle package replacement/uninstall, newly installed apps and essential-app exceptions.
      Model decided (A12): suspend new user apps on the dynamic package event (needs
      package visibility) plus boot/periodic reconcile; Play installs/updates stay on.
      UI and docs must state the brief first-launch window, never "blocked before use".
- [ ] Protect relevant configuration, time, uninstall and user/profile changes where supported.
- [ ] Restore only FamilyWarden-owned restrictions on approved removal; never erase another
      administrator's policy. Preserve the existing signed removal and pact guards.
- [ ] Show exhausted quota, reason, remaining time and unavailable usage access honestly.

**Accept:** targeted runtime evidence for exhaustion, offline restart and essential-app safety;
unit tests cover precedence. No claim of root/recovery-proof enforcement.

Local foundation implemented/tested 2026-10-21: pure `appAction` and `AppEnforcer`
(main sources), exercising a local per-app quota with essential-package safety and
owned-only suspension tracking. Debug fixture exhaustion/update/reboot passes. Full
precedence/grants/shared quotas, signed policy activation, release/removal integration,
UI and physical-device safety remain open. Post-install suspension is **not** strict
pre-execution app approval (P2 round 3).

## P8 — Deliver the first end-to-end extra-time slice

Depends on P5–P7. Start on LAN before adding the selected off-home transport.

- [ ] Child requests minutes for one app/day with a unique request ID and expiry.
- [ ] Parent sees pending requests for authorized children and approves/denies in Android.
- [ ] Verify role, scope, limits and signed authorization; never widen the weekday budget.
- [ ] Persist grants and consumption; make double taps, retries, concurrent parents and
      duplicate delivery idempotent. Expired/revoked requests must not revive.
- [ ] Apply one grant exactly once on the intended phone; report received vs applied states.
- [ ] Enforce request abuse limits and show expired/denied/offline states clearly.
- [ ] Test wrong child/parent/day/app, replay, tampering, restart and grant exhaustion.

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

- [ ] Parent Android: children/devices, app inventory, rules, pending requests, usage/day
      summaries and connection/applied-policy status.
- [ ] Child Android: remaining budgets, blocked reasons, request status and usage visibility.
- [ ] LuCI: configure rules/roles, view usage and requests, inspect capability/failure states.
- [ ] Include new-app approval, essential-app warnings and removal/revocation workflows.
- [ ] Add all English/Danish Android strings/plurals, accessibility labels, large-text,
      dark/light themes and sanitized previews/screenshots.
- [ ] Add mapper/unit tests and focused Playwright coverage for actual backend responses.

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
      not loosen it. Review current DNS snapshot-plus-live union before reusing it.
- [ ] Test amendment windows, expiry, scheduled boundaries, offline use and approved removal.

**Accept:** integration tests demonstrate no new self-approval or DNS/app escape path.

## P12 — Migrate existing DNS/identity responsibilities safely

Depends on P4/P5/P11. Move only responsibilities assigned to Go; this is not a rewrite mandate.

- [ ] Preserve root-private secrets, approved devices, router keys, ClientIDs, ownership
      records, DNS acknowledgements and all pact snapshots.
- [ ] Port AGH reconciliation with the existing real-container tests: own clients only,
      preserve foreign settings, no empty-allowlist lockout, verify changes before ack.
- [ ] Preserve strict DNS/no automatic unfiltered fallback and handset applied/probe status.
- [ ] Add idempotent migration, backup and interruption tests; stop dual writers.
- [ ] Define rollback limits after schema changes; never restore a backup that silently
      loosens an active pact or replays already-spent grants.
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
- [ ] Produce a signed candidate using the existing Android signing key and package ID.
- [ ] Get explicit approval for a one-device pilot and separate router/AGH deployment;
      take private backups, verify active pacts and agree on rollback first.
- [ ] Record pilot evidence, remaining blockers and acceptance before merge/tag/release.

**Accept:** no unresolved safety/security failures, green required CI and explicit rollout consent.

## Deferred (not blockers for the first strict Android release)

- Accessibility/light tier without Device Owner; never describe it as equally strong.
- Wireless-debugging self-provisioning, verified QR provisioning and distribution-policy work.
- OPNsense/pfSense adapters, iPhone, Home Assistant and unrelated traffic/i18n backlog.
- Per-site history, a maintainer-hosted service and a new web dashboard.

## Progress evidence

For each completed phase, append commit IDs, test commands/results, environment and
known limitations here. Never include real children's names, account addresses or secrets.

- Baseline: branch created from v0.9.1. Documentation only; no new feature tests run yet.
