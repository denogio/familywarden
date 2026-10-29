# Core migration plan (A6, A18–A20)

Goal: all product logic in the Go core (`core/`), router specifics in a thin adapter,
one shared administrator UI. The shell plugin (`familywarden/usr/lib/rpcd/familywarden`)
is the interim OpenWrt implementation and only shrinks. Decisions: `app-control.md`
A6 (core owns identity/policy/DNS/apps), A18 (router-agnostic), A19 (core-owned store),
A20 (shared UI embedded natively, on OpenWrt via a LuCI view + rpcd shim).

## Current completion status (2026-10-29)

**R6 proves the owned product boundary; the automated R7 matrix is complete.**
An explicitly authorized physical pilot now runs core-owned configuration; see the
production checkpoint below. The release-signed in-place upgrade was rehearsed on an
emulator and completed on the maintainer's test phone. Broader physical/lifecycle and
active-pact migration validation are not implied. A new router implements platform facts,
network enforcement/application and native host integration, not another copy of
FamilyWarden product rules. Go owning configuration on OpenWrt is necessary but
not sufficient. The source-backed inventory, ordered R0–R7 backlog and acceptance
checklist are in [Router adapter portability](adapter-portability.md).

- **Implemented:** Go identity/requests/grants/usage; portable app-policy compilation
  and AGH reconciliation; opt-in configuration/pact authority, CAS/recovery, document-
  derived phone/app policy and LuCI CAS writes. Core r27/backend rc10-r18/LuCI r8
  are now installed on the authorized production router with explicit configuration import.
- **Still mixed:** owned policy/context no longer read policy-shaped shell output: the
  versioned `core-facts` envelope (handshakes, timezone, captains, profile presence,
  volatile override/verified DNS) feeds the document-derived `AppContext`/`PhonePolicy`
  (facts introduced in core r14/backend rc10-r11). Bonus and manual network/grant-repair,
  owned pact-permit revocation, status/expiry and ordered network intent run in core.
  Core r15/backend rc10-r12 stages `network-intent-v1`, rendered by the Go OpenWrt
  adapter; shell retains LAN discovery, validation and fw4 apply, not owned product
  eligibility or expiry GC. Intent IDs/revisions bind the synchronous apply receipt;
  `network_application` reports acknowledgement, not live dataplane verification.
  R4 additionally owns DNS lifecycle/state and release phrase/status; native credential
  access and authenticated/throttled recovery entry-point integration stay in OpenWrt.
- **R5 implemented:** `core/internal/service` composes the production authority,
  identity and local API without importing OpenWrt. The executable constructs the
  adapter/socket/storage. `web/` owns the shared EN/DA administration workspace;
  LuCI has one mount/RPC bridge and no legacy pages. UCI/nft/fw4/procd remain adapter
  internals, not dependencies of the shared presentation or service composition.
- **R6 proved:** production `service.New`, real durable storage, signed phone routes
  and the unchanged shared UI work with a recording/failing adapter and an empty PATH.
  Owned legacy-method tripwires and a service dependency-graph check prevent OpenWrt
  from becoming an implicit product dependency. Native enforcement remains OpenWrt-only.
- **R7a implemented (core 0.13.0-r18):** read-only known-document preflight now precedes
  constructor recovery/import/application; native daemon startup owns a lifetime writer
  lease before opening the store. Future/truncated document refusal and real process
  contention/exit/kill cases pass focused race tests. This does not make old binaries
  cooperate or validate arbitrary historical restores. See [R7 safety matrix](migration-safety.md).
- **R7b implemented (core r19/backend rc10-r15):** a durable `authority.json` floor
  prevents lost configuration from selecting legacy; owned startup also requires the
  existing registry. Native legacy context/policy/registry seams refuse owned callers,
  including while the journal/core is missing. Matching `authority-v1` adapter support
  is required before activation/adoption. The floor is not an anti-rollback generation.
- **R7c implemented (core r20/backend rc10-r16):** per-file presence witnesses detect
  lost optional ledgers and daily usage before recovery; cached writers cannot reset
  missing counters. Existing data is adopted without byte changes after preflight;
  intentional retention removes its witnesses durably. Native `authority-v2` routing
  also recognizes witnesses. This proves absence detection, not anti-rollback generations.
- **R7d source-state proof:** actual v0.9.1/v0.9.2 backends create fictional active-pact
  households before current APK installation/import. Both paths preserve key/identity,
  DNS/pact restrictions and v2 signed phone continuity (46 checks). The package CI job
  runs `scripts/test-upgrade.sh`; old APK scripts and real app-version runtime are not implied.
- **R7e offline backup (core r21):** `familywardend backup` refuses a running daemon,
  holds native locks, preflights read-only and writes one private, no-clobber, router-
  signed archive; `backup-inspect` needs an external pin and never authorizes restore.
- **R7f restore (core r22):** offline, pinned, cleared-target-only restore of complete
  core-owned households; staged preflight, aside-not-delete, single rename; startup
  materializes and reapplies. Freshness protection was subsequently added in R7g.
- **R7g generation (core r23 + Android):** signed replies carry a witnessed household
  generation (epoch, counter, restore lineage); phones refuse replies that do not continue
  their last accepted state, so an older restored backup cannot replace newer policy.
- **R7h/R7i proved:** tagged source, released v0.9.2 APK and exact-source rebuilt rc4
  upgrades, pinned backup/restore/generation, debug same-key app upgrade, and interrupted
  durable/native mutations. The earlier CI grant failure was traced to concurrent
  policy reads and fixed in `7e600bb`.
- **Physical pilot completed narrowly (2026-10-29):** same-release-key app upgrade,
  router package update and explicit core-configuration import; see below. Physical
  active-pact migration, phone rule mutations, router reboot and wider lifecycle/safety
  validation remain unproven. No second-router implementation or cross-platform build
  is a current workstream or prerequisite.

### Production checkpoint (2026-10-29)

Explicitly authorized router + maintainer test-phone pilot, built from `6e85835`:

- Router: aarch64/mediatek-filogic OpenWrt 25.12.4; backend rc4-r1/core 0.6.0-r1/
  LuCI 0.9.0-r1 upgraded to backend 0.11.0_rc10-r18/core 0.13.0-r27/LuCI 0.11.0-r8.
- Phone: Android 17, release-signed 0.11.0-rc5 (code 121) upgraded over wireless ADB
  to 0.11.0-rc10-pilot1 (code 130), same certificate. The 121→130 in-place update was
  first rehearsed on a disposable Android 15 emulator with Device Owner retained.
- Private quiesced pre-upgrade archive includes configuration, keys, durable state and
  deployed program files; its downloaded checksum matches. Pinned, signed backups were
  also created and inspected before and after explicit configuration import. All real
  household data, APK copies and diagnostics stay outside the repository.
- Router upgraded first, with an observed successful old-app sync before the phone
  update. Identity, key, binding, scope, app revision and applied acknowledgement stayed
  intact. Device Owner, strict Private DNS and its resolver setting were preserved.
- `configuration_import` used the exact exported revision. The complete authoritative
  configuration and effective profile reports match the pre-import export. No pending
  confirmation remains. Ownership and configuration survive a daemon restart; the
  phone resynchronizes and supplies a fresh current app acknowledgement afterwards.
- Existing kernel chains remain present; the native network receipt is acknowledged,
  not independent dataplane verification. AGH reconciliation has no pending/error state.
  The phone's native DNS view reports applied/locked and a successful TLS resolver test;
  this does not independently test every filtering category.
