# DNS filtering — AdGuard Home integration plan

Status: proposed, not implemented. No changes to rivendell are authorized by this plan.
The current pact test must remain untouched. Documentation and default UI text are English.

## Planning checkpoint — read this before continuing

This document is the durable source of DNS planning context. Keep confirmed facts,
proposals, and unanswered questions separate. A planning answer is not production-change
authorization. Record answers under the decision IDs below rather than relying on chat history.

### Preparation work authorized by the user: steps 1–3

- **Step 1, scope:** approved choices are consolidated below. Exact filtering categories,
  removal roles, and security decisions remain explicitly deferred, not guessed.
- **Step 2, investigation:** accessible read-only inventory is complete; see the
  [sanitized preflight report](dns-preflight-report.md). Real cellular confirmation and
  authenticated API setup still need user participation/approval. No settings changed.
- **Step 3, design review:** the [supplied handoff](../docs/design/familywarden-app/README.md)
  is archived with its [review](../docs/design/familywarden-app/REVIEW.md).
  19 opt-in review tests plus 36 existing Android tests pass. The original model is
  not production-ready; review helpers are test-only, not implemented app features.
- Backend implementation, Android migration/enforcement, and production rollout are
  **outside this authorization**. Continue only after the next explicit go-ahead and
  resolution of the gates relevant to that work.

### Confirmed environment and observations

| Item | Confirmed finding | Limits / remaining verification |
|---|---|---|
| Router | rivendell, OpenWrt 25.12.4, FamilyWarden 0.8.0 / LuCI 0.5.0 | A user-run pact test is in progress; do not alter it |
| Existing resolver | AGH `v0.107.79` in Docker at `10.77.0.11`; filtering/access settings inventoried read-only | API requires auth; management credentials/ownership not approved. Existing client IDs/names remain private |
| DNS names | `secure-dns.hojgaard.me` and wildcard subdomains resolve publicly; tested base and `mast-phone.secure-dns.hojgaard.me` | Public address was `87.62.96.86`; use DNS, not a hard-coded public IP |
| DoT | Both tested hostnames accepted TCP 853, TLS 1.3, and a real `example.org A` query | Tested from the home network through the public address, not from an independent external network |
| Certificate | Trusted certificate covers `secure-dns.hojgaard.me` AND `*.secure-dns.hojgaard.me` | Automatic renewal and behavior after renewal still need testing |
| WAN forwarding | Active TCP/UDP 853 DNAT to `10.77.0.11:853` | The phone cannot use it on mobile data yet; root cause is not established |
| NAT reflection | Wi-Fi connections through the public address are SNATed to `10.77.0.1` | AGH therefore cannot infer the original LAN IP on that path |
| IPv6 | No AAAA answer for the tested public resolver name; latest inspection found active global/ULA/link-local router DNS listeners and DHCPv6/RA server mode | Earlier WAN snapshot was incomplete/outdated; actual Android IPv6 bootstrap remains untested |
| Mast | Device Owner app, minSdk 30, already paired using a pinned router signing key | Private DNS enforcement is proposed, not implemented |

No DNS rewrites, DHCP changes, AGH access-list changes, or Mast DNS settings were applied.
The prior router-upgrade troubleshooting touched only FamilyWarden JS timestamps;
it did not configure split DNS.

### Proposed architecture, not yet approved in every detail

- Reuse AGH at `10.77.0.11`; do not install a second AGH on rivendell.
- Keep OpenWrt DHCP. Ordinary LAN devices use stable IP identities mapped from their
  FamilyWarden MAC membership. Confirm DHCP reservations before changing them.
- Add a separate Mast workstream: Device Owner sets a **DoT hostname** through
  `setGlobalPrivateDnsModeSpecifiedHost` and prevents user edits using
  `DISALLOW_CONFIG_PRIVATE_DNS`. This API does not accept a generic DoH URL.
