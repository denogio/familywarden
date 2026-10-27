# Router adapter portability — current audit and completion gates

Initial source audit: `c7d3a8d` (2026-10-27). Subsequent bounded implementation
slices are recorded below. This is not a second-router implementation or deployment report. It refines A18's acceptance criteria following
the user's clarification: adding another router must not require reimplementing
FamilyWarden product rules. See [migration evidence](core-migration.md),
[delivery backlog](app-control-todo.md) and [product decisions](app-control.md).

## Current scope and working method (user clarification)

OpenWrt is the only implementation target now. Other router types are future
possibilities, not current deliverables or build/test targets. Preserve the option
without designing a generic adapter framework. The earlier FreeBSD compile check
below is historical evidence only, not a workstream or a completion gate.

Moving product rules into core must not remove necessary adapter capabilities:
core defines policy meaning and precedence; the adapter may receive schedules,
prioritized rules and deadlines and choose the native enforcement mechanism.
Preserve OpenWrt's kernel-driven schedules and ticket expiry without a new polling
daemon. Apply one bounded migration at a time and verify current OpenWrt behavior.

Use the smallest test that proves each function: focused Go tests for product logic,
a focused adapter/subprocess or disposable-container check for the bridge, and a
module/component test for UI changes. Full e2e/emulator runs are milestone checks,
not the default after each function. Expand only for a concrete affected boundary,
a demonstrated regression or a larger integrated change; do not repeat already
covered suites reflexively.

## What “core migration done” means

A new router integration provides platform facts, applies core-owned network intent,
reports application/failure state, and supplies native service/authentication/UI
integration. It must not implement profile policy, schedule/pact semantics, grants,
bonus limits, DNS policy or app decisions again. nftables versus pf is an enforcement
mechanism difference, not a second product implementation.

**Not done today.** Go-owned configuration on OpenWrt is a substantial completed
slice, but it is not yet a narrow, independently usable adapter boundary. No percentage
is assigned: implemented logic, portability, validation and deployment are separate.
A shell-based firewall renderer may remain in the OpenWrt adapter; rewriting it in
Go is not itself an acceptance criterion. No speculative FreeBSD adapter/framework
is required to finish this boundary.

## Current progress

| Workstream | Current implementation | Portability status |
|---|---|---|
| Identity, enrollment, roles/scopes, signatures, requests/grants, usage/new-app ledgers | Go `internal/identity`, `internal/protocol`, `internal/store`, `internal/usign`; fake adapters test product behavior | Substantial portable implementation; runtime still uses the broad adapter context and home-ticket bridge |
| Household configuration and pacts | Go `internal/configuration` + `internal/rules/configuration.go`: explicit import, CAS, confirm/rollback, effective snapshots, amendment/release, cumulative DNS minimum | Implemented after opt-in import, not the production default; policy/context still depend on shell output |
| Phone/app policy | Go `PhonePolicy`, `AppContext` and app compilers derive owned configuration/frozen rules and core-owned bonus | Mixed boundary: shell still supplies other policy-shaped runtime input and verified DNS data |
| Bonus | Owned-mode validation, durable runtime entries, day/cap/pact decisions and policy/list readers now use core; legacy mode retains its explicitly gated writer | First R1 slice implemented/tested; no active owned-mode shell bonus writer |
| Manual network actions/grant repair | Owned block/allow/reset, additive-ticket and approved-grant eligibility now run in Go; the adapter stores RAM state and applies native rules under shared locks | Action/repair slice implemented; shell status/schedule evaluation, expiry cleanup and pact-projection allow clearing remain R1b/R3 work |
| DNS reconciliation | Go `internal/agh` owns desired client/access reconciliation and verification; owned desired DNS comes from the document | Shell still orchestrates credentials, dirty/retry scheduling, acknowledgements and persisted ownership state |
| Platform enforcement and administration | Go OpenWrt adapter + shell nft/fw4 renderer; LuCI rpcd/session/ACL bridge; shared Requests UI | Working OpenWrt integration, not a platform-neutral enforcement contract or complete shared administration UI |
| Scope | OpenWrt only; preserve future options | No second-router implementation or cross-platform build workstream |

