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

**Step 1 done (2026-10-24), not deployed.** Packages `familywarden` 0.11.0_rc1 (depends on)
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
aarch64 RSS must still be measured on the router (needs explicit permission).

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
Authenticated consumption uploads, shared allocations and the shared administrator UI are
still unimplemented.

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