- Approved ClientID model: derive a DNS-safe unique identifier from the profile's display
  name when DNS is first configured, persist it, and bind it to that profile. The hostname
  is `<stored-client-id>.<configured-DoT-domain>`; later display-name edits must NOT
  regenerate the ID or hostname. Example: `alex.dns.example.net` (fictional data).
  Normalize invalid DNS characters and handle collisions before first assignment; do not
  overwrite another managed/manual client's identity. Any future ID rotation is explicit.
  AGH derives the ClientID from TLS SNI; NAT changes addresses, not SNI. Verify any TLS
  proxy preserves the required identity.
- **ClientID is identification, not authentication.** Anyone able to use a known ID can
  attempt to select its policy. TLS authenticates the server to Android, not the phone
  to AGH. Do not claim that a random ID makes a publicly accessible resolver private.
- Proposed split DNS on rivendell's LAN-only dnsmasq:

  ```uci
  list address '/secure-dns.hojgaard.me/10.77.0.11'
  list local '/secure-dns.hojgaard.me/'
  ```

  These are entries inside the relevant dnsmasq section, not commands to execute now.
  The suffix covers the base name and all subdomains. `local` prevents unmatched record
  types, including AAAA without a local IPv6 answer, being forwarded upstream.
  Do not replace existing address/local lists or unrelated DHCP DNS options.

- Wi-Fi bootstrap DNS must actually reach rivendell for these overrides to apply.
  Read-only inventory found no explicit LAN DHCP option 6 or DNS option, with dnsmasq
  listening on the router. Default router advertisement is expected, but actual Android
  bootstrap still needs checking. If clients query AGH directly, dnsmasq overrides alone
  have no effect. See the preflight report for the mixed existing upstreams.
- Do NOT add a global private-IP rewrite to the same AGH listener serving remote clients:
  it could return an unreachable private address off-site. Keep external authoritative
  answers public, and keep the private override confined to the LAN bootstrap path.
- Split DNS improves the direct local path; it is not the security boundary. Android
  caches/bootstrap behavior on network changes must be tested. Identity must remain
  correct through ClientID even when a connection temporarily uses NAT reflection.
- Do NOT simply remove reflection SNAT: asymmetric routing can break the connection.

### Confirmed product decisions

- The Android app must ship with complete **English and Danish** localization.
  English is the resource fallback; follow Android's system/per-app language selection.
  Include strings, plurals, accessibility labels, setup, dialogs, and user-facing errors;
  use locale-aware formatting and test Danish layouts at large text sizes. Documentation
  and source comments remain English. This is required for the Android release, not
  deferred to the separate LuCI translation roadmap phase.
- DNS filtering is always active while paired, independent of Ulysses pact/schedule state.
- The user approved the pilot identity: their user is **administrator**, and their
  profile is **captain**. These remain independent roles, not synonyms.
- Removal: the authorized administrator may remove their own app when no applicable pact
  is active. Managed devices require administrator approval; lack of a pact does not
  grant self-removal. An active applicable pact remains a barrier. Approval transport
  and authentication still need design (D12/D14); non-admin does not imply child.
- Keep strict Private DNS: no automatic opportunistic/plaintext/unfiltered fallback.
  Redundant filtered upstreams inside AGH, and possibly a second equivalently configured
  AGH later, may improve availability without relaxing this decision. Port-853 blocking
  and captive portals remain separate problems, not reasons to silently disable policy.
- First rollout is the user's own phone only. Filtering must be selectable per profile,
  not one mandatory fixed strict preset. Separate supported protection options, SafeSearch,
  and individual service blocks. Exact available categories/defaults remain to be mapped
  to real AGH capabilities; do not invent independently selectable blocklist groups.
- FamilyWarden may modify only its own namespaced AGH clients and owned client settings.
  Existing clients, global subscription lists, and unrelated settings must remain untouched.
  New category requirements must respect this ownership boundary or return for approval.
- Rename the Android product to **FamilyWarden**, with a separate **Ulysses** section.
  The user also wants a new Android package/application ID now, rather than retaining
  `me.hojgaard.ulysses.mast`. Approved new ID: `io.github.denogio.familywarden`,
  avoiding a personal-domain namespace. Source packages,
  manifest component names, tests, CI scripts, enrollment instructions, and pairing
  protocol naming must be inventoried together before implementation.