- No active pact existed at migration. No pact was created/modified, no restrictions
  were changed for testing, and no child's daily phone was accessed. Physical rule
  writes, active-pact migration, router reboot and broad rollout acceptance remain open.

Authorized icon follow-up (2026-10-29, source `08e0b40`): core 0.13.0-r28 and
LuCI 0.11.0-r14 replace r27/r12; backend rc10-r18 is unchanged. Maintainer Pixel 10
upgraded in place from release-signed rc10-pilot1 (130) to rc10-pilot2 (131), with
matching signing certificate. Package smoke and browser e2e passed on x86 builds of
the same source; deployment packages were built for aarch64/mediatek-filogic. A
quiesced, pinned signed backup was inspected and downloaded with matching checksum.
Authoritative configuration, ownership, router key, phone identity/role/scope,
Device Owner and Private DNS resolver setting remained unchanged. Fresh phone sync
acknowledged the current policy without app errors; inventory upload succeeded and
administrator usage returned two real app icons. Adapter `ubus dns_status` reports
no error/pending work (it is not a core API method). Private APK copies, backups and
deployment receipt remain outside the repository. This does not validate the broader
physical lifecycle/active-pact matrix or implement the Devices prototype follow-up.

Current follow-up: **pilot observation, remaining physical gates and Android regression/
design continuation**, mapped in [Android API readiness](../docs/android-api-readiness.md).
The historical R7 acceptance evidence and remaining rollout gates are recorded in
[migration-safety.md](migration-safety.md); automated success is not production approval.
R6 adds five production-service scenarios and an explicitly enabled shared-UI browser
proof in the Go CI job. Scope includes policy/actions/grants/usage, pacts/recovery,
DNS acknowledgement, failure/refusal and restart/RAM semantics. It changes tests/CI/docs
only; no production binary or shared-UI source changed after R5 checkpoint `2b9df05`.
R5 evidence: core r17/backend rc10-r14/LuCI r6; Go/race/vet, 495 integration checks,
seven Node checks, fresh-package smoke and 33 neutral/native browser cases pass.
Desktop/mobile screenshots were compared against the design reference; mock data
and product rules were not copied. See `web/README.md` for the host contract.
R4 now owns DNS acknowledgements/ownership/retries in independent `dns.json` and
protected release phrase/status behind `/release/`; native secrets, root authentication
and throttling remain in OpenWrt (core r16/backend rc10-r13). Go/race/vet, 495 full
integration, 42 pact-network and 96 real AGH checks passed at R4. R5's package/UI
validation now also passes; nothing has been deployed to a production router.
R2's runtime facts and R3's portable ordered network intent/exact apply acknowledgement
are implemented. Configuration/network apply and recovery receive core-computed intents
under the existing locks; the owned native renderer does not reconstruct eligibility
from UCI or RAM. Legacy mode keeps the shell as policy owner. Evidence and explicit
limits (including the existing fw4 reload gap and acknowledgement versus live
verification) are recorded in [adapter-portability.md](adapter-portability.md).
OpenWrt test parity alone does not close R4–R7. No other router is being implemented.

The sections below retain chronological checkpoint evidence. Earlier statements about
UCI/shell ownership describe that checkpoint; this summary and the linked audit define
current migration status. No percentage or “migration done” claim is implied.

Working scope: OpenWrt only, preserving future options without implementing another
router or a generic adapter framework. Core owns product meaning; the OpenWrt adapter
retains native schedule/priority/deadline enforcement. Test each function/slice at the
smallest meaningful scope; full e2e and emulator runs are integrated-milestone checks,
not the default per edit. Details and R1a evidence: [adapter-portability.md](adapter-portability.md).

## Original migration sequence (historical)

| Step | Moves to the core | Stays in the shell meanwhile |
|---|---|---|
| **1** | Phone identity: enrollment, device registry, roles/parent scopes, capability check, acknowledgements, request verification and reply signing | Policy *content* (pact, schedules, DNS, apps), firewall, sail page |
| 2 | Requests and grants (P8), built in the core from the start | — |
| 3 | Profiles, schedules, app rules, pacts and the effective-policy computation | Firewall compiler reading the effective config |
| 4 | DNS (AdGuard Home) sync | — |
| 5 | Optional: firewall compiler as a Go OpenWrt adapter; sail page | init/rpcd/LuCI wrappers |

The existing seam for step 3/5: shell enforcement already reads only the effective
config (`fw_load` → `/var/run/familywarden/familywarden`). After explicit import,
the Go OpenWrt adapter now materializes that config from the core document; the shell
used to turn it into nftables. Since R3, owned nft rendering instead consumes the
core's ordered network intent; the UCI projection remains for interim facts/host views.
A new platform must not reproduce the shell's legacy product decisions.

## Step 1 checkpoint (historical)

**Step 1 done (2026-10-24), subsequently deployed (see below).** Initial packages `familywarden` 0.11.0_rc1 (depends on)
`familywarden-core` 0.2.0. Evidence: 321 router integration checks against the core-backed
endpoint and ubus methods (4 implementation-detail assertions that read UCI device sections
rewritten to use the API; new checks for the import, restart and core-down); Go unit tests
with a fake adapter (signatures v2/v3/caps, refusal, fingerprint approval, pending bound,
pact rules incl. fail-closed, parent scope/keys, removal window, import once/verified/
failure, save failure, policy error), mutation-checked; usign secret-key parser verified
byte-for-byte against OpenWrt's own `usign -S`; package install/reinstall/removal tests;
32 Playwright tests against the built packages; upgrade rehearsal (`test-pilot-upgrade.sh`)
from rivendell's 0.10.0_rc1 with the Pixel's 0.10.0-pilot1 app: phone imported and kept
syncing without re-pairing, app pause/ack/kill switch work through the core.

Deviations from the plan below: the removal deadline stays in RAM like pending phones (as
before); the core retries a not-ready adapter at boot instead of exiting (procd's respawn
limit could otherwise leave phone sync down for an hour); the core apk is cross-compiled
with plain Go and only packaged by the SDK (`CORE_GO=cross`, ~3 min instead of ~15).
Footprint: aarch64 apk 2.7 MB, binary 7.5 MB; idle RSS 8.2 MB measured on x86_64 — the
aarch64 RSS was still unmeasured at this initial checkpoint. The subsequently
  authorized measurements and deployments are recorded below.

## Deployment (2026-10-25, explicitly authorized)

Production router (OpenWrt 25.12.4, aarch64_cortex-a53) upgraded from shell 0.10.0_rc1 /
LuCI 0.7.0 to `familywarden` 0.11.0_rc3, `familywarden-core` 0.4.0, LuCI 0.8.0 after a
private config/key backup, with no active pact and one enrolled phone. The administrator's
own Pixel 10 (Android 17, Device Owner) was updated first from 0.10.0-pilot1 (code 100) to
0.11.0-rc3 (code 110, same release key), verified against the old router, then the router.
Observed: core retried once (shell still upgrading) and imported the phone from UCI;
firewall chains and the AdGuard Home sync were intact; the phone advertised
`apps-v1,grants-v1,screen-time-v1`, acknowledged the new app revision, kept locked strict
DNS and uploaded usage. **Measured aarch64 core RSS: 7.4 MB** (overlay use 6.9 → 10.1 MB).
Usage totals for the upgrade day start at the phone update (pilot1 did not count totals).

Follow-up the same day: Approve in LuCI failed with "malformed request" because uhttpd adds
`ubus_rpc_session` to JSON-RPC calls and the strict core decoder refused the unknown field
(CLI calls have no session, so tests missed it). The rpcd shim now drops that field
(`d27e27e`, integration check sends it). `familywarden-core` 0.5.0 (also shared-v1) was
installed after a fresh backup; its postinst did not restart the daemon, so 0.4.0 kept
running until a manual restart. 0.5.0-r2 restarts it on upgrade. Measured RSS 7.2 MB.

