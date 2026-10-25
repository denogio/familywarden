# Core migration plan (A6, A18–A20)

Goal: all product logic in the Go core (`core/`), router specifics in a thin adapter,
one shared administrator UI. The shell plugin (`familywarden/usr/lib/rpcd/familywarden`)
is the interim OpenWrt implementation and only shrinks. Decisions: `app-control.md`
A6 (core owns identity/policy/DNS/apps), A18 (router-agnostic), A19 (core-owned store),
A20 (shared UI embedded natively, on OpenWrt via a LuCI view + rpcd shim).

## Order

| Step | Moves to the core | Stays in the shell meanwhile |
|---|---|---|
| **1** | Phone identity: enrollment, device registry, roles/parent scopes, capability check, acknowledgements, request verification and reply signing | Policy *content* (pact, schedules, DNS, apps), firewall, sail page |
| 2 | Requests and grants (P8), built in the core from the start | — |
| 3 | Profiles, schedules, app rules, pacts and the effective-policy computation | Firewall compiler reading the effective config |
| 4 | DNS (AdGuard Home) sync | — |
| 5 | Optional: firewall compiler as a Go OpenWrt adapter; sail page | init/rpcd/LuCI wrappers |

The existing seam for step 3/5: shell enforcement already reads only the effective
config (`fw_load` → `/var/run/familywarden/familywarden`). The core will produce that;
the adapter turns it into nftables. Other platforms get their own applier (pf).

## Status

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
checks and the full disposable OpenWrt suite (45 passed, one existing skip). A regression
probe restores the previous Settings source and confirms that the core-only profile
selector test fails. Tests used an immutable source snapshot and matching existing
development packages with the Settings source overlaid; no new package deployment,
physical-phone change or live-router change occurred.

### Authoritative configuration cutover: traced callers and required transaction

Do not import only the Profiles form and leave competing UCI consumers behind:

| Existing caller/writer | Cutover responsibility |
|---|---|
| `profiles.js` | Replace CBI profile, schedule, app/downtime and delete-cascade writes with the core API/shared UI |
| `settings.js` | **Profile read path moved:** captain choices/DNS labels use core `profiles`; global form fields still use UCI and need the future atomic writer cutover |
| `overview.js` | Replace the UCI profile/device enumeration used to distinguish unassigned MACs |
| `devices.js` | **Read path moved:** labels/selectors (including disabled profiles) use the core `profiles` directory |
| `cli_dns_sync` | Replace lazy `dns_client_id` UCI writes with a core-owned allocation; preserve existing IDs exactly |
| `profile_fragment` / `ulysses_sync` | Project one core-owned effective snapshot; no socket call back into a service waiting for this adapter |
| `pact`, `pact.snap`, `pact.dns`, `release` | Import one consistent active pact, frozen policy/DNS and release deadline, not just current UCI |
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
