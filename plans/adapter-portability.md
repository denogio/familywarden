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

**The owned service boundary is now demonstrated by R6.** Production composition and
the shared UI run against an in-process recording/failing adapter without OpenWrt.
R7 migration/release safety remains open; this is not production-cutover readiness.
Implemented logic, portability evidence, migration validation and deployment are separate.
A shell-based firewall renderer may remain in the OpenWrt adapter; rewriting it in
Go is not itself an acceptance criterion. No speculative FreeBSD adapter/framework
is required to finish this boundary.

## Current progress

| Workstream | Current implementation | Portability status |
|---|---|---|
| Identity, enrollment, roles/scopes, signatures, requests/grants, usage/new-app ledgers | Go `internal/identity`, `internal/protocol`, `internal/store`, `internal/usign` | Owned production composition uses facts and core-owned grant repair; R6 verifies signed routes and persistent ledgers without the legacy policy/ticket bridge |
| Household configuration and pacts | Go `internal/configuration` + `internal/rules/configuration.go`: explicit import, CAS, confirm/rollback, effective snapshots, amendment/release, cumulative DNS minimum | Implemented after opt-in import, not the production default; runtime facts and native host projections remain adapter-backed |
| Phone/app policy | Go `PhonePolicy`, `AppContext` and app compilers derive owned configuration/frozen rules and core-owned bonus | Owned policy is document-derived; the adapter supplies only explicit runtime facts, including verified DNS data |
| Bonus | Owned-mode validation, durable runtime entries, day/cap/pact decisions and policy/list readers now use core; legacy mode retains its explicitly gated writer | First R1 slice implemented/tested; no active owned-mode shell bonus writer |
| Manual network actions/grant repair | Owned block/allow/reset, additive-ticket and approved-grant eligibility now run in Go; the adapter stores RAM state and applies native rules under shared locks | Action/repair, owned pact-permit revocation and the status/expiry report are Go-owned; owned native rendering consumes the core's ordered `network-intent-v1` decisions; shell UCI rule generators are legacy-only |
| Status/expiry reporting | Go `configuration/status.go` evaluates override precedence, allow windows, read-time expiry, pact captain details and bonus totals from one locked runtime snapshot; the adapter reads the volatile state under the shared lock | Owned `list`/`status` RPCs forward to `configuration_report`; legacy shell status remains until R3/native retirement |
| DNS reconciliation | Go `internal/agh` owns desired client/access reconciliation and verification; owned desired DNS comes from the document | Owned core persists acknowledgements/ownership/retry state; native secrets and pre-import compatibility remain in OpenWrt |
| Platform enforcement and administration | OpenWrt nft renderer + native apply; one LuCI mount/RPC bridge; canonical shared workspace in `web/` | Production `service.New` and shared UI pass the R6 recording/failing-adapter proof; real native enforcement remains OpenWrt-only |
| Scope | OpenWrt only; preserve future options | No second-router implementation or cross-platform build workstream |

## Native and compatibility boundaries (source evidence)

Paths below are repository-relative. “Owned mode” means `configuration.json` exists
and the explicit authority cutover has completed.