Later the same day, explicitly authorized: upgraded the administrator's Pixel first to
release-signed 0.11.0-rc4 (code 120, same certificate), then the router to shell
0.11.0_rc4-r1 / core 0.6.0-r1 / LuCI 0.9.0-r1. The upgraded postinst restarted core
successfully; status confirms running 0.6.0-r1. Private backup:
`~/Backups/familywarden/rivendell-20261025-142446/config.tar.gz`. No active pact;
configuration checksum unchanged, firewall chains present, AdGuard Home sync without
error or pending changes. The phone acknowledged its new app revision without error,
advertises `shared-v1`, and today's usage file updated after deployment. USB disconnected
during final checks; final sync/ack and report receipt were verified from the router.
New-app suspension remains disabled on the existing profile; no rules were changed.
Observed core RSS 11.2 MiB; overlay 31.7 MB free.

## Step 2 foundation (2026-10-24)

`familywarden` 0.11.0_rc2 / `familywarden-core` 0.3.0 adds the Go-owned LAN
request/decision ledger and device/key-bound grants, not the complete P8 Android flow.
See `app-control-protocol.md` §3–4 for the implemented transport, bounds, retry/expiry
semantics and remaining consumption gates. Requests/decisions never use UCI or shell
product handlers: the CGI and rpcd methods forward to the core; `EnsureTicket` is a
narrow OpenWrt application bridge using the existing firewall compiler. Existing
block/allow/reset and ticket mutations now share adapter transaction locks. Failed
application restores prior RAM ticket intent, so retries cannot falsely claim success.
Go owns authority; the adapter cannot mint a grant.

Validation: Go vet/race suite; six security mutation probes (mandatory hash, phone binding,
parent scope, durable clock floor/expiry and ticket repair); 363 real OpenWrt integration
checks, including signed CGI upload, exact request/reply hash, approval, capability refusal,
acknowledgement, real daemon restart, kernel ticket restoration, failed compilation rollback
and concurrent manual blocks; built-package install/reinstall/removal tests; 32 Playwright
tests against the resulting 0.11.0_rc2 / 0.3.0 packages. One package-feed fetch stalled;
the bounded rerun passed. No Android code changed, so no emulator run was needed.

Android grant enforcement and request screens followed (protocol §4, 2026-10-25).
Authenticated consumption uploads and the first shared administrator UI were subsequently
implemented and deployed. Shared usage offsets are implemented; bounded shared allocations
or leases are not (cached offsets do not bound offline overspend).

## Step 3a: portable signed app-policy compiler (2026-10-25)

Implemented in `internal/rules.CompileApps`: signed `apps` construction, the unchanged
16-hex canonical revision, and `apps-v1`/`screen-time-v1` requirements. The adapter passes
one locked snapshot of effective canonical app rules and today's bonus for each profile.
Both administrator/request context and phone replies use the same compiler. Malformed
compiler input refuses the complete policy; it never drops a restriction. The existing
canonical hash bytes, zero-minute rules, blocked/approved precedence, overnight downtime,
per-weekday totals, always-allowed packages and bonus wire shape are preserved.

Removed shell `_json_apps`, `_apps_rev`, `_apps_requires` and `_core_requires`.
The explicit `go-apps-v1` handshake is required in both directions: old core/new shell or
new core/old shell refuses service, retaining the phone's trusted cached policy rather
than delivering a weaker policy. **Upgrade core and shell together**. Prepared package
versions: core 0.7.0-r1 / backend 0.11.0_rc5-r1; not installed on production devices.

Evidence: Go vet/race suites (including subprocess adapter tests and a fixed wire/hash
fixture), five caught compiler mutations, 390 router integration checks, package tests
and all 36 Playwright tests. The new real HTTP approval test creates a genuinely signed
phone request, clicks Approve in LuCI without RPC mocks, then verifies the durable grant.
Package tests force an actual local-package reinstall and verify changed daemon PID and
running version. A minimal rc.common lifecycle shim replaces absent procd in that rootfs;
real procd crash/reboot coverage remains in `scripts/test-core-lifecycle.sh`.
Session stripping also preserves duplicate-field, malformed/oversized and invalid UTF-8
rejection instead of normalizing bad inputs before strict decoding.

**Step 3 is not complete:** UCI still owns profiles and configured app rules. The shell
still normalizes raw UCI into canonical app lines, expands weekday/weekend settings,
resolves pact snapshots and manages daily bonus mutations. Next slice must migrate those
writers and their shared editing UI together; do not add an unused parallel JSON store.
Required gates: import/save/read-back once, preserve active pact snapshots and amendment
rules, reject stale edits, fail-closed state corruption, and safe firewall apply/rollback.
No Android change or physical-device/emulator run was needed for this wire-compatible
compiler move. Off-home transport and adult-pool semantics remain separate open gates.

## Step 3b: raw app configuration and safe snapshot compilation (2026-10-25)

`rules.AppConfig` now owns legacy normalization, including strictest duplicate app rules,
blocked precedence, malformed/oversized minute handling, daily/weekday/weekend expansion,
package/downtime validation and sorted canonical bytes. The OpenWrt shell exports raw
configuration only (`go-apps-v2`); both phone/context paths compile it in Go.
A stateless `familywardend app-canon` command supplies the canonical app portion of the
platform's pact snapshot. It reads bounded JSON and never touches the socket or store,
avoiding reentrant calls while the shell holds its firewall/pact lock.

Snapshot compilation fails before emitting a fragment. Creating a pact checks the compiler
before activation; replacing a snapshot uses a temporary file and propagates failure all
the way out of reload. A failed amendment leaves the previous frozen snapshot, effective
configuration and firewall include intact. Core 0.8.0 / backend 0.11.0_rc6 are prepared,
not deployed. App configuration and pact state are **still stored in UCI/shell files**;
this is not the authoritative-store cutover.

Evidence: 404 router integration checks, including six byte-for-byte comparisons against
a frozen pre-migration normalizer (`test/app-canon-legacy.sh`) and injected compiler failures
on pact creation/amendment. Unit tests cover raw configuration, zero vs unset limits,
leading zeros, duplicate blocks, invalid overrides, format versions and bounded CLI input.
Five normalization mutation probes are caught (lost blocks, wrong duplicate minimum,
malformed-to-unlimited conversion, ignored weekday override, zero treated as unset).
Long checks/builds run from immutable temporary snapshots, not the editing checkout.
Snapshot browser checks need their own `npm ci` from the pinned lockfile; a clean source
archive intentionally contains no `node_modules` from the editing checkout.

### Platform transaction gate (backend 0.11.0_rc6-r2)

Pact changes now take the same ticket/state lock order as overrides and tickets; bonus
mutations also hold the state lock while checking the pact and writing their file. No
product rules moved into the adapter. Deterministic tests queue operations behind a
held state lock and verify that files do not change early; parallel extensions must
retain the longest deadline. The pre-fix snapshot fails all three checks (early pact
write, shorter extension overwriting a longer one, early bonus write). The fixed suite
passes 409 integration checks. This is platform-side serialization, **not** yet one
core-owned transaction across phone identity, profiles and pacts.

### Core-backed profile directory (core 0.9.0 / backend rc7 / LuCI 0.10.0)