- A changed application ID is a different Android app, not an in-place APK update,
  even with the same signing key. If the old app is enrolled as Device Owner, use its
  authorized no-pact removal/relinquishment flow before enrolling the new package;
  provisioning constraints may recur. Never bypass an active pact for migration.
  Retain the existing release signing key unless a separate key-change decision is made.
- Public project defaults/docs must not hard-code the user's personal DNS domain.
  Treat the existing `secure-dns.hojgaard.me` as deployment configuration, supplied via
  pairing/signed policy. Whether to replace the live DNS domain too is an open user
  decision; no replacement domain is selected or claimed to be owned.
  No code, package, enrollment, or DNS records are changed during planning.
- The user supplied the FamilyWarden logo design: a lighthouse with a gold lamp/beams,
  mist/slate tower, and wave, using the existing night-sea palette. Original self-contained
  reference (including bundled-font license notices):
  [FamilyWarden logo design](../docs/design/familywarden-logo.html).
  It includes light/dark wordmarks, full and simplified small marks, adaptive Android
  icon variants, a monochrome/themed mark, notification icons, and LuCI examples.
  FamilyWarden uses the lighthouse; Ulysses retains the mast/sail as a subsection mark.
  These are design references, not implemented assets or approval to replace LuCI's theme.
  The illustrated block page does not imply DNS can display a custom page for arbitrary
  HTTPS sites without certificate errors. No app redesign, deployment, or new logo
  generation is authorized yet.

### Remaining decisions (ask the user in small rounds)

| ID | Question | Proposed default / reason | State |
|---|---|---|---|
| D1 | When is phone DNS filtering active? | Always while paired, independent of pact/hotspot schedule | Confirmed |
| D2 | If strict Private DNS is unreachable, accept DNS failure or allow fallback? | Keep strict DNS; no automatic unfiltered fallback. Explain outage/captive-portal consequences | Confirmed |
| D3 | Which devices/profiles and filters belong in the first release? | User's phone only; selectable per-profile protection options rather than a fixed strict package. Map categories/services/defaults to AGH support | Scope and configurability confirmed; capability mapping open |
| D4 | Should tickets affect DNS filtering? | No: internet tickets change connectivity only | Unanswered |
| D5 | How is the external resolver access-controlled? | Nonempty AGH allowlist confirmed; LAN and specific IDs allowed. Agree intended exposure/authentication; real phone cellular failure still needs correlation | Inventory done; access decision open |
| D6 | Can selected LAN devices get stable DHCP reservations, and how should IPv6 be handled? | Two reservations already exist; do not reassign them. Active IPv6 found. Reserve future selected devices only with approval and test IPv6 | Inventory done; changes undecided |
| D7 | May FamilyWarden change DHCP-advertised bootstrap DNS to rivendell if necessary? | No explicit LAN DNS override found; actual Android bootstrap still needs testing. Split DNS is not configured. No whole-network change without approval | Router inspection done; Android test/open approval |
| D8 | How should pact freezing, amendment windows, unpairing, and removal affect DNS? | Freeze the effective policy/hostname during the pact; allow normal amendments; restore the previous owned Android DNS setting on removal | Unanswered |
| D9 | Which AGH settings may FamilyWarden own? | Only its own namespaced clients and owned client settings; preserve global lists and existing clients | Confirmed |
| D10 | What is the deployment sequence? | Lab first, then one explicitly approved device after the current pact test | Unanswered |
| D11 | Which actor may remove/unpair a device? | Pilot user is admin + captain; own-app removal allowed without an applicable pact. Managed-device removal requires admin approval; captain does not itself imply admin | High-level policy confirmed; protocol open |
| D12 | How does the router authenticate an enrolled phone? | Pinning the router key authenticates the router, not the requesting phone. Agree authorized enrollment and per-device proof of identity before capability/removal endpoints | Deferred |
| D13 | How strong must Android bypass resistance be? | Decide ADB, alternate users/profiles, VPNs, app-level DoH, and scope of always-on restrictions; current hotspot-time ADB rules do not automatically protect always-on DNS | Deferred |
| D14 | How is release/removal response freshness proven? | A valid signature or nondecreasing timestamp alone does not prove a response is live. Evaluate request-bound challenge/expiry and replay tests before authorization-sensitive implementation | Deferred |
| D15 | Which policy changes/exceptions may be immediate? | Decide authority and possible delay for AGH-side changes, manual allow rules, and DNS exceptions; external AGH admin access can bypass local UI freezing | Deferred |
| D16 | What are the privacy and recovery policies? | Agree query-log retention, diagnostic redaction, certificate/signing-key rotation, router replacement, lost phone, and recovery ownership | Deferred |