| Boundary | Evidence | Why a new adapter is not simple yet |
|---|---|---|
| Broad interim adapter | `core/internal/platform/platform.go`: `Context`, `Policy`, `Configuration`, `WithContext`, `Facts`, `FactsContext`, `RouterKey`, legacy phone import/removal, `EnsureTicket` | `Context`/`Policy`/`WithContext` are now the legacy policy path only; owned mode uses the explicit `Facts`/`FactsContext` reads. The broad surface stays an interim contract, not the final implementation checklist for every router |
| Extra apply contract | `core/internal/configuration/authority.go`: `Backend.ConfigurationTransaction`, `Backend.NetworkTransaction`; `core/internal/openwrt/configuration.go`, `network.go` | Configuration apply/recovery and volatile network mutation exist outside `platform.Adapter`. Callbacks now receive raw volatile overrides and apply a core-computed `NetworkIntent` alongside the compatibility configuration projection. The core supplies ordered decisions; adapters must preserve synchronous apply/rollback guarantees without reproducing eligibility |
| OpenWrt's native facts transport | `Authority.Context`, `Authority.Policy`, `Authority.WithContext` call `Facts`/`FactsContext` in owned mode; legacy mode keeps `core-context`/`core-policy` | The owned document no longer pulls policy-shaped shell output: `AppContext` checks profile presence only, `PhonePolicy` checks the platform captain and reads the volatile override/verified DNS facts. The legacy shell path remains the policy owner until R3/native retirement |
| Remaining product writers/readers | Shell `gc_expired_tickets`, legacy status helpers and `_clear_allow` are legacy-only | Owned bonus, action, grant-repair, pact-permit revocation and status/expiry consumers are Go-owned. Native expiry cleanup stays with the kernel/adapter. Preserve RAM-versus-durable semantics |
| Status and network compilation | Core `configuration/intent.go` compiles ordered decisions; OpenWrt `intent.go` renders nft syntax; shell `generate_intent_include` discovers LAN interfaces and stages validation | Status evaluation and precedence are core-owned and shared by the report. Owned rendering consumes ordered forward/router decisions, deadlines, assigned MACs and the core-selected lockdown. It does not inspect profile eligibility, UCI, pact files or volatile overrides. Legacy generators remain pre-import only |
| Grant-to-network bridge | Owned `Authority.EnsureTicket` uses shared Go decisions and `Backend.NetworkTransaction`; legacy `cli_core_ticket` refuses owned mode | Owned repair no longer asks shell to decide eligibility: it never shortens, replaces a manual block or opens a pact. The remaining native bridge owns RAM storage, locks and application acknowledgement only; legacy import/path retirement remains separate |
| DNS lifecycle/state | Shell `_dns_sync_locked`, `cli_dns_tick`, `_json_dns`; Go `internal/agh`, `configuration/dns.go` | Owned lifecycle now lives in `configuration/dns_runtime.go`; the native DNS transaction provides secrets/one-time legacy import under its writer lock. Native tick only wakes the core retry decision. Desired DNS is never advertised as verified enforcement |
| Ulysses recovery entry point | Go `PactOperation` handles owned release/cancel timing; shell `cli_sail_*` and `www-familywarden/cgi-bin/sail` handle status, root authentication, answer check and throttling | Core `Recovery` owns phrase/status/timing behind a dedicated route; native CGI retains password authentication and throttling. Ordinary administrator API routing refuses protected release methods |
| OpenWrt layout and locks | `openwrt/configuration.go`: fixed ownership path, UCI/effective/pact projections; `transaction.go`: inherited file locks; shell uses `/proc/self/fd/3` | These mechanisms belong in the OpenWrt adapter. New adapters must satisfy consistency/recovery guarantees, not emulate these paths, Linux descriptor checks or UCI files. Owned DNS uses a desired/credential fingerprint, not the legacy DNS dirty marker |
| Executable composition | The executable constructs OpenWrt; `core/internal/service.New` composes the portable product services | R5/R6 separate and exercise production composition with no OpenWrt dependency. The actual executable still intentionally selects the sole supported platform; no provider/plugin framework was added |
| UI/API surface | Canonical `web/` workspace, core APIs and thin LuCI session/ACL/RPC bridge; no legacy CBI views | R6 mounts the unchanged production UI against the real portable service, with no mocked replies or LuCI globals. Native authentication/session/ACL and protected-recovery entry points remain host responsibilities |
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
- [x] **R1 — Move remaining runtime product state/actions to core.** Migrate bonus,
  block/allow/reset, manual additive tickets, expiry and eligibility checks with their
  writers/readers/locks together. Preserve grant-ticket behavior, pact restrictions,
  day boundaries, failure rollback and existing persistence semantics. Do not leave
  duplicate active writers in owned mode. Legacy routing may remain explicitly gated.
  - [x] R1a: core-owned bonus (core 0.13.0-r10 / backend rc10-r7), including one-time
    legacy import/owned upgrade and policy/list readers; focused tests below.
  - [x] R1b: finish manual network state and expiry/status consumers.
    - [x] Owned manual block/allow/reset/ticket and grant-repair decisions plus native
      transaction/rollback bridge (core r11/backend rc10-r8; evidence below).
    - [x] Owned pact-permit revocation in core (core r12/backend rc10-r9), including
      failed-apply RAM undo and a post-lock legacy cutover guard; focused evidence below.
    - [x] Owned status/expiry report in core (core r13/backend rc10-r10): precedence,
      allow-window evaluation, read-time expiry, pact details and bonus from one locked
      runtime snapshot; `list`/`status` forward to `configuration_report`. Evidence below.