The read-only `profiles` administrator method returns sorted `{id, name}` entries for
all profiles, including disabled ones. It does not expose app configuration or enrollment
keys. The Phones page now obtains both selector/scope options and display names through
that API; it no longer loads or reads the FamilyWarden UCI package. Directory errors
reject the load rather than pretending the registry is empty. This makes the reader
ready for a later authority switch without introducing another stored profile list.
The directory still reads the current adapter context; **no profile store cutover yet**.

Go vet/race suites and 412 integration checks pass, including stable ordering,
empty-directory/error shape, disabled profiles and data minimization. Package checks
exercise the real rpcd bridge; a browser test supplies a core-only profile absent from
UCI and verifies labels/options and zero direct FamilyWarden UCI reads. The full package/
browser pipeline is running on an immutable snapshot at this checkpoint. These versions
are development packages, not production installs.

### Settings profile reads and unavailable-state guard (2026-10-25)

Settings now gets captain choices and DNS profile display names through the existing
core `profiles` directory, including profiles absent from UCI. The global settings
form remains UCI-backed; `form.Map.render()` performs its own configuration load.
This is reader preparation, not a new store, writer migration or authority switch.

Both policy-status and directory response envelopes are preserved and validated.
An error/missing/malformed response rejects the view before constructing editable
fields, rather than treating unavailable pact state as an empty, unfrozen household.
Valid empty directories still work, and a reported active pact keeps global fields
read-only. LuCI displays the rejected-load error and reports that exact error to the
browser; error tests explicitly assert it rather than suppress unexpected failures.

Evidence: syntax checks, five Node view-model tests, 24 focused real-LuCI browser
checks and the full disposable OpenWrt suite (46 passed, including the guarded real
HTTP approval fixture). A regression
probe restores the previous Settings source and confirms that the core-only profile
selector test fails. Tests used an immutable source snapshot and matching existing
development packages with the Settings source overlaid; no new package deployment,
physical-phone change or live-router change occurred.

### Administrator usage coverage (core 0.10.0, 2026-10-26)

