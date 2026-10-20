# App control — v0.9.2 baseline inventory (P0)

Recorded 2026-10-21 from `master` at `v0.9.2` (backend 0.9.2, LuCI 0.7.0), which is what
runs on rivendell. Purpose: know exactly what exists before the Go core takes anything over.
"Owner today" is the only writer; "Proposed owner" is a proposal (decision A6 is open).

## Router API (ubus object `familywarden`, rpcd shell plugin)

| Method(s) | Purpose | Owner today | Proposed owner |
|---|---|---|---|
| `list`, `status` | Effective profile state for LuCI/HA | shell | shell (OpenWrt adapter) |
| `block`, `allow`, `reset`, `ticket` | Whole-profile network overrides; `ticket` extends an active one | shell | shell |
| `pact`, `reload`, `version` | Ulysses pact, recompile, versions | shell | shell (pact semantics later shared) |
| `mast` | Pairing QR/key/fingerprint for LuCI | shell | Go (identity) |
| `devices`, `device_approve/reject/update/delete/allow_removal` | Phone enrollment and roles | shell | Go (identity) |
| `dns_status`, `dns_sync`, `dns_auth`, `dns_services` | AdGuard Home sync | shell | Go (P12) |

Every method must be advertised, dispatched, in both ACL files and return one JSON object
(`scripts/test-syntax.sh` enforces this). Release of a pact stays CLI-only (`sail-*`), never ubus.

CLI (`/usr/sbin/familywarden`): `reload`, `tick` (per minute), `dns-sync`, `dns-tick`,
`mast`, `mast2`, `mast-pubkey`, `sail-status`, `sail-request`, `sail-cancel`, `call`, `list`.

## Phone endpoint (port 7700, `cgi-bin/mast`)

- `?pubkey`: router signing key + fingerprint (pairing).
- `?device=<id>`: v1, legacy Ulysses Mast global captain state.
- `?device&nonce&key&sig[&name]`: v2. The phone signs `familywarden-sync-v2\n<id>\n<nonce>\n`
  with its own Ed25519 (usign) key. Reply: usign signature line + exact JSON payload.
- v2 payload fields: `v`, `server_time`, `nonce`, `device`, `enrollment` (pending/approved),
  `timezone`, `binding{device_id, profile_id, profile_name, captain, administrator,
  can_self_remove}`, `captain`, pact rules, typed DNS policy.
- Gap for app control: no request/upload channel (usage, extra-time requests) and no
  applied-state acknowledgement; replies are policy only. Endpoint is LAN/VPN only.

## Persistent and RAM state

| Path | Kind | Contents | Owner today |
|---|---|---|---|
| `/etc/config/familywarden` | UCI | settings, profiles, schedules, devices | LuCI forms + shell |
| `/etc/familywarden/pact`, `pact.snap`, `pact.dns`, `release` | persistent | pact, frozen snapshots, release request | shell |
| `/etc/familywarden/mast.key`, `mast.pub` | persistent, root-only | router signing key | shell |
| `/etc/familywarden/agh.auth`, `agh.owned`, `dns.ack` | persistent, root-only | AGH login, owned ClientIDs, ack | shell |
| `/tmp/familywarden.state` | RAM | overrides, tickets, removal deadlines | shell |
| `/tmp/familywarden.pending/` (max 10), `.seen/`, `.mast` | RAM | pending enrollments, last fetches | shell |
| `/tmp/familywarden.dns.{dirty,status}` | RAM | AGH sync state | shell |
| `/var/run/familywarden/familywarden` | RAM | effective (pact-resolved) config | shell (`ulysses_sync`) |
| `/var/etc/familywarden.include` | RAM | generated nft include | shell |
| Locks: `/var/lock/familywarden.lock` (+`.ticket`), `familywarden.dns.lock` | — | serialization | shell |

UCI options in use: `enabled default_policy ulysses_profile ulysses_delay dns_domain agh_url`
(settings); `name enabled policy block_router dns dns_client_id dns_protection dns_service device`
(profile); `profile days start stop` (schedule); `id name profile administrator pubkey` (device).
There is no app-control state anywhere yet.

## Android app (`io.github.denogio.familywarden`)

minSdk 30, targetSdk 35; Device Owner. `PhoneKey` (Ed25519 seed in app-private prefs),
`Sync`/`Payload`/`DevicePolicy` (v2 verify, nonce, device binding, no downgrade),
`Store` (last trusted policy, no age expiry), `Mast` (tethering + time restrictions),
`PrivateDns`/`AppliedDns`/`DotProbe` (strict DoT apply/lock/probe), Compose UI.
Not present yet: usage collection, package suspension, requests, parent role, notifications.

## Tests and environments available

- Fast: `scripts/test-syntax.sh` (+ `test/luci-ui.test.cjs`), Android unit tests/lint.
- Container: `scripts/test-integration.sh` (192 checks, real fw4/nft), `scripts/test-agh.sh`
  (throwaway AdGuard Home), `scripts/test-apk.sh`, `scripts/test-e2e.sh` (Playwright, 29).
- Slow (once per milestone): `scripts/test-mast-emulator.sh` (~15 min, Android 15).
- QEMU router `scripts/qemu-dev.sh`; disposable LuCI preview container `fw-overview-dev`.
- Physical: user's Pixel 10 (pilot, Device Owner). The son's Pixel 7 Pro is **not** a test device.

## Stale artifacts

- `.understand-anything/knowledge-graph.json` describes `66c2701` (pre-Ulysses). Do not
  trust it for current code; refresh after the Go architecture settles.
- `plans/roadmap.md` phase table predates 0.9.x; its app-control section is current.

## What P1 must decide before code

See `plans/app-control-todo.md` P1. Blocking the first vertical slice: A6 (Go/shell split),
the extra-time protocol (who signs grants, offline behaviour), rule precedence, usage
definition and day boundaries, retention (A8) and request limits (A7). Off-home transport
(A2) only blocks P9, not the LAN slice.