- [x] **R2 — Replace policy-shaped adapter reads with runtime facts.** After R1, owned
  identity/request/policy paths must run without `core-context`/`core-policy` or UCI.
  Keep legacy import behind the OpenWrt compatibility path; root-private keys and
  verified application facts must still be available without weakening trust.
    - [x] Owned `Context`/`Policy`/`WithContext` and `configuration_bonus_read` now read
      the versioned `core-facts` envelope (`runtime-facts-v1`): handshakes, timezone,
      `pact_captain`/`mast_captain`, profile presence and per-profile volatile facts
      (manual override, verified DNS). No policy content crosses the boundary; the
      authoritative document supplies it (core r14/backend rc10-r11; evidence below).
- [x] **R3 — Define portable network intent and applied-state contract.** Move common
  status/precedence/schedule decisions out of the nft renderer, then adapt the current
  OpenWrt compiler. Include device mappings, LAN/default policy, router-access control,
  lockdown/recovery exceptions, deadlines, revisions and atomic apply failure behavior.
  Rendering native schedules is allowed; reimplementing product eligibility is not.
- [x] **R4 — Complete DNS and protected-release orchestration.** Reuse Go reconciliation
  and owned pact operations. Move remaining portable state/decisions out of shell while
  retaining native secret access, authenticated release isolation, verification-before-
  ack, retry safety and a recovery entry point reachable during lockdown. Depends on
  R1/R2 for consistent runtime facts; use R3's application/error semantics where needed.
- [x] **R5 — Separate daemon composition and finish portable administration.** Select
  platform construction outside product services; complete core API coverage and shared
  views/editors with thin host bridges (A20/P10). Native LuCI form projection may stay
  during migration, but cannot be required by the shared product UI. Depends on the
  relevant R1–R4 operations, not an artificial all-at-once UI rewrite.
- [x] **R6 — Prove the boundary with a test adapter, not a second router.** Run the production core service with
  an in-process recording/failing adapter, no shell/UCI/ubus/fw4 or OpenWrt paths. Test
  the same identities/configuration, schedules/pacts, manual actions/grants/bonus,
  DNS acknowledgements, recovery and shared UI contract. Reuse existing tests; add
  missing cross-boundary scenarios rather than duplicate covered unit cases.
- [ ] **R7 — Close migration/release gates separately.** Complete P12's mixed-version,
  import/upgrade/interruption and safe backup/restore matrix before removing legacy
  paths or declaring production cutover ready. Deployment still needs authorization.

R1–R3 now cover owned runtime actions/status, facts and ordered network intent.
Inventory relevant callers before each remaining ownership change; do not add unused
parallel stores. R1–R6 are implemented; R7 remains required before production-cutover
claims or retirement of legacy compatibility paths.

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

R1b remains open for status/schedule evaluation and expiry/cache cleanup. Owned
pact-permit revocation has subsequently moved to core (next section); `_clear_allow`
is legacy-only. The broad policy-shaped context and full portable network-intent/
applied-state contract remain R2/R3. The whole network migration is not finished.

## R1b pact-permit evidence (core r12/backend rc10-r9, development only)

Core `PactNetworkOverride` now decides whether a bound captain's earlier permanent
or timed allow must be revoked. It uses the active pact's captain, not ignored current
settings; preserves blocks and other profiles; and leaves unbound/expired/released
pacts alone. The Go OpenWrt configuration projection reads/writes the existing RAM
format while already holding the configuration transaction's ticket/state locks.
It calls no nested `NetworkTransaction` or core socket. Every configuration apply,
including import, reapply and startup reconciliation, uses that same core rule.

A failed application undoes the permit projection's RAM change; the existing authority
rollback reapplies the prior configuration/enforcement on failed pact creation. A
successful pact permanently discards the old allow: later release never restores it.
No new durable state or public RPC is introduced. Owned `core-apply-configuration`
no longer calls shell `_clear_allow`; the legacy pact path retains it. A legacy request
that waited for a lock rechecks ownership before writing, preventing a queued pre-import
pact from modifying the new owner's projections.

