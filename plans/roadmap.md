# FamilyWarden — Roadmap for remaining features

*Created 2026-10-14, starting from backend `v0.7.3` / LuCI `0.4.0`.*

The sequence lets each phase build on the previous one and stabilizes the API before
Home Assistant starts depending on it. Versions below are planned targets unless marked complete.

| # | Feature | Size | Dependencies | Version |
|---|---------|------|--------------|---------|
| 1 | ~~Ulysses pact (self-binding lock)~~ ✅ | M | — | backend 0.8.0, LuCI 0.5.0 |
| 2 | Per-profile traffic (nlbwmon) | S | — | backend 0.9.0, LuCI 0.6.0 |
| 3 | Danish translation (i18n) | S | 1+2 (stable strings) | LuCI 0.6.x + `luci-i18n-familywarden-da` |
| 4 | Home Assistant (HACS) | L | 2 (pact information in API) | Separate repository, 0.1.0 |
| 5 | DNS filtering | M–L | — | backend 1.0.0 |

## Working rules (all phases)

- **Definition of done = behavioral evidence.** Every new backend behavior needs a case
  in `scripts/test-integration.sh` (real fw4/nftables in an OpenWrt container) before
  completion. Rule-generation fixes start with a failing test.
- Every ubus method must be advertised in `list`, dispatched in `call`, included in both
  ACL files, and return **one** JSON object. Syntax/integration tests enforce this.
- Makefile version bump → merge → `git tag vX.Y.Z` → CI publishes the release.
- One feature branch and PR per phase; CI must pass before merging.

---

## Phase 2 — Per-profile traffic

**Status:** the frontend already exists (`showTrafficModal` in `overview.js`, calling
`traffic` and expecting `{available, daily, day, week, month}` or
`{available, period, period_start}`). The backend method is missing.

**Design**
- `ubus_traffic <profile>`: if `nlbw` is missing, return `{"available":false}`.
- Otherwise run `nlbw -c json -g mac` and sum rx+tx for the profile's MACs.
- nlbwmon counts per *period* (usually a month), not per day:
  - Default: `{available:true, daily:false, period:<bytes>, period_start:<date>}`
    from the active database.
  - Day/week totals need multiple databases (`nlbw -c json -t YYYY-MM-DD`), and only
    work if nlbwmon is configured for daily intervals (`option commit_interval`/
    `database_interval`). Decide whether this is worth supporting or whether the UI
    should display only "this period".
- `nlbwmon` remains an *optional* dependency, not listed in `DEPENDS`.

**Tasks**
- [ ] `ubus_traffic`, `list` schema, `call` dispatch, and read ACL
- [ ] Integration test: no `nlbw` → `available:false`; fake `nlbw` on PATH returning
      known JSON → correct total for a profile with two MACs
- [ ] Verify actual `nlbw -c json` output on rivendell before writing the parser
- [ ] Playwright: modal displays totals or "unavailable", rather than silently closing

**Decision needed:** implement it (recommended: small, with an existing UI) or remove
its traffic button.

---

## Phase 1 — Ulysses pact ✅ (0.8.0)

**Final design (2026-10-15), merged in PR #1 and tagged v0.8.0 on 2026-10-16.**
Supersedes `plans/archive/PLAN.md` §14. See CLAUDE.md "Ulysses":
- One **captain** (Ulysses profile). Only that profile can make a pact: extend-only
  `until`, or `ongoing` with a weekly amendment window (Sunday 10–11 by default).
  Edits within the window apply immediately.
- The captain's profile is frozen during the pact, and its MACs cannot escape through
  another profile.
- While the captain is blocked, TCP access to the router is restricted for everyone,
  with exceptions for DNS and port 7700.
- Supported early exit: sail page on 7700 (root password + lyric completion), followed
  by a configurable waiting period (default 24 hours, frozen at creation).
- Initial verification covered 89 integration checks (~50 Ulysses-specific, including
  CGI with real password checks), package smoke tests, and browser tests against LuCI
  in a container. LuCI was unreachable during lockdown while 7700 remained accessible.
  The suite has since grown; see CLAUDE.md for current test coverage.

Still pending: deployment/testing on rivendell (procd trigger, real uhttpd instance, reboot).
Unknown devices: handled with `default_policy=block`, frozen during the pact.
Phone hotspot: **Ulysses Mast** (Android Device Owner app, `android/`, `docs/ulysses-mast.md`).
QEMU procd/reboot and Android emulator tests are complete; physical-phone testing is pending.

<details><summary>Original plan (historical)</summary>

Designed in `plans/archive/PLAN.md` §14: freeze the whole profile, allow only root SSH
as an escape hatch, and preserve pacts across reboot. The original plan proposed these
adaptations to the declarative architecture; this is not the implemented API:

- State in `/etc/familywarden.pacts` (persistent, mode 600), `profile=expiry_epoch`.
- **Kernel enforcement, not just UI:** omit allow overrides and ticket elements for a
  bound profile so leaked ubus access cannot unlock it. Rejected block/allow/reset/ticket
  calls return `{"error":"locked by Ulysses pact","locked_until":<epoch>}`.
- Garbage-collect expired pacts in `compile_and_apply`. A blocking pact must also release
  at its end without reload: encode expiry as a kernel-evaluated
  `meta time < "<ISO-time>"` rule, like schedules. Verify in a container first.
- `ubus_pact` can only **extend**, never shorten. There is no `unpact`.

