# DNS preflight — read-only findings

Date: 2026-10-17. Scope: planning steps 1–3 only. No production configuration,
pact, client policy, DNS record, DHCP option, or Android setting was changed.

This is a deliberately sanitized inventory. Existing client identifiers/names,
password hashes, private keys, browsing/query logs, and private rule contents are
not copied here. The query-log database was not read.

## Executive findings

1. Reuse the existing AGH rather than installing another copy on the router.
2. The mobile failure is consistent with the configured access allowlist, but has
   not been conclusively traced from the actual phone/mobile network.
3. Router DNS currently has multiple upstreams, including unfiltered public ones.
   That is not a strict filtering path and must not be described as one.
4. Split DNS is not configured; local queries still return the public address.
5. IPv6 needs explicit treatment: active router DNS listeners include IPv6.
6. Real policy/health, device identity, and removal authorization need more than
   the UI model supplied by the designer. See the design review before adopting it.

## Existing AdGuard Home

| Item | Finding |
|---|---|
| LAN service | `10.77.0.11` |
| Runtime | Docker container `adguardhome`, bridge networking, not router-local |
| Version | `v0.107.79`, schema version 34 |
| Current image | `adguard/adguardhome` (untagged); deployed digest `sha256:aba9e3bf0613be3ba3755e1fc311b126e2c24bec25e18b6483894a88283074f0` |
| DNS | Host TCP/UDP 53 → container 53; IPv4 DNS bind is configured in AGH |
| DoT | Host TCP 853 → container 853; TLS enabled |
| Other published endpoints | Admin HTTP on LAN port 83; setup port 3000; UDP 784 |
| TLS name | `secure-dns.hojgaard.me`; base + wildcard certificate validated in earlier LAN tests |
| Certificate source | Mounted ACME files; renewal automation/reload behavior not audited |
| AGH DHCP | Disabled; do not enable it as part of initial integration |
| Access | Nonempty allowlist: LAN CIDR, one Docker-side address, three non-IP identifiers |
| Illustrative ID | `mast-phone` is not in the current allowlist |
| DNS rate limit | Configured as 0; consider intentional exposure/rate policy before expanding remote access |
| Persistent clients | Two existing clients, one with custom settings; identities not published |
| Filters | Five enabled subscription lists; custom rules and two rewrites also exist |
| Global protection | Filtering, parental protection, and Safe Browsing enabled |
| Global SafeSearch | Enabled for configured providers; YouTube option is false |
| Global blocked services | No service IDs configured |
| Upstreams | `1.1.1.1`, `1.0.0.1`, and DoT to `unfiltered.adguard-dns.com`; no separate fallback list |
| Query logging | Enabled, configured retention interval `7d`; no query data inspected |
| API authentication | Read-only GET `/control/status` on the local mapped endpoint returns 401 |
| API authorization | No authenticated API session was created; service credentials/HTTPS management route still need approval |

The global AGH settings are an observation, **not** approval of those categories for
FamilyWarden, and not proof the user's phone inherits all global settings. Existing
custom-client settings may override them. Subscribed lists are global; do not overwrite
or reassign existing clients without approval.

The plain DNS upstreams are not automatically an unfiltered bypass *around AGH*:
AGH filters before forwarding. This differs from clients/router choosing a public
resolver instead of AGH. Encryption of upstream transport is a separate choice.

### Why Wi-Fi may work while mobile fails

The allowlist accepts local source addresses. Wi-Fi queries via the public address
use NAT reflection, and AGH sees the router's LAN IP, which is allowed.

Off-site traffic retains the mobile public source address. A base hostname or an
unlisted ClientID need not match that allowlist, so queries can be dropped even when
TLS is working. The tested illustrative `mast-phone` name is unlisted. This is a
strong configuration-based hypothesis, not a confirmed diagnosis of the phone's attempt:
its actual configured hostname, source address, carrier path, and TLS/DNS result
have not been observed together.

Do not fix this by clearing allowed clients, allowing every address, or publishing
existing identifiers. ClientIDs can match policy/access rules but are not strong
client authentication. Agree the external-access model before changing it.

