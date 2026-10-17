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

## Important remaining gates

1. **The shipping router endpoint is still v1.** Its global captain response is useful
   for diagnostics but cannot authorize DNS, captain status, or managed removal on the
   new app. The new UI explicitly reports the incompatible router setup.
2. **Prototype v2 parsing is not an enrollment/authentication protocol.** A random
   device ID and a nonce do not authenticate a phone. Before a production v2 endpoint,
   implement approved enrollment proof, device/capability authorization, replay/freshness
   protections, and managed-device administrator approval. Never return privileges just
   because a caller supplied a profile or device ID. Freeze device/profile association
   changes that could escape an active captain pact, negotiate protocol compatibility,
   and only report filtering as applied after acknowledged AGH configuration, not from
   the requested settings alone.
3. **Strict DNS writes and resolver probing are not implemented in this checkpoint.**
   Android settings are read back rather than inferred from receipt of policy; resolver
   testing is disabled. Finish the strict-DNS/lifecycle work and release/restoration
   handling before claiming this app provides DNS enforcement. Also test startup clock
   integrity (including a wrong clock before enrollment): automatic time configuration
   alone is not proof that the OS clock has already synchronized before the initial
   hotspot decision. Include before-first-unlock reboot behavior and alarm recovery,
   not only an emulator reboot followed by unlocking the screen. Do not introduce a
   24/48-hour stale-cache escalation.
4. AdGuard integration still needs an authenticated management route, mapping to actual
   supported client controls, preservation of unrelated configuration, and approved
   defaults/exceptions. No invented per-client filter-list groups.
5. Physical-phone cellular DoT, hotspot behavior, and a separately authorized Device
   Owner migration remain outstanding. A different application ID cannot replace the
   existing owner in place. Never release/reset the current owner to try this branch.

## Verification at this checkpoint

- **81 ordinary Android unit tests passed** (the original 36 plus 45 implementation
  and transport cases). The opt-in archive review suite passes **100 tests**, including
  its 19 characterization/proposal cases; a subsequent ordinary run contains only 81.
- Debug and **unsigned** release builds plus `lintDebug` pass. The shrunk release is
  approximately 4.13 MiB. The release manifest has the new ID/AdminReceiver, native
  locale configuration, and no debug receiver. No signed release was published.
- Both resource sets contain all **176** translatable strings. Android 15 native locale
  switching, English/Danish, dark/light, and 200% text were exercised in the real UI.
- The isolated emulator suite passes: `router.invalid` pairing through a numeric
  address, real v1 incompatibility, scoped v2 fixture enforcement, optional/forced
  Ulysses visibility, protected data/uninstall, stale removal confirmation rejection,
  offline reboot, and authorized Device Owner release. This is not an AGH/DoT test.
- **124 backend integration checks passed**, including static/live-address metadata
  paths. Shell syntax, Markdown links and whitespace checks pass. The OpenWrt package
  explicitly depends on `jsonfilter`; a fresh SDK package build remains a release gate.
- Review follow-up also covers DST gaps/repeated hours, bounded epoch seconds (not
  milliseconds), malformed DNS-object rejection, typed refusal causes,
  missing-policy vs v1-incompatibility copy, and
  fresh removal decisions using signed server time.

## Isolated tests

- `scripts/test-integration.sh`: real fw4/backend integration, including numeric pairing
  metadata, unchanged key identity, backward-compatible URI fields, and invalid-address
  rejection. The container's test UCI changes do not affect any router outside it.
- Android ordinary unit tests: existing policy/signature tests plus pairing, scoped-policy,
  independent-state, and weekday/overnight/exclusive-end schedule display tests.
- `scripts/test-mast-emulator.sh`: disposable Android 15 and router containers only.
  The initial real v1 response must remain unsupported; the script then explicitly
  installs `test/android/device-policy.cgi` to exercise prototype v2 behavior.
- **The test CGI deliberately authorizes arbitrary valid test-device IDs. It is not
  secure production code**, is not installed by either OpenWrt package, and refuses
  operation without a disposable-lab sentinel. Passing its tests does not validate
  enrollment authentication or real AdGuard integration.
- Screenshots must use fictional fixture names only. Diagnostics omit names, ClientIDs,
  addresses, keys, and payloads; still review before sharing.

Keep this checkpoint and `plans/dns-filtering.md` up to date as the implementation
advances. Do not interpret local build/test success as deployment authorization.
