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
- [ ] Submit the prompt to Claude Design and obtain the design export (user handoff).
- [ ] Review exported states, factual claims, accessibility and native LuCI compatibility.
- [ ] After design approval, implement Overview first and test in disposable LuCI.
- [ ] After Overview acceptance, apply the shared visual language to other existing pages.

No generated design or UI implementation is implied by the completed research/prompt tasks.

## P0 — Establish the implementation baseline

- [x] Create the branch from the merged v0.9.1 commit.
- [x] Write this backlog and link it from the product plan and roadmap.
- [ ] Update stale current-status sections in `CLAUDE.md` and the roadmap: DNS and
      enrollment are implemented; distinguish completed pilot evidence from remaining gates.
- [ ] Inventory shell APIs, policy wire format, keys, UCI options and persistent files;
      record which component owns each before and after migration.
- [ ] Record baseline fast test results, package versions and available test environments.
- [ ] Mark stale knowledge-graph entries explicitly; refresh after the architecture settles.

**Accept:** a reproducible baseline, no claim that old planning text describes current code.

## P1 — Resolve contracts and product semantics

Depends on P0. Discuss unresolved choices before building their enforcement.

- [ ] Specify signed device roles, parent-to-child scope, revocation and multiple parents.
      A normal parent phone must not need Device Owner merely to approve requests.
- [ ] Define precedence: essential apps, explicit blocks, downtime, per-app/category
      quotas, one-off grants, adult pool, and active pacts.
- [ ] Define whether grants bypass downtime/explicit blocks, and when unused minutes expire.
- [ ] Define per-device vs shared multi-device budgets; do not claim global offline
      budgets without an allocation strategy that prevents double spending.
- [ ] Define usage: foreground time, screen lock, split screen, picture-in-picture,
      background audio, overlapping activities and incomplete Android event history.
- [ ] Define household timezone/day boundaries, DST, clock rollback and travel behavior.
- [ ] Set request rate limits, outstanding-request bounds, expiry and repeated-denial UX (A7).
- [ ] Choose usage retention/deletion and visibility for child, parent and adult (A8).
- [ ] Decide initial Go/shell responsibility split and later migration boundary (A6).
- [ ] Choose off-home transport (A2): compare direct/VPN, relay and FCM wake-up options
      against self-hosting, delivery reliability, Doze, privacy and setup cost.
      FCM is not inherently incompatible with self-hosting; document actual credentials
      and service dependencies. Signing is not encryption.
- [ ] Specify who authorizes offline grants: router only, or delegated parent keys;
      cover revocation, trust expiry and offline replay/double-spend limits.
- [ ] Record schema examples, error codes and compatibility rules in a protocol document.

**Accept:** explicit decisions for the first vertical slice; deferred choices stay labeled.

## P2 — Verify Android capabilities with bounded experiments

Depends on the relevant P1 semantics. No child-phone experiments.

- [ ] Check current official Android API behavior for UsageStats, package suspension,
      package visibility, special access, foreground execution and install restrictions.
- [ ] Prove usage access grant/setup, detection of revocation and recovery. Treat adb
      app-ops setup as a candidate, not proof that Device Owner can self-grant access.
- [ ] Test suspend/unsuspend of one disposable app, failure returns, active-app behavior
      and non-suspendable/system packages; never blanket-suspend critical packages.
- [ ] Identify launcher, dialer/emergency access, input method, Settings recovery and
      FamilyWarden itself as safety cases with explicit allowlist rules.
- [ ] Verify package-added/replaced/removed events and the install-to-suspend race.
      Do not promise an unapproved app can never run until evidence supports it.
- [ ] Measure counting/enforcement latency and battery cost with screen on/off and Doze.
- [ ] Verify ordinary-account sign-in on an authorized disposable setup if still needed;
      do not reuse supervised-account failure as proof of ordinary-account success.
- [ ] Record results, device/API/build and known bypasses; remove test state afterwards.

**Accept:** a documented supported approach and honest limitations, without production changes.

## P3 — Bootstrap the Go core

Depends on P0/P1. Use the standard library first; add dependencies only for concrete needs.

- [ ] Create Go module, executable, configuration validation and clean shutdown.
- [ ] Separate pure domain rules from storage, API transport and the OpenWrt boundary;
      no adapter framework or speculative interfaces with no actual consumer.
- [ ] Choose durable storage based on atomicity, router flash use, backups and build size.
      Define schema versioning, crash recovery and bounded queues before writing data.
- [ ] Add restricted local service transport, health/version/capability reporting and
      redacted logs; do not expose an unauthenticated WAN listener.
- [ ] Add deterministic clocks and fixtures for policy, expiry and day-boundary tests.
- [ ] Add Go formatting, unit tests, race checks where supported and build jobs to CI.

**Accept:** a small runnable core with durable-state tests; legacy behavior still works.

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

## P7 — Enforce child app rules locally

Depends on P2/P5/P6.

- [ ] Add app inventory and stable package identity; treat labels/icons as untrusted UI data.
- [ ] Implement a pure evaluator for schedules, quotas and precedence from P1.
- [ ] Suspend/unsuspend only managed packages, record failures and reconcile at boot,
      package changes, policy updates and budget/schedule boundaries.
- [ ] Handle package replacement/uninstall, newly installed apps and essential-app exceptions.
- [ ] Protect relevant configuration, time, uninstall and user/profile changes where supported.
- [ ] Restore only FamilyWarden-owned restrictions on approved removal; never erase another
      administrator's policy. Preserve the existing signed removal and pact guards.
- [ ] Show exhausted quota, reason, remaining time and unavailable usage access honestly.

**Accept:** targeted runtime evidence for exhaustion, offline restart and essential-app safety;
unit tests cover precedence. No claim of root/recovery-proof enforcement.

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