## Concrete boundary debts (source evidence)

Paths below are repository-relative. “Owned mode” means `configuration.json` exists
and the explicit authority cutover has completed.

| Boundary | Evidence | Why a new adapter is not simple yet |
|---|---|---|
| Broad interim adapter | `core/internal/platform/platform.go`: `Context`, `Policy`, `Configuration`, `WithContext`, `RouterKey`, legacy phone import/removal, `EnsureTicket` | Mixes legacy import, product-policy-shaped input, locking, key access and network side effects. Existing methods are an interim contract, not the final implementation checklist for every router |
| Extra apply contract | `core/internal/configuration/authority.go`: `Backend.ConfigurationTransaction`, `Backend.NetworkTransaction`; `core/internal/openwrt/configuration.go`, `network.go` | Configuration apply/recovery and volatile network mutation exist outside `platform.Adapter`. The callback currently carries raw configuration plus a synchronous apply function; a new backend still has to understand how effective intent and runtime facts are assembled |
| Shell required even after import | `Authority.Context`, `Authority.Policy`, `Authority.WithContext` in `authority.go` call the wrapped adapter; `openwrt.go` calls `core-context`/`core-policy` | The owned document does not remove the shell round trip. `AppContext` still requires per-profile runtime entries and matching pact facts (bonus now comes from core-owned state); `PhonePolicy` checks shell captain/override, and preserves the verified DNS input. Split these into explicit runtime/application facts, not another policy compiler |
| Remaining product writers/readers | Shell `effective_override`, `gc_expired_tickets`, `_clear_allow` and status helpers; public bonus/network writers now forward in owned mode | Bonus, action and grant-repair decisions are now Go-owned. Shell still clears captain permits during pact projection and evaluates status/expiry; those consumers remain to migrate. Preserve RAM-versus-durable semantics |
| Status and network compilation | Shell `_now_match_schedule`, `profile_effective_status`, `gen_*`, `generate_include` | Status evaluation and precedence are mixed with nft syntax. Core must define precedence/allow windows/network intent; adapter must render/apply it without independently deciding whether a profile deserves access |
| Grant-to-network bridge | Owned `Authority.EnsureTicket` uses shared Go decisions and `Backend.NetworkTransaction`; legacy `cli_core_ticket` refuses owned mode | Owned repair no longer asks shell to decide eligibility: it never shortens, replaces a manual block or opens a pact. The remaining native bridge owns RAM storage, locks and application acknowledgement only; legacy import/path retirement remains separate |
| DNS lifecycle/state | Shell `_dns_sync_locked`, `cli_dns_tick`, `_json_dns`; Go `internal/agh`, `configuration/dns.go` | The reconciler is portable, but a new router still needs more than an AGH HTTP client. Separate core-owned acknowledgement/ownership semantics and retry orchestration from secret access, storage paths and service wakeups. Never advertise desired DNS as verified enforcement |
| Ulysses recovery entry point | Go `PactOperation` handles owned release/cancel timing; shell `cli_sail_*` and `www-familywarden/cgi-bin/sail` handle status, root authentication, answer check and throttling | Release timing is substantially migrated; protected entry-point/phrase semantics and transport/auth plumbing remain mixed. Preserve a distinct protected release route, not an ordinary administrator bypass |
| OpenWrt layout and locks | `openwrt/configuration.go`: fixed ownership path, UCI/effective/pact projections, DNS dirty marker; `transaction.go`: inherited file locks; shell uses `/proc/self/fd/3` | These mechanisms belong in the OpenWrt adapter. New adapters must satisfy consistency/recovery guarantees, not emulate these paths, Linux descriptor checks or UCI files. The adapter already writes the DNS dirty marker; it is not missing from Go |
| Executable composition | `core/cmd/familywardend/main.go` directly constructs `openwrt.Adapter`; OpenWrt paths are defaults | Compiling for FreeBSD still selects OpenWrt. Separate reusable service composition from platform selection/layout, without a general plugin framework |
| UI/API surface | Existing Profiles/Settings use core CAS but LuCI CBI/UCI as a local form projection; overview/actions and sail use OpenWrt endpoints. Shared module: `luci-app-familywarden/htdocs/luci-static/resources/familywarden/shared/family.js` | A new platform cannot simply mount the complete administration UI yet. Finish shared controls on portable core APIs; host adapters retain native authentication/session/ACL and transport integration |
| Restore/upgrade boundary | Core state spans configuration, devices, request/grant and usage documents; keys and AGH state are additional files | Configuration export alone is not a complete transferable household backup. A19's single backup artifact remains a goal; identity, spent grants, pact minima and enforcement acknowledgements must not be replayed or silently weakened |