The user explicitly asked to record unresolved questions and revisit them only when
relevant, not resolve everything now. D1/D2 are settled. Keep the rest as visible gates:
resolve each question before implementing the behavior it governs. Do not invent a
role matrix or treat proposed defaults as user decisions. No implementation/deployment
is authorized by this backlog.

### Deferred-question timing

- Before enrollment/capability/removal implementation: D11, D12, and D14.
- Before always-on Android enforcement: D13 and DNS restoration details in D8.
- Before exposing remote DNS/API or changing network configuration: D5–D7 and D16 security/recovery details.
- Before activating filtering or exceptions: D3, D4, D9, D15, and applicable pact-amendment rules.
- Before deployment: D10 plus every unresolved gate affecting the enabled features.

D11 now has an approved high-level rule: admin self-removal without an applicable pact;
managed-device removal needs admin approval. The pilot user is both admin and captain,
but the roles remain separate. How approval is authenticated, whether it happens on
another device, and how it is bound to a live removal request remain D12/D14 work.
Existing Mast behavior is not automatically the future FamilyWarden permission model.

ClientID naming is approved: use a DNS-safe unique form of the profile name **at the
moment DNS is first set up**, store it separately, and keep it fixed and profile-bound.
Display-name changes do not rename the DNS identity. Do not derive it afresh during sync.
This is profile-level identification, not authentication or an administrator credential.
Per-device revocation if multiple phones later share one profile remains a future design
question; it is not required for the initial one-phone rollout.

### Read-only investigation still needed

- Completed: AGH version/hosting, published DNS/TLS ports, sanitized global filtering,
  access-list structure, query-log retention setting (not logs), and router DNS configuration.
  Remaining: authenticated API route, intended management permissions, full TLS/proxy and
  renewal audit. Do not request passwords in chat or publish query logs.
- Inspect DHCP option 6, IPv6 RDNSS/DHCPv6, and actual Android bootstrap behavior.
- Diagnose the mobile-data failure using a coordinated TCP/TLS attempt. Capture only
  necessary metadata; distinguish no inbound SYN, failed return path, TLS failure,
  and DNS refusal. Port forwarding alone does not prove external reachability.
- Verify any AGH access-list ClientID support against the installed version. Even if
  supported, it remains a bearer identifier, not strong device authentication.
- Independently test cellular access; do not count the successful LAN hairpin test as
  an external test.

### Mast implementation requirements (separate from router DNS interception)

- Extend the signed router payload with a versioned DNS policy/hostname and update
  `Payload`, `Policy`/`Store`, `Mast`, UI state, and removal handling together.
  Validate hostname and policy fields before accepting them. No credentials in payloads.
- Preserve the agreed offline model: last verified policy remains until successful sync;
  no age-based 24/48-hour escalation. Per D1, DNS policy is always active while paired,
  independently of the current pact or hotspot decision.
- Set Private DNS in background work: Android's API performs a blocking resolver
  validation. Handle return codes, unsupported ownership/user configurations, and network
  failures explicitly. Never mark enforcement successful just because desired state was saved.
- Distinguish applied DNS policy, endpoint reachability, and sync freshness in the UI.
  Preserve prior settings and verify which modes the public Android APIs can restore;
  do not promise exact restoration until tested. Restore only settings Mast owns,
  document platform limitations, and do not clear unrelated restrictions.
- Test whether another browser/app can use its own DoH or VPN. System DNS control does
  not automatically cover those, and phone Private DNS does not prove hotspot-client DNS
  enforcement. Do not conflate these with the already implemented hotspot restriction.
