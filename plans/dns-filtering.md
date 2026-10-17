# DNS filtering — AdGuard Home integration plan

Status: proposed, not implemented. No changes to rivendell are authorized by this plan.
The current pact test must remain untouched. Documentation and default UI text are English.

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

1. **Where will AGH run?** Prefer an existing always-on server if available, provided the
   routing path preserves client identity. Otherwise evaluate router-local AGH in a lab.
   Rivendell's last observed overlay had about 35 MB free. That leaves limited room
   for AGH, blocklists, and logs. Total/free RAM still needs measuring; the previously
   observed tmpfs size is not a RAM-capacity measurement. Do not install there before
   measuring actual resource use with the intended lists.
2. **What does filtering mean?** Proposed presets: `off`, `standard` (configured ad/tracker
   lists), and `family` (standard + parental/SafeSearch options). Confirm whether adult
   content, specific services, or particular domains are the main requirement.
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
- Use the existing profile `dns_profile`, with an explicit dropdown for supported presets.
  Missing/empty/`off` means no FamilyWarden DNS policy. Legacy arbitrary values need a
  validation/migration warning; do not silently reinterpret them.
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
4. Optionally block TCP and UDP 853 for selected clients (common DoT/DoQ traffic).
   This is mitigation, not complete encrypted-DNS/VPN prevention.
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
inside LuCI, provider plugins, Mast/Android DNS enforcement away from home, TLS interception,
and claims of complete DoH/VPN bypass prevention.

## Sources

- [AGH client identifiers and per-client rules](https://github.com/AdguardTeam/AdGuardHome/wiki/Clients)
- [AGH OpenAPI schema — pin to the tested release before implementation](https://github.com/AdguardTeam/AdGuardHome/blob/master/openapi/openapi.yaml)
- [OpenWrt AGH deployment and resource considerations](https://openwrt.org/docs/guide-user/services/dns/adguard-home)
- [OpenWrt optional AGH resolver alongside dnsmasq](https://openwrt.org/docs/guide-user/services/dns/adguard-home-dummy-interface)