The administrator's seven-day usage rows now include `reported`, distinguishing a
missing report from a real zero sample. The shared view preserves unknown coverage
on older cores, renders gaps/zero honestly and labels chart days accessibly in EN/DA.
This is a read-summary addition, not a profile/pact store migration or a signed phone
policy change. Core 0.10.0 is prepared, not deployed. Go vet/race, six Node tests and
53 browser tests pass with the compiled source binary/UI overlaid in disposable LuCI;
new package/release installation remained a separate gate at that local checkpoint.
Follow-up [CI 40279978074](https://github.com/denogio/familywarden/actions/runs/40279978074)
passes the x86 SDK build, package install/postinst/reinstall/remove smoke tests,
aarch64 core build, packaged LuCI e2e and Android checks. It does not deploy or release.

### Partial identity patches (core 0.10.0-r2, 2026-10-26)

The current device writer had a lost-update race: `Update` copied omitted profile,
name and administrator fields before the final write lock. A later name-only patch
could undo a concurrent administrator change. Omitted fields now resolve from the
current row under the same core mutex as context checks, validation and persistence.
A deterministic context-gate test reproduces the stale administrator restoration on
the prior code; no timing sleeps or statistical race loops are needed.

This repairs one real current writer, without introducing a parallel configuration
store, a new RPC contract or a signed-policy change. Explicit edits to the same field
still follow existing last-writer semantics; this is not configuration CAS. The
platform's context lock still ends before the core write: shared cross-process pact
transactions and the authoritative configuration cutover below remain incomplete.
Go vet/race, syntax and the full real OpenWrt integration suite pass on the final
immutable inputs. The throwaway integration router is capped at 768 MiB/two CPUs;
its container is removed after the run. No deployment.

### Runtime pact gate (core 0.11.0 / backend 0.11.0_rc8, 2026-10-26)

Existing core mutations now use `Adapter.WithContext`: the core mutex is taken first,
then the OpenWrt state lock stays held from snapshot through validation and persistence.
This covers approve/update/delete/removal, administrator request/new-app decisions and
all four authenticated upload paths. The fake adapter implements the same boundary.
Ticket/enforcement jobs run only after both locks release, preserving ticket→state lock
order and avoiding an IPC callback into a core already holding its mutex.

The Go OpenWrt adapter passes its locked open-file description as child fd 3. The
private `core-context ... held-v1` mode checks the descriptor's inode and locks that
same description instead of deadlocking on a second fd. The parent retains the lock
after the snapshot subprocess exits, including during durable writes. Acquisition and
command execution are bounded; snapshot errors release the lock without calling the mutation.
Callbacks themselves may report failed saves and also release the lock.

Private format `go-apps-v3` is mandatory on both context/policy paths: install these
core/backend development packages together. Older formats/missing or foreign descriptors
are refused, never interpreted as an unfrozen household. Phone wire formats, keys,
store schemas, app revisions and grant/retention rules do not change. No deployment.

Go vet/race, syntax and real fw4/nft integration pass on immutable inputs. Tests check
all ten runtime mutation routes and their document/usage writes, deferred home tickets,
real inherited-descriptor lock lifetime, callback-error cleanup, cancellation, invalid
snapshots and mixed-format refusal. A negative probe that releases the lock before
the callback fails as intended. The real router test queues a binding change behind
the state gate and verifies no early write and pact refusal afterward.
The opt-in CI emulator job now groups lifecycle and signed child/ordinary-parent suites;
[CI 40305278912](https://github.com/denogio/familywarden/actions/runs/40305278912)
passes SDK/package tests, packaged LuCI and both grouped Android lifecycle and signed
child/ordinary-parent suites. The clean-worker fixture signer was repaired in 92b6fde;
no production signing key was used.

This is a concrete runtime race fix, **not the household-store cutover**. Direct UCI
form writes are still outside this gate, and the shell still owns pact semantics and
snapshot files. The configuration/CAS/apply-confirm/rollback work below remains open;
no parallel or unused configuration store has been added.

### Versioned migration input (core 0.11.0-r2 / backend rc8-r2)

The local root-only `familywardend api configuration_export` reader now returns one
version-1 configuration snapshot and a content revision. It captures configured and
frozen profiles separately under one shared platform lock, rather than attempting to
import the effective profile as the editable configuration. Raw unset/zero values,
disabled profiles, existing DNS ClientIDs, anonymous section identities, schedules,
app/downtime sections, legacy frozen associations, DNS snapshots and release deadlines
are represented. Runtime usage/grants, credentials and router signing keys are excluded.
The Go revision covers the complete snapshot, including frozen state and release time.
Missing frozen data is an error, not an empty/unbound household.

This is the live migration **reader**, explicitly returning `owner:"legacy"`; it does
not create a second configuration store or switch authority. The transactional writer,
import/read-back completion, store-backed policy/projection and UI writer cutover below
are still work in progress. No editable API or deployment is implied by the export.
Go vet/race, syntax and real OpenWrt integration pass with live-vs-frozen edits,
disabled profiles, anonymous IDs, ClientIDs and missing-snapshot refusal covered.

### Configuration authority (core 0.12.0 / backend 0.11.0_rc9, 2026-10-26)

`core/internal/configuration` now owns one version-1 configuration document
(`$FW_PERSIST/core/configuration.json`) with a content revision, and the shell stops
writing to it once the document exists:

- **Opt-in activation:** `configuration_import` accepts the legacy export only with its
  exact revision. Import is explicit; a fresh router stays legacy until activated.
- **CAS:** `configuration_apply` compares the read revision inside the transaction; a
  pending edit survives its browser session and returns `confirmation_required`.
  `configuration_confirm` completes it with the revision + token. An unconfirmed edit
  expires after two minutes (`configuration_tick`/deadline) or a core restart — the
  previous configuration is restored and the revision seal is not lost.
- **Transaction order:** core mutex → platform state lock + ticket lock → legacy facts
  → validate → pending save → `core-apply-configuration` renders the effective UCI and
  fw4 include under the inherited state fd → confirm or rollback. A failed apply leaves
  the authority fail-closed (`blocked`) until restart recovery; future or corrupt
  documents refuse both reads and the signed legacy fallback.
- **Pact ownership moves with the document:** while the document exists the shell's
  `gc_pact`, sail request/cancel, tick, reload, pact and DNS ClientID writes short-circuit
  to the core. `list` now reports `configuration` (enabled, default_policy, captain,
  waiting period, assigned devices) so LuCI never loads the UCI collection to render
  ownership facts; `profiles` disables Save/Reset and `reload` re-applies the
  projection when the owner is core. Legacy `ubus_pact`/sail keep working untouched when
  no document exists.
- **Validation:** apply-time revalidation rejects duplicate identities, malformed JSON
  (duplicate keys, trailing data), unbounded or non-decimal limits and invalid
  DNS/ClientID state; DNS ClientIDs are allocated inside the transaction when missing.
- **LuCI Settings saves through CAS (2026-10-26):** in core-owned mode Settings reads
  `configuration_export`, stages the edited global fields onto that document and runs
  `configuration_apply` → `configuration_confirm` with the exported revision
  (`familywarden/configuration.js`); a stale revision is shown as an error, never merged.
  UCI is not written.
- **LuCI Profiles saves through CAS (2026-10-26):** the same CBI form renders from the
  projection, but only after `fwconfig.matchesUci` confirms the loaded UCI equals the
  exported document (otherwise the page refuses to load). Row add/remove and profile
  create/delete stay in the browser's UCI cache (no server-side UCI staging); Save &
  Apply, create and delete write `fwconfig.fromUci` (the shell export's option mapping)
  through apply + confirm. Verified with mocked CAS tests and once against a real
  imported router (document revision advanced, confirmed, projection updated, no UCI
  deltas).

Verified on the final immutable inputs: Go vet/race, syntax, the full real OpenWrt
integration suite (import → CAS → confirm → stale refusal → re-import no-op → restart
rollback → fail-closed future schema), 55 packaged-LuCI Playwright tests including the
new owned-mode profiles/settings gate, and a disposable real-procd QEMU router (import
of an active pact, visible pending edit, reboot rollback with pact, release deadline
and signing key intact). Core/backend versions are prepared; **no deployment**.

### Phone pact/rules from the core document (core 0.13.0, 2026-10-26)

When the core owns the configuration, `Authority.Policy` computes the signed reply's
`captain`, `pact` and `rules` from the document (`configuration.PhonePolicy`: active pact
with type/until/release, effective enabled/policy and complete captain windows, frozen
until an explicit amendment). The shell's `core-policy` now only contributes runtime
facts in that mode: the captain's manual override, DNS acknowledgement and app input with
today's bonus. A different captain or a missing override refuses the reply (the phone
keeps its last trusted policy); any other difference from the shell's projection is
logged and the document wins. Legacy routers are unchanged.

Evidence: unit tests (no pact, other profile, frozen `until` pact, ongoing amendment
window, divergent captain, missing override) and real OpenWrt integration (460 checks):
an owned router with a live looser schedule edit sends the frozen window, the release
deadline and a manual block, and the core log shows no shell/core divergence. A mutant
that reads live instead of effective configuration fails both the frozen-window and the
divergence checks.

The imported configured-vs-frozen amendment-window edge was resolved in core
0.13.0-r8 (see below): neither entering the window nor editing another profile can
activate previously ignored configured captain rules.

### AdGuard Home reconciliation in Go (core 0.13.0 / backend 0.11.0_rc10, 2026-10-26)

`core/internal/agh` now owns the AGH protocol and FamilyWarden's ownership boundary:
own `familywarden-<ClientID>` clients only, never taking over a foreign client's id,
deleting only own stale clients, adding/removing only owned `allowed_clients` entries
after a fresh re-read, never touching an empty (open) allow list, verifying the list
after `access/set`, and acknowledging a profile only after AGH accepted its client
(a failed profile keeps its previous acknowledgement). Legacy ClientID derivation
(`Slug`, numeric suffixes against every taken id) is reproduced exactly.

The shell's `_dns_sync_locked` keeps only platform duties: the desired profiles (live
DNS plus the captain's pact snapshot, frozen domain), credentials, and storing the
stateless `familywardend agh-sync` result (acks, owned ids, legacy ClientID
allocations to UCI). Its HTTP/reconciliation helpers are removed, including the
service catalogue (`dns_services` now calls `familywardend agh-services`). An older
core without these helpers fails explicitly and keeps the last acknowledged DNS
policy. Both commands accept bounded input, reject trailing documents and invalid
endpoints, bypass HTTP proxies and refuse redirects rather than forward credentials.

Evidence: Go unit tests against an in-memory AGH API, and the unchanged
`scripts/test-agh.sh` (58 checks against a real AdGuard Home v0.107.79 container,
now driving the Go reconciler) plus 460 integration checks. A mutant that writes an
empty allow list fails the real-container check.

In core-owned mode the desired DNS set also comes from the document
(`configuration.DNS` via `configuration_dns`): live DNS plus an actively bound
captain's snapshot (DNS stays on, protections/services only accumulate, frozen
domain), including a captain whose profile was deleted. The shell's projection-based
computation remains for legacy routers only. `test-agh.sh` now also imports a real
router into core ownership (fw4 apply), enables DNS through a confirmed core edit and
verifies the AGH client and acknowledgement; a direct UCI edit that switches DNS off is
ignored. The resolver URL also comes from the document for both sync and service
catalogue calls; direct UCI endpoint edits cannot redirect the stored credentials.
Re-enabling the UCI path in owned mode fails that check.

Additional safety regressions cover an allow list that becomes empty between the
first read and the final re-read (leave it empty and retain ownership records), and
missing/null access-list fields (refuse the sync before any writes rather than
replace foreign settings with inferred empty values). Missing client/catalogue
collections are errors, not valid empty collections.

Mixed-version DNS safety (core 0.13.0-r5 / backend 0.11.0_rc10-r3): the private
`configuration_dns` reply now requires `dns-config-v1` plus typed captain/input,
domain/URL strings and a profiles array. A missing URL in an older core's reply
must not be mistaken for explicitly cleared settings (the previous combination
could remove the last DNS acknowledgement). Both sync and catalogue refuse the
incompatible/incomplete reply before touching acknowledged policy. The real AGH
suite wraps the core CLI to reproduce the older shape and a version-marked reply
with a missing URL: both preserve the acknowledgement and matching packages recover
without re-pairing. The pre-fix source fails the acknowledgement/catalogue checks.

### Recovery and downgrade limits (optional core ownership)

These are safety limits, not a supported downgrade procedure or deployment approval.

- An unconfirmed configuration edit is deliberately rolled back on its deadline or
  daemon restart. A browser timeout, RPC transport success or reboot does not confirm
  it; only its current revision/token does. A confirmed edit is not automatically
  rolled back on restart.
- A corrupt/future-version document, failed read-back or uncertain apply must stay
  fail-closed. Since core 0.13.0-r4, the pending journal's previous configuration
  must pass its own revision check as well as schema validation: the candidate's
  revision does not cover it. Schema-valid corruption is refused before deadline
  rollback or restart can change the store/platform. Do not delete `core/configuration.json`
  to make legacy mode work. In unmarked historical stores its absence selects legacy;
  core r19/backend rc10-r15 retain core routing through `core/authority.json` and refuse
  missing journal/registry recovery. Never remove the floor to force legacy: it is not
  a recovery procedure. Repair requires compatible software and explicit review.
- `core/migration-backup` contains the original legacy configuration/pact projection
  at first activation. It is neither a current household backup nor an undo button;
  restoring it after core edits can remove restrictions or reinstate old bindings.
- A durable backup must preserve the entire `/etc/familywarden/` tree (including
  core documents/usage, router keys, AGH credentials/ownership/acknowledgements and
  legacy projections), plus `/etc/config/familywarden` and the package/version
  manifest. Keep root-private permissions and do not publish/export these secrets
  as test fixtures. Live filesystem copies across separate document replacements
  are not a transactionally consistent backup; backup/recovery must be quiesced and
  separately authorized on a real router.
- Restoring only an old grant/usage/new-app ledger can replay already consumed
  grants, lower accounting offsets or resurrect approvals. Restoring an entire old
  tree is also not automatically safe: check current pact restrictions/deadlines,
  device bindings and monotonic grant/usage state before any restoration. No tool
  currently merges such historical state safely.
- Package downgrade is not schema downgrade. A pre-authority core/shell must not be
  installed over an owned household just because an older package can read the UCI
  projection. The private `go-apps-v3` and stateless AGH helper seams refuse known
  mismatches, but they do not prove arbitrary old/new package combinations safe.
  Retain legacy paths until the disposable upgrade/interruption/mixed-version matrix
  has passed; the P12 downgrade gate remains open.

Recovery evidence: unit tests first reproduced acceptance of a schema-valid but
revision-invalid rollback target at both deadline and restart, then pass with the
shared sealed-configuration validator. Additional tests interrupt initial import at
its first journal write, platform apply and completion write; a durable but unfinished
import refuses legacy fallback and restart finishes the exact saved revision despite
subsequent UCI edits. The real router suite also corrupts a pending rollback target
and checks rejection with the journal unchanged, before restoring it and testing the
normal reboot rollback. These checks do not close the broader package-downgrade gate.

### Owned app-policy configuration comes from the document (core 0.13.0-r3)

`configuration.AppContext` builds the app compiler input directly from
`Effective(configuration, router-local time)`: enabled/suspend-new flags,
weekday/weekend totals, package rules, downtime and always-allowed packages.
Only today's bonus and the timezone come from the adapter snapshot. It rebuilds
profile labels, app revisions and capability requirements for all authoritative
profiles and discards stale platform-only profiles. Missing runtime facts, malformed
bonus or a divergent pact captain refuse the operation instead of weakening policy.

`Authority.Context`, `Authority.WithContext` and `Authority.Policy` share this
compilation, so request validation/mutations and signed phone replies use the same
revision. `WithContext` keeps compilation and the mutation inside the adapter's
state lock; v2 replies do not acquire a v3 app-policy payload. Legacy ownership and
the private `go-apps-v3` adapter seam remain unchanged. The shell still exports
legacy app fields for compatibility, but they are not the owned-mode policy source.

Regression tests supply deliberately looser platform rules (999 minutes), stale
profile labels and an extra deleted profile; the document's frozen limits, zero
weekday limit, downtime, blocked precedence and today's bonus must win. A mutation
that returns the adapter context unchanged fails both app-context and phone/mutation
revision tests. The real router test changes the runtime projection to 777 minutes
and still receives the authoritative frozen 10-minute policy.

### Strict DNS edits before persistence (core 0.13.0-r6)

New/changed DNS fields are validated before saving or applying the candidate:
ClientIDs must use AGH's supported grammar; protections must be known policy IDs;
service IDs must fit the phone's `[a-z0-9_]{1,64}` contract; configured domains and
complete `<ClientID>.<domain>` hostnames must fit Android's hostname grammar/length;
resolver URLs must be plain HTTP(S) base endpoints without embedded credentials,
query or fragment. The endpoint check is shared with the AGH CLI.

Enabling DNS requires a domain even when the profile already has a ClientID, and
clearing a domain while any affected profile still enables DNS is refused. Disabling
DNS and clearing the domain in one explicit edit remains allowed outside a pact.
Untouched legacy fields retain import semantics: an unrelated rename does not
normalize or silently drop old DNS data. Unit tests reproduced acceptance of invalid
ClientIDs/protections/services/domains/endpoints and oversized combined hostnames
before the fix, then verify refusal without store/platform writes, valid edits and
legacy preservation. The real AGH test rejects activation without a domain on a
profile with an existing ID and then successfully applies the valid edit.

### Explicit pact amendment from Profiles (LuCI 0.11.0-r2 / backend rc10-r4)

When an owned captain's ongoing-pact amendment window is open, Profiles Save & Apply
asks for explicit confirmation that the change commits immediately and has no
connection rollback. Only then does it call revision-bound `configuration_amend`,
not `configuration_apply`/`configuration_confirm`. The RPC bridge and both write ACLs
include only that amendment method; sail release remains CLI-only. Other profiles and
ordinary Settings/Profile edits retain the normal confirmed CAS flow.

Browser regressions cover cancellation (no write, staged fields retained), successful
amendment and a window that closes before the core handles the write (error, staged
fields retained, no ordinary apply/confirm fallback). Core tests separately refuse stale
revisions, other-profile edits and a closed window without changing journal/platform
state. An unmocked disposable router check imports the configuration, creates an open
ongoing pact, edits the captain through the real LuCI session/ACL/RPC bridge, and
verifies the frozen edit and absence of a pending rollback before and after daemon
restart. No live router or physical phone was touched.

### Preserve exact unchanged records across the UCI form projection (LuCI 0.11.0-r3)

`fwconfig.fromUci` now reuses unchanged authoritative field values, records and
collections after semantic comparison. In particular, UCI's missing lists (`[]` in
the browser mapping) must not rewrite the document's `null` lists or reorder an
unchanged collection. Otherwise saving another profile could appear to mutate the
frozen captain and be correctly refused by the core's exact pact guard. The guard
itself is not weakened.