**Tasks**
- [ ] Backend helpers, `gc_expired_pacts`, `ubus_pact`, override rejection,
      and `pact_until` in `list`/`status`
- [ ] Rule generation: no allow/ticket for bound profiles; time-limited drops
- [ ] Write ACL for `pact` in both ACL files
- [ ] "Bind to the mast" UI with time picker, warning, frozen profile, lock banner/countdown
- [ ] Integration: reject overrides and shortening, omit accepts, preserve pact file across
      `/etc/init.d/familywarden restart`, collect expired pacts
- [ ] QEMU reboot test (real procd/boot cannot be tested in the container)

**Original question:** is the June design still desired?

</details>

---

## Phase 3 — Danish translation

The old Lua translations disappeared with the Lua frontend. Modern LuCI uses `.po` files.
Documentation remains English; this phase concerns optional UI localization.

**Tasks**
- [ ] Ensure visible view strings use `_()` (check profiles/overview/info)
- [ ] Generate `luci-app-familywarden/po/templates/familywarden.pot` using LuCI's
      `scripts/i18n-scan.pl`, then add `po/da/familywarden.po`
- [ ] Let `luci.mk` build `luci-i18n-familywarden-da`; include it in CI artifacts,
      releases, and `test-apk.sh` (`.lmo` installs under `/usr/lib/lua/luci/i18n/`)
- [ ] CI: validate `.po` (`msgfmt --check`) and check for missing `.pot` entries

---

## Phase 4 — Home Assistant (HACS)

**Repository:** separate `denogio/hass-familywarden`. HACS requires one integration per
repository, with `custom_components/familywarden/` at its root.

**Transport:** ubus over HTTP (`POST http://<router>/ubus`, JSON-RPC). Authenticate with
`session.login` and a dedicated rpcd login (`/etc/config/rpcd`) with only the
`familywarden` ACL, not root. Renew expired sessions.

**Proposed entities per profile** (reconcile pact fields with the implemented API):
- `switch.<profile>_internet`: on = `allow`, off = `block`; attributes: policy, override,
  override_expires, pact_until
- `sensor.<profile>_status`: allowed/blocked, following live schedules
- `button.<profile>_reset`: return to schedule/policy
- `sensor.<profile>_traffic` when traffic data is available
- Services: `familywarden.ticket` (profile, minutes) and `familywarden.pact`
- During a pact, switches/buttons are unavailable; expose lock information in attributes

**Implementation:** config flow (host, username, password, SSL verification),
`DataUpdateCoordinator` polling `list` every 30 seconds, immediate refresh after
commands, and diagnostics. A Lovelace card is optional; a YAML dashboard example
is sufficient for v0.1.

**Tests/CI:** `pytest-homeassistant-custom-component` with mocked ubus, `hassfest`,
HACS validation in GitHub Actions, tagged releases (HACS reads GitHub releases).
End-to-end against `openwrt/rootfs` with rpcd if supported there; otherwise QEMU.

**Prerequisite here:** README instructions for creating the rpcd user, optionally
with a `uci-defaults` helper.

---

## Phase 5 — DNS filtering

**Proposed direction: AdGuard Home + FamilyWarden-managed DNS enforcement.**
Full design and acceptance gates: [DNS filtering plan](dns-filtering.md).

The earlier proposal redirected profile DNS to a public filtering resolver. That remains
the smaller option, but AGH provides local control of filtering settings, exceptions,
and reporting. FamilyWarden should manage only its namespaced clients and the firewall
path, not replace AGH's blocklist engine or administration UI.

**Decisions needed**
- AGH host placement (existing server versus resource-tested router installation).
- Filtering requirements: ads/trackers, parental protection, SafeSearch, or specific services.
- Client identity with OpenWrt DHCP: AGH MAC identifiers require AGH DHCP, so the initial
  design needs verified IP mapping plus an explicit IPv6 strategy.
- Outage policy and whether tickets should ever relax DNS filtering; proposed defaults:
  no unfiltered fallback for selected clients, and tickets affect internet access only.

**Tasks**
- [ ] Prove topology and per-client identity using a disposable AGH setup
- [ ] Pin a tested AGH release/API and implement minimal namespaced reconciliation
- [ ] Replace the free-text `dns_profile` stub with supported presets and readiness status
- [ ] Generate LAN-scoped UDP/TCP DNS enforcement through the existing compiler pipeline
- [ ] Cover source preservation, IPv6, local names, API/resolver outages, and lease reuse
- [ ] Freeze effective DNS policy during Ulysses pacts; test amendment/ticket behavior
- [ ] Explicitly authorized one-device rollout with backup and rollback, after the current pact test

**Limits:** arbitrary per-profile blocklist sets, AGH DHCP migration, full DoH/VPN
prevention, and off-home Android DNS filtering are outside the first release.

---

## Ongoing work / technical debt

- [ ] Regenerate the knowledge graph with `/understand`; current analysis is from `66c2701`
- [x] Local QEMU test router (`scripts/qemu-dev.sh`): procd trigger, reboot, clock jumps
- [ ] QEMU in CI (GitHub runners have KVM): cover procd/reboot, which containers cannot
- [x] Archive `PLAN.md`, `new-thoughts.md`, and the original kernel-scheduling plan in `plans/archive/`
- [x] Playwright e2e in CI against LuCI in an OpenWrt container (`scripts/test-e2e.sh`)
- [x] `PKG_MAINTAINER`/copyright: `denogio <dennis@hojgaard.me>`
