# FamilyWarden Android implementation checkpoint

Local work on `feature/familywarden-android`. This is **not a production release or
an in-place update to the installed Mast app**. No live router, AdGuard settings,
existing Device Owner enrollment, or user pact is changed by this branch's tests.

## Implemented locally

- New application ID `io.github.denogio.familywarden`, visible FamilyWarden identity,
  lighthouse vector/adaptive/themed icon, Compose Overview / DNS / Settings, and an
  optional Ulysses destination. Existing policy evaluator and Mast components are reused.
- English and Danish resource sets; system/app language selection through Android's
  native locale configuration. Archived design mockups remain English and unchanged.
- Separate policy, Android DNS read-back, network-bound resolver-health, and router-sync
  facts. Missing association, missing policy, unlocked/mismatched settings, and a test
  from another network/generation cannot produce the positive DNS headline.
- Captain eligibility and administrator/self-removal capabilities are separate. An
  applicable active pact forces Ulysses visibility and prevents disabling, re-pairing,
  and removal. Managed-device no-pact alone does not authorize removal.
- Prototype parsing/tests for a signed v2 response with device binding, request nonce,
  timezone, strict rules, and optional typed DNS policy. The enrollment latch and payload
  are committed atomically; an enrolled device cannot downgrade to global v1 policy.
- Re-pairing verifies the candidate reply before replacing stored credentials/cache,
  and rechecks existing authorization immediately before replacement. Final owner
  removal also re-fetches authorization inside the owner-action implementation. That
  fresh decision uses signed server time, so a fast phone clock cannot authorize
  removal of a fixed-duration pact.

## DNS-independent control endpoint

`rivendell.lan` is known to dnsmasq but returned NXDOMAIN from AGH during a read-only
check. The numeric port-7700 endpoint was reachable. Therefore:

- LuCI's existing pairing metadata now adds `address=<numeric LAN IPv4>` when a
  hostname was supplied and a valid LAN address can be discovered. Legacy `host`,
  `key`, scheme, and key fingerprint remain compatible.
- New Android code prefers and validates that numeric endpoint. The original hostname
  can remain display metadata, but is not needed for sync. Manual/legacy hostname
  pairing resolves once before changing stored pairing; subsequent control requests
  use the numeric address and the same pinned signing key. Captive-portal HTTP
  redirects are refused, not treated as alternative control endpoints.
- This does not discover a changed router IP automatically. A stale address fails sync
  and retains the last verified policy; no guessed gateway or unfiltered DNS fallback
  is introduced. LAN/VPN routing is still necessary; pinning an IP does not itself
  force Android to keep using home Wi-Fi if it prefers a different default network.
- Before rollout, design/test same-key control-address repair for changed LAN IPs,
  separately from re-pairing to another router. It must require a newly verified,
  device-bound reply and cannot discard a cached applicable pact or change the trust
  key merely because the old address is unreachable. This is not implemented here.
- Optional AGH forwarding `[/lan/]10.77.0.1` is **not applied**. It changes shared AGH
  upstream behavior, beyond the approved FamilyWarden-owned client boundary, and needs
  separate explicit production approval.

## Enrollment, device policy and DNS (backend 0.9.0 / LuCI 0.6.0)

Implements the decisions recorded in `plans/dns-filtering.md` (2026-10-17):

- **Enrollment:** the app creates its own usign (Ed25519) key and signs every request
  (`familywarden-sync-v2\n<id>\n<nonce>\n`). Unknown or re-keyed phones wait in RAM
  (at most 10) and appear under LuCI › FamilyWarden › Phones with a **SHA-256 of the
  full key** (usign's 8-byte key ID is caller-chosen, so it is not used for comparison).
  Approval sends the fingerprint the administrator saw; a changed key is refused.
  Only an approved id+key pair gets a binding; everything else gets a signed "pending".
- **Pact freeze:** phones of the captain profile are part of the pact snapshot and can't
  be moved, deleted or given removal permission until the pact ends (also not in the
  amendment window). New phones may still join the captain profile.
- **DNS under a pact:** the captain's DNS settings (on/off, protections, services,
  ClientID, DNS domain) are snapshotted when the pact is made. While it lasts, the
  AdGuard client gets the snapshot **plus** anything added since: tightening applies at
  once, removals and switching off wait until the pact ends or its weekly window.
  Clearing the AdGuard Home settings does not drop the phone's policy either.
- **Removal:** administrator phones may remove the app when no applicable pact is
  active; other phones need LuCI "Allow removal" (1 hour). Removal re-fetches a fresh
  signed reply and uses its server time.