A Node regression first reproduced the unintended rewrite, then checks ordinary
edits, creation and deletion without modifying the input document or unrelated
captain fields. The browser regression edits an unrelated profile with nullable
captain/collection fields. An additional unmocked router check imports nullable
lists, creates an active captain pact, edits another profile through real LuCI
CAS/confirm, and verifies the captain's frozen null fields and pact are unchanged.

### Verify AGH client read-back before acknowledgement (core 0.13.0-r7)

Client add/update success is now followed by one fresh `/control/clients` read before
acknowledging new policy or adding its ID to a restrictive allow list. Verification
checks exact owned name/ID, explicit per-client/global flags, filtering/Safe Browsing/
parental settings, SafeSearch switches, blocked services and absence of custom
upstream/tag/logging overrides. A mismatched profile keeps its prior acknowledgement
(or none) and is not newly allow-listed; verified other profiles can still sync. An
unreadable/malformed read-back refuses the complete acknowledgement replacement.

The new regression first demonstrated that a fake AGH server returning success while
ignoring `filtering_enabled` was optimistically acknowledged. It now retains the old
ack instead, with additional mismatch cases for each restriction and a failed read-back
HTTP response. The real AGH v0.107.79 suite exercises the same read-back path. This is
server-configuration verification, not a claim of physical handset DNS/runtime tests.

### Imported pact stability across amendment windows (core 0.13.0-r8)