The private `pact-network-v1` handshake is checked before the configuration callback
and required on native apply. New core rejects an old backend before mutation; a new
backend rejects the old untagged apply path. Upgrade core/backend together. This is a
specific compatibility guard, not completion of P12's mixed-version/restore matrix.

Focused validation: Go/race core-decision, RAM projection/undo, lock lifetime and
incompatible-bridge checks; `FW_TEST_FOCUS=pact-network sh scripts/test-integration.sh`
passes 42 real OpenWrt checks. A deterministic gate pauses a legacy request after its
ownership check, completes a real import, then verifies the request is refused after
resuming. Both permanent/timed cases verify revocation before injected validation
failure, exact prior RAM plus configuration/native-rule rollback, successful revocation,
no captain ticket/permanent escape, unrelated-profile preservation and no resurrection
on zero-delay root-only fixture release. Reapply/startup clear stale bound permits;
captain blocks and the legacy behavior remain intact. No full browser e2e, emulator,
package build matrix, production router/phone access or other-platform work.

## R1b status/expiry evidence (core r13/backend rc10-r10, development only)

The owned `list` and `status` replies are now one core decision. `Authority`
`configuration_report` (optional `profile` field) asks the backend for one locked
runtime snapshot: the volatile overrides, the last signed-payload fetch epoch and the
versioned network facts. `status.go` then applies the same precedence the legacy shell
used (global disable, manual override, policy, allow windows), expires overrides at
read time, projects the frozen pact captain, revokes standing captain allows through
`PactNetworkOverride`, and reports today's core-owned bonus total. It makes no RAM
writes and triggers no compilation: a lapsed ticket reads as `none` immediately, while
the kernel's timeout element expires natively.

`platform.Context` gains `MastLastSync` (informational fetch epoch, never confirmed
phone enforcement). The `configuration.Backend` contract gains `NetworkSnapshot`; the
OpenWrt adapter takes the shared state lock, parses every volatile row (duplicate or
malformed rows fail closed), reads the mast journal and serves facts through the
existing `core-network-facts` handshake (`network-actions-v1`). The shell's
`ubus_list`/`ubus_status` forward to `configuration_report` in owned mode and keep the
legacy path otherwise; a core-down report is refused, never answered from legacy state.

Focused validation: Go/race unit checks for precedence, window boundaries (daytime,
overnight, equal endpoints, 24:00), read-time expiry, pact captain details/permit
revocation, bonus totals, single-profile shape and strict decoding; OpenWrt adapter
checks for the locked snapshot, absent-state-as-empty, malformed RAM/journal/facts
refusal and lock release; `FW_TEST_FOCUS=status sh scripts/test-integration.sh` passes
22 real OpenWrt checks covering legacy pre-import behavior, explicit cutover, owned
list/status through the original RPCs, a deterministic 00:00-24:00 schedule window,
override precedence, read-time expiry without recompile or RAM rewrite, core bonus,
pact captain fields with a standing pre-pact allow revoked, and core-down refusal. The
full integration run, the network/bonus/pact-network focuses, syntax and LuCI e2e
verify no regression. No production router/phone access.

## R2 runtime-facts evidence (core r14/backend rc10-r11, development only)

Owned mode no longer reads policy-shaped shell output. `platform.Adapter` gains two
explicit methods: `Facts()` and `FactsContext(fn)`. Both run the shell's new
`core-facts` command (handshake `runtime-facts-v1`, shared state lock; `held-v1`
reuses the inherited exclusive lock on fd 3 exactly like `core-context`). The
envelope carries versioned handshakes, timezone, `pact_captain` (drift check against
the document's active pact), `mast_captain` (drift check against the document's
captain), the profile id set (presence check) and per-profile volatile facts: the raw
manual override and the verified DNS data. It carries no profile names, app input,
captain/pact/rules projections.

`Authority` branches on the document: owned `Context`/`Policy`/`WithContext` and
`configuration_bonus_read` consume the facts and rebuild everything else from the
authoritative document (`AppContext` now checks profile presence only; `PhonePolicy`
reads the captain's manual override from the facts and keeps the verified DNS as is;
the obsolete platform-projection log is gone). The legacy `core-context`/`core-policy`
path is unchanged and remains the policy owner before import. `platform.Profile` gains
`Override`/`DNS` facts fields; `platform.Context` gains `MastCaptain` and
`RuntimeFormat`.