- Test split-DNS bootstrap and DoT across Wi-Fi → cellular → Wi-Fi, reconnect/reboot,
  cached public/private endpoints, API timeout, port-853 blocking, captive portals,
  certificate renewal, and resolver outage on real Android hardware.
- Add emulator tests for policy application/change locking/removal, and real-device
  acceptance for network switching. A new Mast release/signature-compatible update is
  required; installing a router package alone cannot add phone-side enforcement.


## Recommendation

Use **AdGuard Home (AGH) for DNS filtering**, with FamilyWarden managing profile-to-client
policy and nftables enforcing the DNS path. Keep OpenWrt dnsmasq/odhcpd responsible for
DHCP and local network discovery initially. Do not build a DNS filter or blocklist engine.

The previous recommendation was per-MAC nft DNS redirection to a public filtering
resolver (for example Cloudflare Family). That is smaller and has no local filtering
service, but offers less control over exceptions, lists, reporting, and per-client policy.
AGH is the better fit if those controls are wanted. Redirection and AGH are complementary,
not competing mechanisms: one enforces where queries go; the other filters them.

Start with **one AGH backend**, not a pluggable framework for several providers.
AGH remains an optional dependency; existing internet on/off behavior must not change
for installations that do not enable DNS filtering.

## Decisions needed before implementation

1. **AGH placement:** an existing instance is confirmed at `10.77.0.11`. Reuse it.
   Hosting/version and source-preserving LAN routing still need verification. Router-local
   installation is no longer the proposed deployment.
2. **Selectable filtering:** the user wants to choose protection features/services per
   profile, not accept one fixed package. Map real AGH client controls first. Presets may
   be optional shortcuts later; exact defaults and unsupported category requirements
   remain open. Global subscription lists are not independently selectable per client.
3. **Outage behavior:** proposed default for selected clients is no unfiltered fallback.
   Keep the last applied policy if the management API is unavailable; if the DNS resolver
   itself is down, selected clients lose DNS rather than silently switching providers.
   Unselected clients and router-local package resolution should remain independent.
4. **Tickets:** proposed default: an internet ticket changes internet access only, not DNS
   filtering. A temporary DNS exception would be a separate, explicit feature later.

## Important constraints discovered during research

### Identity is the hard part

FamilyWarden identifies devices by MAC. AGH supports client IPs, CIDRs, MACs, and encrypted
DNS ClientIDs, but **MAC-based persistent clients require AGH to be the DHCP server**.
ARP-derived friendly names do not make MAC-based policy matching work with OpenWrt DHCP.

With dnsmasq/odhcpd retained, the first integration must reconcile FamilyWarden MACs to
verified current IP identities, preferably reserved DHCPv4 addresses. Expired/reassigned
leases must lose old ownership before addresses are assigned to another profile.
Do not treat a stale neighbor entry as authoritative proof of ownership.