## Target responsibilities, not a premature API design

**Portable core owns:** configuration/validation; profile/schedule/pact semantics;
manual overrides, additive ticket and bonus decisions; roles/grants/usage; signed
phone replies; desired DNS and verified reconciliation state; intended versus applied
status; safe recovery. A platform error cannot be reported as successful application.

**Router adapter owns:** LAN/lease/interface discovery and timezone facts; mapping
profile devices to that platform's enforcement identifiers; native firewall compilation,
validation, atomic activation/removal and applied/error reporting; OS secret/lifecycle
integration and platform-specific migration. Platform facts must not smuggle in a
second profile/pact/app policy authority.

**Host/UI adapter owns:** native authentication, session/ACL integration, protected
release entry point, and transport to the same core API/shared presentation. No new
standalone password, port or framework is implied.

Time-dependent enforcement is part of the contract: schedules, overnight/`24:00`
boundaries, ticket expiry, amendment/lockdown exceptions and timezone changes must
retain their meaning. OpenWrt can keep native kernel schedule rules. Another platform
may need a different enforcement mechanism, but must not invent schedule/precedence
semantics or require network polling to keep Android policy working. pf capability,
address matching and schedule mechanisms still need actual platform validation;
“nftables to pf” is not assumed to be a mechanical translation.

## Ordered implementation backlog

These are migration tasks, not new product scope. Reuse the existing core models,
CAS/recovery journal, test adapters and Go AGH reconciler. Introduce only the narrow
contracts needed by each migrated caller.

- [x] **R0 — Audit the real boundary and reconcile status.** This document records
  active owned-mode dependencies as well as legacy-only compatibility paths.
- [ ] **R1 — Move remaining runtime product state/actions to core.** Migrate bonus,
  block/allow/reset, manual additive tickets, expiry and eligibility checks with their
  writers/readers/locks together. Preserve grant-ticket behavior, pact restrictions,
  day boundaries, failure rollback and existing persistence semantics. Do not leave
  duplicate active writers in owned mode. Legacy routing may remain explicitly gated.
  - [x] R1a: core-owned bonus (core 0.13.0-r10 / backend rc10-r7), including one-time
    legacy import/owned upgrade and policy/list readers; focused tests below.
  - [ ] R1b: finish manual network state and expiry/status consumers.
    - [x] Owned manual block/allow/reset/ticket and grant-repair decisions plus native
      transaction/rollback bridge (core r11/backend rc10-r8; evidence below).
    - [ ] Move remaining pact-projection permit clearing and status/expiry product
      decisions; distinguish the core decision from native rendering/cache cleanup.
- [ ] **R2 — Replace policy-shaped adapter reads with runtime facts.** After R1, owned
  identity/request/policy paths must run without `core-context`/`core-policy` or UCI.
  Keep legacy import behind the OpenWrt compatibility path; root-private keys and
  verified application facts must still be available without weakening trust.
- [ ] **R3 — Define portable network intent and applied-state contract.** Move common
  status/precedence/schedule decisions out of the nft renderer, then adapt the current
  OpenWrt compiler. Include device mappings, LAN/default policy, router-access control,
  lockdown/recovery exceptions, deadlines, revisions and atomic apply failure behavior.
  Rendering native schedules is allowed; reimplementing product eligibility is not.
- [ ] **R4 — Complete DNS and protected-release orchestration.** Reuse Go reconciliation
  and owned pact operations. Move remaining portable state/decisions out of shell while
  retaining native secret access, authenticated release isolation, verification-before-
  ack, retry safety and a recovery entry point reachable during lockdown. Depends on
  R1/R2 for consistent runtime facts; use R3's application/error semantics where needed.