Focused validation: Go/race unit checks for the owned policy from facts (captain
override fact, verified DNS passthrough, mast-captain divergence refusal, non-captain
shape, `FactsContext` lock flag), `configuration` suite regression (owned
context/mutation revision parity, bonus readers, backend downgrade refusal now rides
on the facts handshake); OpenWrt adapter checks for the facts envelope, incompatible/
broken envelope refusal, and the inherited-fd lock through the callback with release
on success and failure. Real OpenWrt: `FW_TEST_FOCUS=status` gains two owned checks
(the facts report the owned manual override and leave others untouched) and the core
section gains nine `core-facts` checks (missing/foreign handshake, envelope version,
volatile override, no app input, held-v1 fd and foreign-fd refusal, unknown mode).
Full integration, the network/bonus/pact-network focuses, syntax and LuCI e2e verify
no regression. No production router/phone access.

## R3 network-intent evidence (core r15/backend rc10-r12, development only)

`BuildNetworkIntent` compiles the authoritative effective configuration and one locked
volatile override snapshot into `platform.NetworkIntent` (`network-intent-v1`). Core
owns global/pact freeze, assigned-device exclusions (including disabled profiles),
override/ticket eligibility, ordered forward/router decisions, scheduled allow windows,
router-drop recovery exemptions and captain-lockdown selection. Timed allows carry
absolute deadlines; the native renderer translates windows into kernel rules and
splits overnight windows, including the exclusive 24:00 endpoint. No polling is added.
The existing router-access semantics are preserved: a manual block affects forwarding;
router access follows the profile's `block_router`/policy, not a new independent drop.

Configuration and network transactions expose all raw override rows under the existing
locks. Core supplies candidate and recovery intents; OpenWrt stages JSON diagnostics
and an nft fragment under `/var/run/familywarden/`. The Go native renderer consumes
ordered decisions, not profile eligibility. The owned shell only discovers LAN
interfaces, checks the version/intent marker, validates the staged fragment and invokes
the existing fw4 apply. Missing/foreign intent retains the previous include instead of
falling back to UCI; unavailable LAN mapping refuses a requested default-block apply.
Owned compilation no longer runs shell override-expiry GC. RAM remains RAM; intents
and acknowledgements are derived runtime data, not another durable policy store.

Each intent carries its configuration revision and a content-derived ID (volatile
network actions change the ID, not the configuration revision). Native apply returns
that exact ID only after successful validation/reload; the Go adapter refuses missing
or mismatched receipts. `configuration_status.network_application` exposes the last
process-local acknowledgement, revision and timestamp, explicitly **not** a live
verification of the dataplane or phone. Startup reconciles before publishing a fresh
receipt. Failed application remains an error, restores prior RAM/native intent where
possible, and blocks uncertain policy service until restart reconciliation. A restored
intent is not success for the failed request. The historical fw4 chain-reload gap is
unchanged: this is not a claim of a new atomic dataplane or independent kernel readback.

Both transaction fact handshakes advertise the intent format before their callback;
upgrade core/backend together. Legacy imports/renderers stay gated before ownership.
Focused Go/race tests cover intent precedence, windows/deadlines, pact freeze/revocation,
receipt identity, failed apply and old-bridge refusal; absent global enabled/policy values
retain their legacy defaults in rendering and status. Go tests/race/vet and syntax pass.
Real OpenWrt: 495 full checks, 44 network, 42 pact-network, 24 status and 13 bonus checks
pass. Network adds exact receipt/revision, missing/foreign intent and last-include
retention checks, plus injected include-activation failure: a failed swap cannot delete
active chains or acknowledge a new intent, and prior RAM/include are retained. Fresh SDK packages pass install/reinstall/removal smoke tests and
browser tests (65 legacy-phase passes / 4 skipped, plus all 4 owned-core tests).

The first browser milestone exposed a shared busy-state race: Cancel ticket looked
enabled while the preceding extension's action guard still ignored clicks. LuCI r5
now disables all action buttons for the shared busy interval, retaining stale-data
lockout; a held-response regression makes this interleaving deterministic. The focused
regression and the full fresh-package browser rerun pass. No Android emulator or
production router/phone access; R4–R7 and the complete P12 migration/restore matrix
remain open.