The enforced snapshot is now time-independent while the pact remains active:
`Effective` always uses `pact.frozen`. Opening or closing an amendment window permits
or refuses an explicit write; it does not itself promote ignored legacy UCI edits.
Likewise, an ordinary edit to another profile no longer refreshes the captain snapshot.
Only the explicit `configuration_amend` or pact-amendment operation refreshes frozen
rules/DNS (preserving frozen phone associations), including a deliberate amendment
that activates configured values without changing their current representation.

Two regressions first reproduced automatic rule promotion on window entry and during
an unrelated confirmed edit. Tests now keep the imported 10-minute/20:00 restrictions
before, inside and after the window, then explicitly amend to 999 minutes/23:00 and
verify that accepted snapshot persists after the window closes. The router integration
fixture imports while its ongoing window is already open and repeats the unrelated
edit checks against real signed phone replies. No new schedule-polling loop or daemon
is introduced.

### Midnight boundaries remain consistent across kernel and phones

Core 0.13.0-r8 / backend 0.11.0_rc10-r5 preserve the document's accepted `24:00`
internet/amendment endpoint without emitting an invalid nft clock literal. The
adapter uses the existing day match as the exclusive end-of-day boundary, skips the
nonexistent start-day portion of a `24:00` overnight start, and omits empty next-day
`<00:00` portions. Input, forwarding and lockdown still use kernel-evaluated rules;
there is no userspace schedule loop or one-second truncation.

Signed internet windows are separately normalized for Android `LocalTime`: end
`24:00` becomes `00:00`, start `24:00` shifts to the next weekday, equal `24:00`
bounds retain the documented whole-day meaning, and the empty `24:00..00:00`
interval emits no allow window (never an accidental all-day allowance). Ordinary
and existing overnight wire windows remain unchanged, with no new phone feature or
wire version required.

App downtime retains its existing 00:00..23:59 compiler/mobile contract: new edits
with `24:00` or no day are rejected before persistence instead of being silently
discarded by legacy normalization. Unchanged legacy downtime is not reinterpreted
on unrelated edits. Unit regressions fail against the prior clock implementation;
router tests cover both traffic chains, lockdown, a real signed midnight endpoint
and rejection of a downtime the app compiler cannot enforce.

### Explicit pact DNS edits (core 0.13.0-r9 / backend rc10-r6 / LuCI r4)

`configuration_dns_update` is a revision-bound, DNS-only transaction for an active
pact. Unlike an ordinary apply/confirm, it has no connection/deadline rollback;
LuCI asks explicitly before committing. It rejects any bundled settings, app,
profile, binding or other-profile edit, identity rotation, stale revision or pending
configuration. Failed backend application restores the previous document/snapshot.
An amendment-window save still uses the explicit full `configuration_amend` route.

The current captain record stores the requested future DNS policy. The pact DNS
snapshot and frozen captain accumulate the previous and added protections/services;
switching current DNS off or removing entries cannot remove that active minimum.
Enabling DNS allocates a stable ClientID atomically; an existing bound identity and
DoT domain are retained. Only explicit full amendment or pact end can loosen the set.
The router must verify AGH state before its existing acknowledgement/phone path
advertises the policy; a successful form save alone is not phone enforcement proof.

The DNS-only browser payload preserves every non-DNS field exactly, including
readonly defaults. All shared form projection now reads options through `uci.get`:
`uci.sections` enumerates live IDs but retains properties of staged option unsets.
The real, unmocked LuCI flow exposed an unchecked DNS flag being resubmitted as on;
the new regression also covers removed last list values and cleared app limits.

Validation: Go/race/vet, syntax, 9 shared-module checks, 487 router checks, 92 real
AGH checks and 65 browser tests pass. Unmocked authenticated LuCI enabled DNS, then
saved a deferred disable without browser-side UCI mutations or apply/confirm; its
allocated identity and accumulated restrictions survived core restart. The OpenWrt
adapter still materializes current configuration into `/etc/config/familywarden`
and the effective frozen rules into `/var/run/familywarden/familywarden`; these
copies are expected to change and are not competing configuration authorities. These are disposable checks,
not deployment or a complete upgrade/restoration matrix. The earlier consolidated
CI `40456639004` passed package/Go/Android-build/LuCI jobs but failed the Android
request scenario after grant-process restart; that failure remains under investigation.

### Unmocked owned-core browser gate

CI enables a second e2e phase in a fresh disposable OpenWrt container, separate
from the legacy suite's pact state. An explicit phase flag prevents import tests
from running during the legacy phase or against an ordinary configured router.
The fixture prepares synthetic resolver settings before the root-only import;
no public import method is added.

The tests exercise import, CLI apply/confirm plus browser projection, and
real authenticated LuCI pact creation/DNS enable/deferred disable. Browser RPC
capture checks both FamilyWarden operations and UCI mutations (not just
FamilyWarden calls). The current UCI projection must follow the core document,
while the accumulated pact DNS minimum and revision survive core restart.
A byte-identical UCI-file assertion was incorrect: the adapter intentionally
rewrites that projection. No product change was needed to satisfy this contract.

Validation on an immutable snapshot with freshly built packages: 65 legacy-phase
browser tests pass (the three owned tests are skipped), then all three owned-phase
tests pass in a fresh container. Package installation/removal smoke checks, syntax,
9 shared-module checks and Go race/vet checks pass. The focused owned run also
passes independently; the phase guard skips all three without connecting to a
router. Broader migration, mixed-version, physical-device and restoration gates
remain open.

A follow-up fourth owned-browser check forwards the real apply RPC, then restarts
the daemon before delivering that response to LuCI. The real confirm RPC is
refused (`no pending confirmation`); LuCI shows the failure and retains the unsaved
form instead of reporting success or reloading it away. The confirmed document,
revision and UCI projection are restored; page reload displays the restored name.
All four owned checks pass together against the validated packages. This is a
browser interruption gate, not another claim of full restoration coverage.

### Android request failure evidence (historical diagnostics checkpoint)

At this checkpoint the CI grant-consumption failure after process restart remained
unexplained. It was subsequently traced to concurrent policy replacement during
startup reads and fixed in `7e600bb`; see [migration-safety.md](migration-safety.md). An unchanged local run using the failed CI packages passed; this is
not proof of an environmental/timing cause. Failure-only synthetic diagnostics
now preserve activity/service/power/usage state, router requests/usage and logs,
with CI artifact upload. Each collector is isolated so its failure cannot mask
the original assertion. Grant-input construction logs failures without changing
policy, enforcement or timeouts. Local validation: 199 Android unit tests,
`lintDebug`, `assembleDebug`, and an injected collector-failure check pass.
The first local build lost its Gradle daemon; a resource-bounded build passed.
No new emulator or physical-device success is claimed by these diagnostics.

**Root cause (2026-10-28, from those diagnostics of CI run 40490235715):** not an
environmental flake. After the same-UID kill, the `DEBUG_SYNC` broadcast restarted the
process; the sync stored a new reply while `AppControlService.tick` was reading the old
one. The tick parsed policy A, then re-read the cache for grant input, found reply B
("policy cache changed during enforcement read", visible at every process start in the
app log) and enforced **without grants**: the granted fixture was suspended at
11:03:45 (`PACKAGES_SUSPENDED`), Android closed its task and FamilyWarden's task came to
the front, so the rest of the granted minute was never used and the test timed out.
Fail-closed, but a functional bug: a child can be pushed out of an approved app.