- **AdGuard Home:** profiles with "Strict DNS" get a `familywarden-<ClientID>` client;
  only those clients and ClientIDs FamilyWarden added to `allowed_clients` are changed
  (re-read before, verified after; an empty list is never touched). The ClientID comes
  from the profile name once and is kept on rename. Phones receive DNS only after AGH
  acknowledged the client. Credentials: write-only via LuCI, root-only on the router.
- **Android DNS:** strict Private DNS to `<ClientID>.<dns_domain>`, locked with
  `DISALLOW_CONFIG_PRIVATE_DNS`. Android refuses a resolver it can't reach right now;
  the app then reports the failure instead of protection. Removal sets Automatic and
  unlocks. The DoT connection test is bound to network, policy and generation.

## Pilot deployment (2026-10-18, user-authorized, after the pact had expired)

- rivendell upgraded 0.8.0 → backend 0.9.0 / LuCI 0.6.0. Installed code identical to
  the branch; all profile statuses and UCI/firewall/DHCP fingerprints unchanged.
  Backup: `~/Backups/familywarden/rivendell-20261018-080111` (includes AdGuard YAML before/after).
- DNS enabled only for the user's own (captain) profile, with filter lists, Safe
  Browsing, parental and SafeSearch. AdGuard diff: one new `familywarden-<id>` client and
  one new `allowed_clients` entry; nothing else changed. A pre-existing user allow rule
  for `www.google.com` overrides SafeSearch for Google (global and per client); untouched.
- User's Pixel 10 (Android 17): old Mast removed through its own no-pact removal,
  accounts removed, signed `0.9.0-pilot` (stable key) set as Device Owner, accounts
  re-added, paired via QR, fingerprint approved in LuCI as administrator.
- Verified on the phone: strict Private DNS `<id>.secure-dns.hojgaard.me`, locked; on
  Wi-Fi **and on cellular only**, normal names resolve and a blocked adult domain does
  not (strict mode has no fallback, so lookups went through AdGuard over DoT). #32 closed.
- Ulysses is off in the new app until the user opts in under Settings → Features.

## Remaining gates (before rollout)

1. Real device: hotspot restriction under a new pact, and a reboot without the router.
2. Positive in-app DoT probe against a suitable publicly trusted resolver/certificate:
   not covered by the current disposable resolver fixture.
3. Physical startup-clock and before-first-unlock behavior, plus real network/address
   transitions. Same-key numeric address repair is emulator-verified (see below), not
   a completed physical-device gate.
4. Choose default protections/services per profile (#33). No AGH or router change has
   been made by this work beyond the approved `familywarden` AGH user.

## Runtime follow-up (2026-10-26)

The resource-capped Android 15/OpenWrt suite passes all 98 successful checks, including
same-key address repair while pact-bound. Foreign QR keys, unreachable endpoints and
unsigned repair replies preserve the old endpoint/pin/device identity and restrictions;
fresh signed replies permit address repair and return without re-enrollment. The
signed request/grant/new-app/ordinary-parent suite also passes its 31 checkpoints.

This validates the existing phone-policy path with the selector-checkpoint debug APK
and matching core 0.9/backend rc7 development artifacts. Emulator 37's API35 network
bootstrap now explicitly uses user-mode Wi-Fi and waits for TCP connectivity. Tests
read sync observations through the core API, not removed shell RAM files. Both suites
run on immutable snapshots and remove their containers/networks afterward. No physical
phone, live router, deployed policy, signing key or production package was changed.
The remaining physical and positive-resolver gates above stay open.

## Verification at the original checkpoint

- Router: 178 integration checks (real fw4/nftables, enrollment, freeze, removal,
  crafted names); 49 AdGuard Home checks against a throwaway v0.107.79 container
  (same image digest as production).
- Android: unit tests, debug build and lint pass. Emulator (Android 15): 81 checks,
  starting from a 0.8.0 router that must not authorize the phone, upgraded in place to
  0.9.0, then pending → fingerprint matches LuCI → approval → pact enforcement → DNS
  refused by Android and reported → offline reboot → removal restores Private DNS.
- LuCI Playwright: 17 tests against the 0.6.0 package.
- The former insecure v2 test fixture has been deleted.

Screenshots use fictional names only. Diagnostics omit names, ClientIDs, addresses,
keys and payloads.

Keep this checkpoint and `plans/dns-filtering.md` up to date as the implementation
advances. Do not interpret local build/test success as deployment authorization.