IPv6 privacy addresses need an explicit solution. Choose and test either trustworthy
IPv6 identity reconciliation or an enforced IPv4-only DNS transport for selected clients
(block selected clients' IPv6 port-53 path while redirecting their IPv4 queries).
IPv4 DNS can still return AAAA records; this does not require disabling IPv6 internet.
A managed VLAN/CIDR is another option if topology warrants it. Do not claim complete
IPv6 DNS coverage while only the IPv4 path is managed.

### Preserve the source address

Clients must reach AGH directly or through source-preserving routing/DNAT.
Putting dnsmasq in front of AGH normally makes every query appear to come from the router,
which defeats per-profile policies.

If AGH is another host on the same LAN, forced DNAT can produce an asymmetric return path.
The common hairpin-SNAT workaround hides the original client IP. Do not ship that topology
with per-client policy claims. Prefer a routed resolver subnet/VLAN, or a tested local AGH
listener. Also test direct same-LAN queries, which may never cross the router's hooks.

### An AGH client is not an arbitrary blocklist group

The current LuCI `dns_profile` field is a free-text stub. It does not correspond to a
built-in AGH "group" API. MVP presets map to supported per-client settings:
filtering enabled, parental protection, SafeSearch, and later blocked services if needed.
AGH's subscribed blocklists are global; arbitrary independent list sets per profile
are not part of the MVP. Client-scoped custom rules are a later extension.

DNS filtering cannot reliably block arbitrary HTTPS paths, YouTube in-stream ads,
all VPNs, or hard-coded DoH over port 443. State these limits in the UI.

## Proposed ownership and data model

- FamilyWarden UCI remains the source of truth for profile membership and `dns_profile`.
- AGH owns its upstreams, subscriptions, query logging, and ordinary manual clients.
- FamilyWarden manages only clearly namespaced clients, e.g. `familywarden:<section-id>`.
  A managed client can contain all verified device IPs in that profile. Use stable section
  IDs rather than display names, which can contain punctuation or change.
- The existing profile `dns_profile` is a legacy stub, not a finished policy schema.
  Design explicit supported per-profile controls for the approved configurable UI;
  presets may be optional shortcuts, not the only choice. Legacy arbitrary values need
  a validation/migration warning; do not silently reinterpret them. Leaving a profile
  unmanaged does not guarantee unfiltered DNS if AGH's existing global settings filter it.
- Store AGH credentials separately in a root-only file, not the LuCI-readable profile UCI,
  status JSON, logs, or pact snapshots. API settings expose readiness, never passwords.
- A bounded reconciliation operation reads desired state, obtains verified identities,
  compares managed AGH clients, and applies only changes using the documented API.
  No daemon per profile and no rewrite of the complete AGH YAML configuration.
- Keep a last-successful reconciliation record and distinguish desired from applied state.
  Serialize reconciliations; reject overlapping client IDs across profiles. Do not
  claim filtering is ready before both client policy and the DNS path are verified.
- Hooks: explicit configuration apply, boot, and relevant lease/identity changes. Add a
  low-frequency repair check only if those hooks cannot recover outages reliably.
  Do not hold the firewall compiler lock during unbounded remote HTTP calls.

## Enforcement path

1. Scope new rules to configured LAN interfaces and selected MACs only.
2. Route both UDP and TCP DNS on port 53 to the approved AGH resolver. Keep control over
   queries addressed to the router and public resolvers, not just DHCP-advertised DNS.
3. Implement and test the selected IPv6 strategy; no unfiltered secondary DNS server.
4. Optionally block unapproved TCP/UDP 853 for selected clients (common DoT/DoQ traffic).
   Explicitly permit the approved Mast DoT endpoint before those drops; do not break our
   own managed resolver path. This is not complete encrypted-DNS/VPN prevention.
5. Keep dnsmasq available for local names/PTR through a domain-specific AGH upstream
   or a tested independent local resolver path. Avoid AGH↔dnsmasq forwarding loops.
6. If selected DNS must remain reachable during internet blocking, add only a narrow
   MAC + resolver-address + TCP/UDP-53 exception before FamilyWarden forward drops.
   Do not restore general internet access. Existing fw4 zone policy must also permit
   the resolver path; an accept in a later chain cannot undo an earlier drop.
7. For router-local AGH, prefer a dedicated listener address on port 53 when feasible.
   Alternate ports require auditing both `familywarden_input` and `familywarden_lockdown`;
   DNAT happens before filtering, so TCP DNS redirected to another port can otherwise
   be blocked by the current port-53-only exceptions. Never exempt the AGH admin UI.
8. Generate rules through `generate_include`, validate in the disposable table, and
   include new chains/sets in cleanup, reload, disable, and uninstall paths.
   No separate script that flushes the firewall or races fw4.

Use an explicit readiness sequence: prepare AGH client settings, validate proposed rules,
then activate interception. If a later step fails, retain the last working state and
report the partial failure; do not enable an unconfigured/bypassable DNS path.

## Ulysses behavior

- Freeze the captain's effective DNS preset and applicable policy settings in the pact
  snapshot, just like schedule/device settings. Use `fw_load`, not live UCI, for decisions.
- Resolve changing addresses from frozen MAC membership; freezing a stale IP forever is
  not correct. Preserve the existing captain-MAC precedence over other profiles.
- Allow-window and ticket transitions do not turn DNS filtering off.
- Define whether the weekly amendment window permits DNS policy changes; recommendation:
  use the same amendment rules as the rest of the captain profile.
- An independently administered AGH is an external escape path: someone with its admin
  credentials can change filtering outside FamilyWarden. UI freezing is not protection
  against AGH administrators, root, alternative networks, VPNs, or arbitrary DoH.
  Do not advertise DNS filtering as an absolute self-binding guarantee.

## Delivery phases and acceptance gates

### A. Disposable topology proof (first)
- Pin an AGH release and use its matching OpenAPI schema, rather than coding against master.
- Test AGH on the chosen host/subnet; measure memory/storage if router-local.
- Two fictional clients, Alex and Taylor, must appear with distinct identities.
- Demonstrate different policies through actual DNS answers, not just API success.
- Verify UDP/TCP, local names, forced public DNS, and the intended IPv6 behavior.
- Gate: choose host placement and identity strategy before production integration.

### B. Minimal adapter and UI
- Implement namespaced client reconciliation and the three presets; list management
  stays in AGH. Start with reserved IPv4 identities and the explicitly enforced IPv6 strategy.
- Use authenticated HTTPS with certificate verification for off-router API access;
  loopback HTTP is acceptable locally. Timeouts, no credentials in command-line logs,
  no broad AGH admin access granted through the ordinary FamilyWarden read ACL.
- Add a connection test and per-profile readiness/errors in LuCI. No unimplemented
  preset may be selectable as though filtering is active.
- Test against a real pinned AGH container, including auth/API failures, restart,
  lease reassignment, profile rename/removal, duplicate IDs, and manual-client preservation.

### C. Firewall and pact integration
- Add compiler-generated interception and optional port-853 restrictions.
- Extend syntax/integration tests for valid includes, disabled state, cleanup,
  idempotency, LAN-only scope, source preservation, IPv6, TCP DNS, and rollback.
- Test ticket/allow behavior, Ulysses snapshot/amendment rules, and continued sail-page access.
- Verify resolver outage versus API outage separately; no silent unfiltered fallback.

### D. Controlled rollout (separate authorization)
- Complete QEMU/container checks, then back up DHCP, firewall, AGH config, and managed clients.
- Do not deploy during the current production pact test. Start with one non-captain
  test device and verify both allowed and blocked test domains using deterministic fixtures.
- Do not modify DHCP ownership, RA, or all-house DNS without explicit approval.
- Provide a documented rollback that removes only FamilyWarden-managed DNS objects and
  restores the previous resolver/DHCP settings without weakening an existing pact.

## Out of scope for the first release

AGH DHCP migration, arbitrary per-profile blocklist subscriptions, query-log dashboards
inside LuCI, provider plugins, TLS interception, and claims of complete DoH/VPN bypass
prevention. Mast Private DNS is now a proposed separate workstream, subject to D1–D10;
it is no longer categorically excluded, and is not yet implemented.

## Sources

- [AGH client identifiers and per-client rules](https://github.com/AdguardTeam/AdGuardHome/wiki/Clients)
- [AGH OpenAPI schema — pin to the tested release before implementation](https://github.com/AdguardTeam/AdGuardHome/blob/master/openapi/openapi.yaml)
- [OpenWrt AGH deployment and resource considerations](https://openwrt.org/docs/guide-user/services/dns/adguard-home)
- [OpenWrt optional AGH resolver alongside dnsmasq](https://openwrt.org/docs/guide-user/services/dns/adguard-home-dummy-interface)
- [dnsmasq domain suffix address/local behavior](https://dnsmasq.org/docs/dnsmasq-man.html)
- [Android DevicePolicyManager Private DNS API](https://developer.android.com/reference/android/app/admin/DevicePolicyManager#setGlobalPrivateDnsModeSpecifiedHost(android.content.ComponentName,java.lang.String))
- [Android private-DNS configuration restriction](https://developer.android.com/reference/android/os/UserManager#DISALLOW_CONFIG_PRIVATE_DNS)