Fix: every accepted sync stores a fresh nonce together with its clock, so an unchanged
nonce after all reads proves one consistent snapshot. If it changed, the tick keeps the
suspensions in force and re-reads at once (at most three times, then the previous
fail-closed path). `AppControl.replacedDuringRead` has a JVM test (205 tests pass);
three complete `test-requests-emulator.sh` runs passed 31/31 (a fourth stopped in the
emulator's network bootstrap before any test). The race is timing-dependent and was not
deterministically reproduced locally; the evidence is the CI diagnostic sequence.

### Legacy phone clock/day compatibility (core 0.13.0-r9 / LuCI 0.11.0-r4)

Legacy ownership now normalizes the signed phone's internet windows too, not only
the document-owned path. Only changed clock/day fields are rebuilt; unknown top-level
and per-window rule fields survive, and ordinary valid legacy policy bytes remain
unchanged. Day tokenization matches the kernel's ASCII separators: broader Java
whitespace cannot invent phone allow-days absent from the firewall.

New schedule/app-downtime day edits use that same grammar, and a pact amendment day
must be exactly one known weekday (not a compound string that happens to be a substring
of the weekday list). Unsupported app-downtime hours are also rejected by the LuCI
form before any write, for legacy and owned rendering alike. Unrelated legacy
normalization remains unchanged. Unit regressions reproduced the legacy `24:00`
reply and a Unicode-separated downtime that would be discarded; router tests verify
a real legacy-signed midnight window alongside its installed kernel rule.

### Authoritative configuration cutover: traced callers and required transaction

Do not import only the Profiles form and leave competing UCI consumers behind:

| Existing caller/writer | Cutover responsibility |
|---|---|
| `profiles.js` | **Owned mode moved:** edits/create/delete use core CAS; CBI's local UCI cache is only the form model, not a server-side writer |
| `settings.js` | **Owned mode moved:** core-profile choices and core CAS for global edits; UCI remains the display projection |
| `overview.js` | **Moved:** unassigned MACs use `list.configuration.assigned_devices`, not UCI |
| `devices.js` | **Read path moved:** labels/selectors (including disabled profiles) use the core `profiles` directory |
| `cli_dns_sync` | **Owned mode moved:** core allocates IDs in its edit transaction; desired DNS state and endpoint come from the document; Go owns AGH reconciliation |
| `profile_fragment` / `ulysses_sync` | **Owned mode moved:** Go materializes the effective snapshot; shell applies firewall/platform state without a reentrant socket call |
| `pact`, `pact.snap`, `pact.dns`, `release` | **Owned mode moved:** explicit import captures configured/frozen state and release; files become adapter projections |
| `block/allow/reset/ticket`, sail authentication | Keep platform enforcement/authentication behind the adapter; reconcile with core-owned pact/config transactions |

The next vertical slice must include a versioned configuration document and its real
readers/writers, not an unused parallel JSON store. Save/read-back must succeed before
marking import complete. Preserve profile IDs (and anonymous section identity where
referenced), MACs, existing DNS ClientIDs, disabled profiles, current vs frozen settings,
and any active release deadline. A corrupt or future-version document must refuse both
mutations and policy rather than fall back to stale UCI. Re-import must never overwrite
subsequent core edits.

Configuration writes need compare-and-swap against the read revision. Pact creation,
amendment, DNS allocation and configuration writes must share one transaction owner;
checking `PactCaptain()` and later writing a separate document is not sufficient. The
core must render/apply the candidate through the adapter, handle apply/save failures,
and retain a durable prior state for changes requiring connection confirmation (A20).
On crash/restart an unconfirmed change must reconcile/roll back; a browser timeout is
not confirmation. Validate this on a disposable real-procd router before replacing the
UCI UI or enabling cutover in production. The existing shell-to-core-to-shell policy
cycle must be removed or reduced to a stateless projection before this transaction is wired.

## Step 1 in detail

### Boundary

- **Core** implements the phone endpoint handler and the device/LuCI API. It never calls
  `uci`, `ubus`, `fw4`, `usign` or OpenWrt paths. All platform facts come through one
  Go interface, `platform.Adapter`, with exactly the methods step 1 needs:
  - `PolicyContent(profile, device, version, caps) ([]byte, error)` — interim: the
    signed payload's policy part (pact, rules, DNS, apps, `requires`) still computed by the
    shell. Removed in step 3.
  - `Profiles() ([]Profile, error)` — id, name, exists (validates bindings/scopes).
  - `PactCaptain() (string, error)` — profile bound by an active pact, or "".
  - `TimeZone() string`, `Now() time.Time`.
  A fake adapter in `core` tests implements the same interface (second consumer, A18).
- **OpenWrt adapter** (Go package `adapter/openwrt` in the core binary): implements the
  interface by running the shell CLI (`familywarden policy-json …`, `pact-captain`),
  reading UCI profiles and `system.zonename`. Only this package knows OpenWrt.
- **Transport, interim:** uhttpd keeps port 7700 (also the sail page and the lockdown
  exception). `cgi-bin/mast` becomes a forwarder: it passes the query string to the core
  socket and returns the core's reply byte for byte. URLs, parameters and replies are
  unchanged, so no phone notices. Later the core may bind the port itself.
- **LuCI, interim:** the Phones page calls the same ubus methods; the shell's `devices`/
  `device_*` handlers forward to the core over the socket (rpcd shim). LuCI is unchanged.

### Data

- Core store gets a versioned `devices.json`: id, name, profile, enrolled public key,
  administrator, approves, removal deadline. Pending enrollments, last fetch,
  acknowledgements and capability results stay in RAM (as today; reboot clears them).
- **One-time migration** on first start: UCI `config device` sections → `devices.json`,
  then the UCI sections are removed only after the core has persisted and verified the
  import. Idempotent; a failed import changes nothing and the shell path keeps working.
- **Router key:** the core reads the existing usign secret key (`/etc/familywarden/mast.key`)
  through the adapter and signs with it. No new key, no re-pairing. Requires a usign
  secret-key parser (unencrypted keys only; refuse anything else).

### Pact rules without a snapshot

Today device bindings are frozen by copying them into `pact.snap`, because UCI can be
edited directly. In the core store they can only change through the core API, so the
API enforces the same rules against `PactCaptain()`:

- no moving, deleting, re-keying or removal permission for a device bound to the captain;
- moving a device *into* the captain profile is allowed (it only strengthens the pact);
- no new parent scope over the captain; existing scope is kept but not signed;
- a captain's phone gets no parent keys.

If `PactCaptain()` fails, mutations are refused (fail closed); signing replies does not
weaken anything because policy content still comes from the pact-aware shell.

### Acceptance

- `scripts/test-integration.sh` passes **unchanged** against the core-backed endpoint
  and ubus methods (309 checks today): enrollment, fingerprint approval, pact freeze,
  v3/capabilities, refusals, parent scopes, acknowledgements.
- Go unit tests per rule with the fake adapter: signatures (v2/v3/caps), nonce/device
  binding, replay/stale, downgrade, capability gating, pact rules, migration and
  partial-failure rollback, usign secret key parsing (fixtures from real `usign -G`).
- Upgrade rehearsal: router at current version + enrolled emulator phone → new router;
  same router key, phone keeps syncing without re-pairing (`scripts/test-pilot-upgrade.sh`).
- Core down: the endpoint returns an error, never an unsigned or weaker reply; phones keep
  their last trusted policy. LuCI shows the core as not running.
- Measured on aarch64 before any production install: package size, binary size, idle and
  per-request RSS (local cross-build first; on rivendell only with explicit permission).

### Not in step 1

Requests/grants, profiles/pacts in the core, DNS sync, the shared UI. The Phones page is
the first candidate for the shared UI once its API is in the core.