## Rivendell DNS and network

| Item | Finding |
|---|---|
| Firmware | OpenWrt 25.12.4; deployed FamilyWarden 0.8.0 / LuCI 0.5.0 |
| LAN DNS | dnsmasq listens on LAN port 53 |
| Explicit DHCP option 6 | Not configured in `dhcp.lan.dhcp_option` |
| Explicit LAN DNS option | `dhcp.lan.dns` unset |
| DHCPv6 / RA | Both in server mode; `ra_dns` unset |
| Expected IPv4 bootstrap | With the observed defaults, router DNS is expected; actual Android DNS/bootstrap still needs checking |
| dnsmasq upstream options | `10.77.0.11`, `1.1.1.1`, `9.9.9.9` |
| dnsmasq upstream source | `noresolv` unset, so automatically learned upstreams may also participate; runtime effective list not fully audited |
| Local names | Domain `lan`, local suffix `/lan/`, rebind protection enabled |
| Resolver split DNS | No matching address/local override found for `secure-dns.hojgaard.me` |
| Reservations | Two configured host reservations; no identities copied |
| IPv6 | DNS listeners include global/ULA/link-local IPv6; do not assume IPv4-only LAN |
| WAN DoT | Active TCP/UDP 853 DNAT to AGH; this does not prove a successful off-site DNS query |
| Reflection | Reflected DoT traffic is SNATed to router LAN address |
| Router packet capture | No `tcpdump` binary found; none installed |

A dnsmasq upstream list is not necessarily a strict primary/backup order. With public
resolvers listed alongside AGH, queries are not guaranteed to be filtered. Do not
change the whole-house DNS configuration without separate approval.

### Read-only DNS checks

Both `secure-dns.hojgaard.me` and `mast-phone.secure-dns.hojgaard.me` were queried
against the router and directly against AGH. All four returned the public CNAME/address,
not `10.77.0.11`. No AGH rewrite targeting this resolver domain was found.

The earlier successful real DoT queries through the public address were LAN-originated.
They validate TLS and DNS service, not independent off-site access or registered
ClientID policy selection.

## Confirmed local-name failure during app implementation

A later read-only check confirmed that `rivendell.lan` returns `10.77.0.1` from
router dnsmasq, but NXDOMAIN from AGH at `10.77.0.11`. Direct HTTP to the numeric
router endpoint on port 7700 succeeds. Strict Private DNS therefore breaks a
hostname-paired app's sync when that resolver cannot resolve local router names.

The management/control path must not depend solely on the filtered DNS path.
Retain an explicitly paired numeric LAN endpoint, still verifying the pinned router
signature and requiring actual LAN/VPN reachability. Do not add an unfiltered system
DNS fallback. Domain-specific AGH forwarding (`[/lan/]10.77.0.1`) is a possible
separate fix for local names, not a change applied by this investigation. Keep other
upstreams intact and verify local-zone forwarding does not create a DNS loop.

## Required follow-up, not performed

- Coordinate an actual cellular test using a deliberately authorized test identity;
  distinguish TCP reachability, TLS validation, allowlist refusal, and filtering.
- Agree external resolver exposure and identifier lifecycle, including revocation and
  brute-force/rate handling. Do not use existing identifiers as public example data.
- Establish an approved, TLS-protected authenticated AGH management route and ownership
  of managed clients. A 401 response is not a failed server; it proves auth is required.
- Inspect actual Android network bootstrap and IPv6 behavior before implementing split DNS.
- Review certificate renewal and AGH reload integration without copying private key material.
- Agree filtering categories/services/exceptions and query-log retention before rollout.
## Integrity checks completed

At the end of the investigation, checksums for the AGH YAML and rivendell's
`dhcp`, `firewall`, `familywarden`, and `uhttpd` configurations matched the initial
fingerprints. No restore/restart/install command was issued to either production host.
Existing private client IDs and configuration secrets were not committed.

## Scope boundary

These findings authorize no changes. Current questions are tracked in
[the main DNS/product plan](dns-filtering.md). Code implementation and deployment
remain separate from the requested planning/read-only/design-review work.