- [ ] **R5 — Separate daemon composition and finish portable administration.** Select
  platform construction outside product services; complete core API coverage and shared
  views/editors with thin host bridges (A20/P10). Native LuCI form projection may stay
  during migration, but cannot be required by the shared product UI. Depends on the
  relevant R1–R4 operations, not an artificial all-at-once UI rewrite.
- [ ] **R6 — Prove the boundary with a test adapter, not a second router.** Run the production core service with
  an in-process recording/failing adapter, no shell/UCI/ubus/fw4 or OpenWrt paths. Test
  the same identities/configuration, schedules/pacts, manual actions/grants/bonus,
  DNS acknowledgements, recovery and shared UI contract. Reuse existing tests; add
  missing cross-boundary scenarios rather than duplicate covered unit cases.
- [ ] **R7 — Close migration/release gates separately.** Complete P12's mixed-version,
  import/upgrade/interruption and safe backup/restore matrix before removing legacy
  paths or declaring production cutover ready. Deployment still needs authorization.

R1 continues with the remaining pact-projection/status/expiry consumers after the
owned bonus and network-action slices. Inventory the relevant callers before changing state
ownership; do not add an unused parallel store. Exact function signatures and storage
changes are implementation decisions to review against existing behavior, not settled
by this planning document.

## R1a evidence: core-owned bonus (development, not deployed)

The existing household journal now stores bonus runtime entries separately from
editable/sealed configuration. Import and upgrading an existing owned document
capture the legacy file once under the configuration transaction lock, including
future-dated rows. Core owns validation, 1–240 minutes per call, the 1440-minute
per-target/day cap, additive total/app entries, router-local day selection, pruning
on mutation, active-captain refusal and persistence. Configuration revision is not
changed by bonus; configuration confirmation/rollback and pact operations retain it.
Bonus writes are refused while configuration confirmation is pending.

Owned `bonus` RPC calls forward original JSON to core, including strict duplicate-field
validation; legacy calls keep the previous implementation. A legacy writer waking
from the state lock rechecks ownership before writing. Phone/context compilation
ignores shell-provided bonus, and `list.bonus_today` reads versioned core totals
before taking the shell lock (no recursive socket/lock dependency). Core-down reads
and writes fail visibly instead of returning zero or falling back to the legacy file.
OpenWrt retains native firewall enforcement; bonus needs no firewall apply.

The private `bonus-state-v1` handshake distinguishes an empty import from a missing
backend feature. Upgrade core/backend together. Older cores reject the added runtime
journal member; no automatic downgrade or restore safety is claimed. The old file
is left untouched as compatibility/backup input, not an active owned-mode store.

Validation: focused Go bonus/app-context tests with race detection, the affected
configuration transaction/recovery tests and rules/adapter tests, plus
`FW_TEST_FOCUS=bonus sh scripts/test-integration.sh` (13 real OpenWrt bonus bridge
checks). Covers import, upgrade, independent targets/caps, concurrency, day rollover,
stale legacy state, duplicate fields, persistence failure, restart, pact refusal,
rollback and core-down behavior. No full e2e, emulator, package-build matrix or
production deployment was run for this slice. R1 as a whole remains open.

## R1b action/repair evidence (development, not deployed)

Owned public `block`, `allow`, `reset` and `ticket` RPCs now forward original JSON to
core, preserving their existing external names. Core validates requests and decides
pact eligibility, additive absolute ticket deadlines, replacement of an override and
approved-grant repair precedence. `Authority.EnsureTicket` no longer invokes the
legacy shell eligibility/writer path in owned mode. Pending configuration confirmation
refuses network mutations; legacy mode remains explicitly routed to the old bridge.