## R4 DNS and protected-release evidence (core r16/backend rc10-r13, development only)

Owned DNS lifecycle now runs in `configuration/dns_runtime.go`. The separate versioned
`dns.json` runtime document atomically retains acknowledged profiles, owned ClientIDs,
attempt fingerprint/time and errors across restarts and configuration rollback.
The OpenWrt adapter supplies secrets and locked one-time legacy import data only;
owned sync never rewrites `dns.ack`/`agh.owned`. Core uses the existing AGH reconciler
and shared no-proxy/no-redirect HTTP client, publishes only verified completed results,
retains previous acknowledgements on failure and refuses unsupported/corrupt runtime.
Desired-state and credential fingerprints trigger reconciliation; failed unchanged
attempts retry after five minutes. Native tick is only a wakeup, not a retry decision.
Credentials remain write-only in the root-private adapter file. Owned phone policy
reads core-owned acknowledgements, not DNS-shaped shell facts. Shell lifecycle commands
forward to the versioned core API and fail visibly on incompatible/missing replies;
queued legacy writers recheck ownership under their DNS lock.

Core `Recovery` owns the release phrase and document-derived status. The dedicated
root-only `/release/` route is separate from administrator API routing; ordinary
`/api/configuration_sail_*` requests are refused. The native CGI retains root-password
verification, session destruction, serialized attempts and failure delay, bounds its
input and forwards only the answer after authentication. Existing privileged root CLI
recovery stays supported. Cooling-off/cancel semantics remain the existing core pact
operations; no ordinary administrator bypass or new public listener is added.

Go unit/race/vet and syntax checks, all 495 OpenWrt integration checks, 42 focused
pact-network checks and 96 real AdGuard Home checks pass. Coverage includes import-once,
verified readback, persistence failure, retry cadence, credential changes, malformed/
future state, native secret locking, mixed-version/core-down refusal, untouched legacy
acks, cumulative pact DNS after restart and protected-route/phrase isolation. Package
and shared-UI milestone validation follows with R5; no production router or phone access.
R5 and R6 were subsequently completed below; R7 remains open.

### R5 completion — portable composition and shared administration (2026-10-28)

`core/internal/service.New` is the production composition. The executable supplies
OpenWrt construction, storage and socket lifecycle; the service package does not
import OpenWrt. This is not the complete R6 recording/failing-adapter proof.

Canonical UI sources are `web/`: Family/profile cards, Requests/usage/new apps,
Devices/enrollment, Ulysses and Administration, with configuration, app/schedule,
DNS, identity and settings editors. English/Danish resources have key parity.
The prototype supplies visual language only; no mock product rules or device data
are shipped. Missing reports remain unavailable rather than fabricated zero usage.
Core computes the router-local screen budget, including today's total bonus.

LuCI has one menu entry and one thin mount/RPC bridge; legacy views, CBI editors and
duplicate shared resources were removed. Host-frame sizing stays in that bridge;
product CSS/layout stays in `web/`. Drafts, CAS/confirm/rollback, fingerprint approval,
frozen-rule guards, DNS-only commits and stale-action refusal have browser coverage.

Evidence: Go race/vet, 495 real fw4/nft integration checks, seven Node checks, syntax,
fresh APK smoke, and 33 browser cases (27 neutral/native foundation, four owned-core,
two shared-control cutover cases). Desktop/mobile screenshots were inspected against
the prototype, including its LuCI-host variant. Package revisions: core 0.13.0-r17,
backend 0.11.0_rc10-r14, LuCI 0.11.0-r6. No production router or phone was modified.

### R6 completion — production service and UI without OpenWrt (2026-10-28)

`core/internal/service/portability_test.go` supplies an explicit recording/failing
consumer of `platform.Adapter`, `configuration.Backend` and `DNSPlatform`. Every
legacy method trips an owned-mode guard. Facts contain presence/deadlines only;
poisoned compatibility labels/app settings cannot become the product authority.
The interim compatibility projection calls the public core `PactNetworkOverride`
operation, as OpenWrt does, rather than implementing another pact-permit rule.
No OpenWrt package, shell/UCI/ubus/fw4 command or router filesystem path is used.
A real temporary `store.Store`, generated signing keys and an empty PATH are used.