`Backend.NetworkTransaction` exposes a locked volatile snapshot and an apply callback.
The OpenWrt adapter keeps the existing `/tmp/familywarden.state` format: it is platform-
backed RAM, not a second durable configuration store. Daemon restart retains state;
router reboot clears it, as before. Configuration revision/export is unaffected.
Other profiles/lines are preserved, and duplicate/malformed affected override rows
are refused.
The tagged `network-actions-v1` bridge uses the same ticket-before-state lock order
as configuration and participating legacy writers. Native apply inherits the held
lock descriptors; it makes no recursive socket call. Legacy writers recheck ownership
before writing after a lock wait; direct legacy grant-ticket writes refuse owned mode.

An application failure attempts to restore prior RAM and native enforcement and still
reports the requested action as failed. After a failed apply, core conservatively
blocks further policy/mutation service until restart reconciliation, even if rollback
was acknowledged; restoration is not a
retry that turns the failed request into success. Existing fw4/compiler limitations
(including its chain-reload window) are not declared solved or replaced by an atomic
portable dataplane. Native schedules and nft timeout sets remain unchanged; no new
polling daemon is introduced. Status now shares list's read-time expiry normalization,
so an elapsed ticket is not reported as an unlimited allow.

Focused validation: Go/race decision/authority, inherited-lock, rollback and parser
checks; affected configuration/adapter tests and vet; `FW_TEST_FOCUS=network sh
scripts/test-integration.sh` passes 34 real OpenWrt checks. These cover legacy/cutover,
native block/allow/reset, additive deadlines, no permanent ticket escape in either
chain, rejected duplicate fields/private unlocked apply, daemon restart, injected
validation failure with prior RAM/enforcement restored, kernel expiry without polling,
expired status/list, pact guards and core-down refusal without RAM writes. Product
unit tests also cover grant retry/longer/permanent permission, manual-block precedence,
concurrent tickets, integer bounds/overflow, incompatible adapters and pending edits.
No full e2e, Android emulator, package build matrix or deployment for this slice.

R1b is not closed: shell still owns pact-projection `_clear_allow`, status/schedule
evaluation and expiry/cache cleanup. The broad policy-shaped context and full portable
network-intent/applied-state contract remain R2/R3. Do not describe the whole network
migration or adapter boundary as finished.

## Acceptance checklist for adapter-ready completion

- [ ] A router implementer needs platform facts, enforcement/application and host
  integration only; no second implementation of product decisions or store schemas.
- [ ] Production core and shared UI work with the recording/failing adapter; owned
  services do not call policy-producing shell commands. Missing facts fail closed.
- [ ] OpenWrt still passes real fw4/nft and authenticated UI/package tests against the
  new contract, including concurrent actions, schedule boundaries and apply failure.
- [ ] Legacy import/format support is isolated, versioned and removable after parity;
  a new router is not required to implement OpenWrt's legacy import/lock protocol.
- [ ] Intended, verified/applied and unavailable/stale states remain distinct for both
  network and phone/DNS policy. Restart/offline operation cannot loosen a pact.
- [ ] Build portability, migration safety and actual platform runtime support are
  reported separately. No pfSense/OPNsense support claim until its adapter is tested.

## Evidence and explicit limits

Already implemented/tested before this audit: Go unit/race/vet coverage, real OpenWrt
integration (487 checks at the latest recorded local checkpoint), real AGH tests (92),
package install/remove smoke tests, 65 legacy-phase browser checks plus three owned
checks with freshly built packages (`41f309b`), then all four owned checks including
lost-confirmation recovery (`c7d3a8d`). Android diagnostic changes passed 199 JVM tests,
lint and debug compilation. Counts are checkpoint evidence, not portability completion.

The initial audit also cross-compiled `familywardend` for `GOOS=freebsd GOARCH=amd64`
with `CGO_ENABLED=0`. It still constructs the OpenWrt adapter: no FreeBSD runtime,
firewall behavior, packaging or UI integration was tested. That initial audit changed
no implementation; the subsequent bonus implementation is recorded above. Neither
work accessed a production router or phone.

CI `40490235715` targets `41f309b`, not the later fourth browser test. At this audit
checkpoint its Go, router test, package builds, Android build and LuCI jobs passed;
the grouped Android emulator job was still running. The earlier emulator
failure remains unexplained; diagnostics and an unchanged successful local rerun
are not a root-cause fix or a blanket reliability claim.