Five bounded tests drive the production HTTP handler and real signed phone routes:

- Document-derived policy, native/phone midnight representations, overnight status
  boundaries, additive tickets, bonus day rollover and exact intent acknowledgements.
- Distinct service restart and simulated native-RAM reset, durable bonus/identities,
  and rollback of a lost connection confirmation.
- Signed child requests, scoped parent decisions, delivered grants, persisted usage,
  grant expiry/repair precedence, frozen phone bindings and rejected pact amendments.
- Pact survival across service restart, lockdown/recovery intent, isolated protected
  release, waiting periods and no resurrection of pre-pact network permission.
- Failed configuration/network application, restoration/fail-closed policy service,
  missing-fact refusal, and DNS acknowledgements only after real Go AGH reconciliation
  against a local HTTP fixture; credentials stay outside the core documents.

`TestPortableSharedUI` mounts the same `web/` files against that service. Its browser
driver has no mocked RPC replies: real CAS/save/confirm, signed-upload usage/history,
failed application and stale-action refusal are checked. Its unauthenticated loopback
`httptest` host exists only in test code; it is not a standalone product listener.
The only helper process is the browser driver, not a platform command.

Run `go test -race ./internal/service -run TestPortable -count=1` in `core/`.
After installing the existing `test/e2e` dependencies and Chromium, run
`FW_TEST_SHARED_UI=1 go test -race ./internal/service -run '^TestPortableSharedUI$' -count=1`.
CI's Go job is configured to run both the ordinary race suite and the explicit
browser proof, and to check that the production service dependency graph excludes OpenWrt.

Local final evidence: full Go race suite with the browser enabled, go vet/gofmt,
495 real OpenWrt fw4/nft checks, eight Node checks, syntax/dependency checks and
workflow YAML validation passed. The R5 checkpoint already passed APK smoke and
39 native/shared browser checks; those larger package/UI suites and Android were
not rerun for this test-only slice.

R6 changes test/CI/documentation only; production runtime sources are unchanged from
R5 checkpoint `2b9df05` (core r17/backend rc10-r14/LuCI r8). R7's mixed-version,
interruption, full backup/restore and release gates remain open. No second-router
runtime support, generic adapter framework or production deployment is claimed.

### R7a progress — reject incompatible recovery and concurrent startup

Core 0.13.0-r18 validates the five known versioned documents before startup can apply
rules, roll back a pending edit or import identities. Its native executable acquires a
private lifetime writer lease before store initialization, and waits for HTTP draining
before normal release. Future/truncated document cases and real process contention,
normal shutdown and SIGKILL tests exercise these boundaries. No ownership/schema floor
or historical restore guarantee is implied. R7 remains open; the remaining version,
interruption and backup/restore matrix is in [migration-safety.md](migration-safety.md).

### R7b progress — durable ownership routing

Core r19/backend rc10-r15 persist/read back `core/authority.json` before native apply.
A lost journal or owned registry refuses recovery, not legacy import; native routing
also keeps the core path while the core/journal is unavailable. Old policy/context and
registry seams are refused under the native lock. Adoption requires the native
`authority-v1` capability and preserves the v1 household revision, pact and identity.
The ownership floor is not a newest-state counter: full historical compatibility,
optional-ledger loss and safe old-backup admission remain open in the R7 matrix.

## Acceptance checklist for adapter-ready completion

- [x] A router implementer needs platform facts, enforcement/application and host
  integration only; no second implementation of product decisions or store schemas.
- [x] Production core and shared UI work with the recording/failing adapter; owned
  services do not call policy-producing shell commands. Missing facts fail closed.
- [x] OpenWrt still passes real fw4/nft and authenticated UI/package tests against the
  new contract, including concurrent actions, schedule boundaries and apply failure.
- [ ] Legacy import/format support is isolated, versioned and removable after parity;
  a new router is not required to implement OpenWrt's legacy import/lock protocol.
- [ ] Intended, verified/applied and unavailable/stale states remain distinct for both
  network and phone/DNS policy. Restart/offline operation cannot loosen a pact.
- [x] Build portability, migration safety and actual platform runtime support are
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
