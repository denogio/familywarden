# familywarden — OpenWrt Parental Control Plugin

## Language and privacy

Use English for replies, all documentation, source comments, and default UI text.
The Android app must also provide complete Danish localization from its first
FamilyWarden-branded release; use Android string/plural resources and system/app locale.
Design mockups are English only; do not duplicate screens for Danish.
Use only fictional children's names (e.g. Alex) in fixtures, examples, UI placeholders,
documentation, and screenshots. Never copy children's real names or device names
from a running router into the repository or public artifacts.

## Documentation sources (Context7)

Use these library IDs directly (skip resolve-library-id):

### Repositories (source code/implementation)
- OpenWrt core / UCI / netifd / nftables: `/openwrt/openwrt`
- ubus IPC / JSON-RPC / C API: `/openwrt/ubus`
- LuCI feed / package source: `/openwrt/luci`

### Website (generated JS API reference — exact signatures)
- LuCI client-side JS API (rpc/uci/form/ui/network): `/websites/openwrt_github_io_luci`

### Choosing a source for FamilyWarden
- LuCI frontend (rpc.declare, CBI classes, widgets) → `/websites/openwrt_github_io_luci`
- ubus backend in C (ubus_add_object, blobmsg policy) → `/openwrt/ubus`
- UCI/nftables/system integration → `/openwrt/openwrt`

Always specify a topic (e.g. "rpc.declare signature", "TypedSection", or
"ubus method registration"), rather than just a repository name.

## Status (2026-10-16)

Versions: `familywarden` **0.8.0**, `luci-app-familywarden` **0.5.0**.
Ulysses PR #1 is merged into `master`; `v0.8.0` is tagged. Release publication depends
on successful CI. Previous published release: v0.7.3.
Repository: https://github.com/denogio/familywarden — CI builds releases (see "CI / release").

### Architecture (since 0.5.x)
- **No polling daemon or cron for schedule evaluation.** Backend logic lives in the rpcd plugin
  `familywarden/usr/lib/rpcd/familywarden` (installed as `/usr/libexec/rpcd/familywarden`).
- **Declarative pipeline** (`compile_and_apply`): UCI + RAM state (`/tmp/familywarden.state`)
  → generate `/var/etc/familywarden.include` → `nft -c` validation in a disposable table
  → atomic `mv` → `nft delete` of old chains/set → `fw4 reload`.
- **The kernel evaluates schedules and tickets:** schedules become `meta hour`/`meta day` rules;
  tickets are elements in the `familywarden_ticket` timeout set. Schedule boundaries need no recompile.
- **Init script** (procd) runs `$BIN reload` at boot and uses `procd_add_reload_trigger`
  for UCI changes. A running instance keeps the reload trigger active; Ulysses also uses a periodic tick.
- **rpcd responses must be one JSON object.** `compile_and_apply` prints `{"status":"ok"}`
  or an error itself; handlers must not print an additional response (fixed in 0.7.2).
- **apk scripts:** postinst must not check `"$1" = configure` (an opkg convention that apk
  never supplies). Use `[ -n "$IPKG_INSTROOT" ] && exit 0` to skip image builds.

### Implemented
- ✅ **rpcd plugin:** ubus API: list, status, block, allow, reset, ticket, reload, version.
- ✅ **nft forward chain** (`familywarden_forward`, forward hook priority 1): overrides,
  policy=block, scheduled access (allow windows + default drop).
- ✅ **nft input chain** (`familywarden_input`, 0.7.x): per-profile `block_router='1'`
  drops TCP to the router (LuCI/SSH) while blocked; DHCP/DNS still work. Uses the same
  allow-window + trailing-drop model as forwarding, sharing `_sched_accept_rules`.
  Fixed in 0.7.3; multiple windows on one day previously locked router access incorrectly.
- ✅ **Self-healing firewall:** stale chains are deleted before fw4 reload; invalid includes
  are rejected and the last valid ruleset is retained.
- ✅ **Ulysses (0.8.0):** see below.
- ✅ **Package scripts:** postinst adds the firewall include once (broken under apk before
  0.7.2); prerm removes it.
- ✅ **LuCI JS views:** overview (cards, per-day highlighting), profiles (inline schedules
  using TableSection), settings, info (versions).
- ✅ **Makefiles:** OpenWrt apk builds; `scripts/build-apk-docker.sh` uses the SDK in Docker.
- ✅ **`scripts/deploy.sh`:** copies backend/LuCI sources directly to a router, using Makefile paths.
- ✅ **CI/CD:** GitHub Actions tests → build → package smoke tests; tagged releases.

### Ulysses Mast (Android, `android/`)

Device Owner app for the captain's phone. Restricts hotspot and tethering using
`DISALLOW_CONFIG_TETHERING` while the captain is bound and blocked. Android's
`UserRestrictionActionListener` handles existing tethering; physical-phone behavior
has not yet been verified. Guide: `docs/ulysses-mast.md`.

- **Router:** `cgi-bin/mast` on port 7700. First response line is the usign signature;
  the remainder is exact payload JSON (`mast_payload`: pact and effective captain rules).
  Keys: `/etc/familywarden/mast.{key,pub}`. `?pubkey` supports manual pairing.
  `ubus mast {host}` provides the public key, fingerprint, last sync, pairing URI,
  and QR SVG (`qrencode` dependency). Captain status includes `mast_last_sync`.
- **App** (Kotlin, `me.hojgaard.ulysses.mast`, minSdk 30): `Policy` is pure logic matching
  router semantics. It uses the cache until the next successful sync, with no age-based
  expiry. `Usign` uses BouncyCastle Ed25519. `Sync` rejects signature failures and
  responses older than the last `server_time`.
- **Enforcement:** `Mast.enforce` restricts tethering, plus `DISALLOW_DEBUGGING_FEATURES`
  in release builds while blocked. Automatic time/timezone are enforced while paired.
  Exact alarms handle evaluation boundaries; WorkManager syncs every 15 minutes.
- **UI:** Jetpack Compose + Material 3, light/dark themes, large-text support, previews.
  `mapMastUiState` maps verified state; `Policy.nextBlockChange` supplies actual hotspot
  transitions, unlike `nextChange`, which supplies evaluation boundaries.
- **Pairing/removal:** Google Code Scanner handles `ulyssesmast://pair?host=…&key=…`
  (also accepted as a deep link). Fingerprint confirmation pins the key. Pairing is
  guarded while bound, not merely hidden. Paired removal requires a fresh signed no-pact
  response both when opening and when confirming the removal dialog.
- **Debug build** (`src/debug`): `DebugReceiver` supports pair/sync via `am broadcast`
  and retains adb. Neither concession exists in release builds.
- **Build:** Docker (`ghcr.io/cirruslabs/android-sdk:35`; local Java 27 is too new for AGP)
  or CI. Gradle wrapper and distribution are verified against official checksums.
  R8/resource shrinking reduces release APK size to about 4.1 MB.
- **Signing:** `~/.config/familywarden/mast-release.jks` and GitHub secrets
  `MAST_KEYSTORE_B64`/`MAST_KEYSTORE_PASSWORD`. Updates MUST use the same signing key.
- **Verified on Android 15 emulator:** sideloaded APK enrolled as Device Owner using adb;
  restrictions follow pacts, release, and new pacts; persist after force-stop attempts
  and offline reboot; `pm clear` (SecurityException) and uninstall
  (`DELETE_FAILED_DEVICE_POLICY_MANAGER`) are refused. `dpm set-device-owner` may fail
  immediately after boot until `hasIncompatibleAccountsTask` finishes; the script retries.
- **Limits:** a new pact must reach the phone before it can be enforced there. Router
  last-sync information records a fetch, not confirmed enforcement. Root/recovery/reset
  remain outside the protection boundary. No physical Pixel/Android 16 runtime test yet.

### Global settings
- `settings.enabled='0'`: same chains/set, but no rules; status reports `allowed`.
- `settings.default_policy='block'`: the final `familywarden_forward` rule drops LAN
  ingress from MACs not assigned to any profile, including disabled profiles.
  `lan_devices` uses `fw4 -q zone lan`, falling back to UCI. WAN responses are unaffected.
  Overview lists DHCP leases not assigned to profiles.
- Both settings are frozen during a Ulysses pact: `enabled` is treated as 1, and
  `default_policy` is saved in the pact.

### Ulysses (the captain's pact)

Designed with Dennis on 2026-10-14/15. One profile is the **captain**
(`settings.ulysses_profile`); only that profile can make a **pact**
(`ubus call familywarden pact`). Persistent state in `/etc/familywarden/`
(`pact`, `pact.snap`, `release`) survives reboot.

- **Pact types:** `until` (ends at an epoch, extend-only) or `ongoing` (no end,
  weekly amendment window, Sunday 10–11 by default). Ongoing pacts can only be amended
  within that window.
- **Frozen profile:** enforcement uses `pact.snap` (enabled, policy, block_router,
  devices, schedules). UCI edits are ignored except inside the amendment window,
  where they apply immediately and refresh the snapshot. Captain and waiting period
  are frozen in the `pact` file.
- **No escape through another profile:** captain MACs are stripped from other profiles
  in the effective configuration. Allow and ticket are refused for the captain;
  block and reset remain available.
- **Lockdown** (`familywarden_lockdown`, input priority 2): a separate base chain prevents
  its accept rules from bypassing `familywarden_input`. While the captain is blocked,
  TCP access to the router is dropped for all devices. Exceptions: loopback,
  established traffic, DNS (53), sail page (7700), and the amendment window.
  The kernel evaluates captain schedules (`_sched_accept_rules` with `_sched_devices="any"`).
  For `until`, the drop rule includes `meta time < <epoch>`.
- **Supported early exit:** sail page on **port 7700**, served by a separate uhttpd
  instance (`uhttpd.familywarden`, created by postinst), with
  `www-familywarden/index.html` and `cgi-bin/sail`. Requires the root password
  (checked using `ubus call session login`, three-second failure delay and flock)
  plus the line "By the light of moon I will ..." (answer `press on`, from Josh
  Garrels' "Ulysses"). Release follows the waiting period (`settings.ulysses_delay`
  hours, default 24, frozen at creation). A release request can be canceled.
  CLI `sail-*` commands must NEVER become ubus methods (`test-syntax` checks this).
  `block_router` drops also exempt port 7700.
- **Effective configuration:** `ulysses_sync` (inside `compile_and_apply`) writes
  `/var/run/familywarden/familywarden`. All enforcement/reporting must use `fw_load`,
  never direct `config_load`. LuCI overview uses effective schedules from `list`, not UCI.
- **Tick:** the procd instance runs `familywarden tick` every minute, recompiling only
  when a release is due or an `until` pact expires.
- **LuCI:** "⚓ Make a pact"/"Extend pact"/"Amend pact" appear only for the captain,
  through `ulysses.js`. A frozen profile has no footer or Delete action. Ulysses settings
  are read-only during the pact. Activation uses ubus rather than a UCI field, because
  Save & Apply rollback would undo activation if lockdown disconnected the browser.
- **Unassigned devices:** use `default_policy=block` to deny internet to devices outside
  all profiles. That setting is also frozen during the pact.
- **If the sail page fails** during an ongoing pact, OpenWrt failsafe (reset button at
  boot) remains a physical-access recovery option.

### Known gaps
- ⚠️ **Per-profile traffic:** `overview.js` calls `traffic` (nlbwmon), but the rpcd plugin
  has no such method. The modal silently closes on error.
- ⚠️ **No i18n:** old Lua controller/`familywarden.da.lua` files were removed; no `.po` files exist.
- ⚠️ **Outdated knowledge graph:** analyzed at `66c2701`, before block_router and
  kernel scheduling. Run `/understand` again.

### Test status
- ✅ `scripts/test-syntax.sh`: rpcd/init parsing, required functions/endpoints
  (advertised + dispatched), chain/set consistency, Makefile↔init↔rpcd paths,
  LuCI JS parsing, valid JSON.
- ✅ `scripts/test-integration.sh`: real rpcd plugin against real fw4/nftables in an
  `openwrt/rootfs` container. Covers daytime/overnight schedules, block, allow,
  disabled profiles, block_router, block/allow/reset/ticket via `call`, ticket GC,
  idempotent reload, invalid include rejection, and Ulysses.
  **Add a case here whenever changing rule generation.**
- ✅ `scripts/test-apk.sh`: installs built packages with feed dependencies in a container;
  checks files, postinst/prerm, and config preservation across reinstall.
- ✅ **Mast:** 36 unit tests (`android/`), lint, debug/release builds, and
  `scripts/test-mast-emulator.sh` (Android 15, KVM, SDK in Docker volume `mast-sdk`).
  UI checks cover pairing confirmation, light/dark, 200% text, stale removal dialogs,
  and successful Device Owner removal after release.
- ⚠️ shellcheck reports platform-related warnings (SC1091 OpenWrt includes, SC3043 `local` in ash).
- ✅ **Playwright e2e** (`test/e2e/`, 17 tests): `scripts/test-e2e.sh [apk-dir]` runs
  against LuCI in an OpenWrt container (`scripts/e2e-container-setup.sh`, with a stub
  for procd's `system` object), also in CI. For QEMU:
  `FW_BASE_URL=http://localhost:8080 FW_PASSWORD=ulysses npx playwright test`.
  `ulysses.spec.js` creates real pacts and requires `FW_E2E_ALLOW_PACT=1`, set only
  for disposable containers by test-e2e.sh. Save & Apply is a dropdown (`li`);
  await "Configuration changes applied". `E()` flattens only one array level.
- ✅ **QEMU (real OpenWrt 25.12/procd):** `scripts/qemu-dev.sh`, user-mode networking,
  no root required; `start --fresh`, `deploy`, `time "21:29"`, `console`, `run`, `reboot`.
  Verified 2026-10-15: procd trigger (~2 s), automatic 21:30 lockdown (LuCI/SSH closed,
  7700 open), release via 7700 + tick, and persistence across reboot. The console
  represents physical access during lockdown. OpenWrt quirk: first `reload_config`
  after installation does not fire for configs absent at boot; LuCI explicitly reloads.
- Images live in git-ignored `images/` (base image, `qemu-dev.qcow2`, `run/`).
  Run `npm install` in `test/e2e/` and `npx playwright install chromium` before e2e tests.
- Do not upgrade the live router or modify an active QEMU pact without explicit permission.

### DNS filtering — NOT IMPLEMENTED (UI stub only)

The vision includes DNS filtering, but there is no implementation:
- The rpcd plugin never reads `dns_profile` or changes DNS.
- LuCI saves `dns_profile` to UCI, but nothing consumes it.
- A commented UCI example exists, but no backend logic.
- Current blocking uses MAC-based nft drops (internet on/off), not DNS.
- Proposed direction: AdGuard Home client-policy integration plus nft DNS-path enforcement.
  See `plans/dns-filtering.md`, especially its planning checkpoint and D1–D16 decisions.
  Existing AGH is at `10.77.0.11`; base/wildcard DNS is `secure-dns.hojgaard.me`.
  LAN-hairpin DoT works, but cellular access is unresolved. Split DNS and Mast Private
  DNS/ClientID enforcement are proposals, not applied configuration. D1/D2 are confirmed:
  always-on strict DNS, no automatic unfiltered fallback. Initial scope: user's phone;
  Android product becomes FamilyWarden with a separate Ulysses section and a planned logo.
  Approved new package ID: `io.github.denogio.familywarden` (not implemented).
  Lighthouse logo reference: `docs/design/familywarden-logo.html`; Ulysses retains its
  mast/sail subsection mark. Planning only. Package change needs Device Owner migration, not an APK
  update in place. Personal DNS names are deployment settings, not project defaults;
  changing the actual DNS domain is still an open question.
  The user authorized preparation steps 1–3 only (scope, read-only inventory, design review).
  Read `plans/dns-preflight-report.md` and `docs/design/familywarden-app/REVIEW.md` before
  continuing. Inventory and design review are complete; 19 opt-in contract-review tests
  plus 36 existing tests pass. The provided model is a reference, not runtime code.
  Actual filter values, API administration, removal authorization, and cellular access
  still have user/verification gates. Do not infer permission for implementation/deployment.
  Unresolved questions remain deferred until their relevant work; do not decide them silently.
  Future removal needs authorization as well as no active applicable pact; the exact
  captain/admin/managed-device matrix is still open. Do not infer non-admin means child;
  never confuse ClientID with authentication or LAN tests with external reachability.
  Do not change the live router during its current pact test.

### CI / release
- `.github/workflows/ci.yml`: push/PR → syntax + integration tests → SDK build
  (`openwrt/sdk:x86-64-25.12.4`) → package smoke tests → artifacts → Playwright
  against LuCI in a container using CI-built packages. Android tests/lint/builds
  run separately. Release requires successful build, e2e, and Android jobs.
- Bump `PKG_VERSION`, commit, merge, then `git tag vX.Y.Z && git push --tags`.
  The tag must match `familywarden/Makefile`, or release fails. LuCI is versioned separately.
- `dist/` is git-ignored for local builds. Packages are published as GitHub releases
  (since v0.7.3). Keep the Android signing key stable for future updates.
- Commit author/committer: `denogio <11715805+denogio@users.noreply.github.com>`.
  No assistant co-author trailers. Maintainer/copyright: `denogio <dennis@hojgaard.me>`.

### Next steps

See **`plans/roadmap.md`**: Ulysses (0.8.0), traffic, i18n, Home Assistant, DNS filtering.

---

## Scope document

### Vision

A native OpenWrt package offering profile-based parental control with schedules,
internet blocking, and eventually DNS filtering, exposed through ubus for Home
Assistant and other clients. It aims to match stock-firmware parental controls
(Cudy, ASUS, TP-Link) while providing an open API and full control.

### Package structure

```
familywarden/
├── familywarden/                 # Shell backend running on the router
│   ├── usr/lib/rpcd/familywarden  # ubus API + nft include compiler
│   ├── etc/init.d/familywarden   # procd: reload at boot and on UCI changes
│   ├── etc/config/familywarden
│   └── usr/share/rpcd/acl.d/
├── luci-app-familywarden/        # LuCI JS views in htdocs/
├── android/                     # Ulysses Mast Device Owner companion
├── test/e2e/                    # Playwright against LuCI
├── dist/                        # Local builds (git-ignored)
└── hass-openwrt-familywarden     # Future Home Assistant HACS integration
```

## 1. UCI configuration (`/etc/config/familywarden`)

```uci
config global 'settings'
    option enabled '1'
    option default_policy 'allow'

config profile 'test'
    option name 'Test Profile'
    option enabled '1'
    option policy 'scheduled'
    list device 'aa:bb:cc:dd:ee:ff'
    option dns_profile 'child'

config schedule 'test_weekday'
    option profile 'test'
    option days 'mon tue wed thu fri'
    option start '07:00'
    option stop '20:00'
```

**Schedule model:** schedules are pure *allow windows*. With `policy 'scheduled'`,
internet is blocked by default, and allowed only when the current day/time falls
within at least one window. There is no `action` option (removed 2026-06-04 because
block actions combined with default-deny had confusing semantics).

## 2. Backend (`familywarden`)

There is no schedule-polling daemon. The rpcd plugin generates a fw4 include on
changes, and the kernel enforces schedules and tickets.

### Responsibilities
- Read UCI + RAM state (`/tmp/familywarden.state`: overrides and ticket expiry).
- Compile `/var/etc/familywarden.include` and run `fw4 reload`.
- Expose the `familywarden` ubus object.

### Firewall integration (nftables, fw4 include)

```nft
set familywarden_ticket { type ether_addr; flags timeout; }

chain familywarden_input {        # Only profiles with block_router='1'
    type filter hook input priority 1; policy accept;
    ether saddr @familywarden_ticket tcp dport 0-65535 counter accept
    # ...profile-specific router-access rules...
}

chain familywarden_forward {
    type filter hook forward priority 1; policy accept;
    ether saddr @familywarden_ticket counter accept comment "familywarden:ticket"
    # override → policy=block → scheduled: accept inside windows, otherwise drop
    ether saddr aa:bb:cc:dd:ee:ff meta hour >= 07:00 meta hour < 20:00 meta day { 1, 2, 3, 4, 5 } counter accept comment "familywarden:test:sched-allow"
    ether saddr aa:bb:cc:dd:ee:ff counter drop comment "familywarden:test:scheduled"
}
```

## 3. ubus API

```sh
ubus call familywarden list              # List all profiles
ubus call familywarden status '{"profile":"test"}'
ubus call familywarden block '{"profile":"test"}'
ubus call familywarden allow '{"profile":"test"}'
ubus call familywarden reset '{"profile":"test"}'
ubus call familywarden ticket '{"profile":"test","minutes":30}'
ubus call familywarden pact '{"until":0,"window_day":"sun","window_start":"10:00","window_stop":"11:00"}'  # Captain's pact: epoch or 0=ongoing
ubus call familywarden reload
ubus call familywarden version           # Installed backend/frontend versions
```

## 4. LuCI frontend (`luci-app-familywarden`)

### Pages
- **Overview:** profile status, quick actions, automatic refresh.
- **Profiles:** devices, policy, block_router, and inline schedules (days/time windows).
- **Settings:** global settings.
- **Info:** installed versions through `ubus version`.

## 5. Implementation plan

### Phase 1 — MVP backend (DONE — runtime verified 2026-06-04)
- [x] UCI configuration format
- [x] rpcd plugin + procd init.d service (replaced the original shell daemon in 0.5.x)
- [x] nftables chain management through fw4 include (QEMU verified)
- [x] Kernel schedule evaluation (`meta hour`/`meta day`, replacing cron)
- [x] Full ubus API (QEMU verified)
- [x] rpcd ACL for HTTP access

### Phase 2 — Usability (DONE, except traffic backend)
- [x] LuCI overview, profile editor, and schedule editor
- [x] Fully tested `ticket` command (runtime verified 2026-06-04)
- [x] Browser verification through Playwright
- [x] block_router: restrict router access for blocked profiles (0.7.x)
- [ ] Per-profile traffic — frontend only, backend missing

### Phase 3 — Home Assistant (NOT STARTED)
- [ ] HACS custom integration
- [ ] Per-profile switch and sensor entities
- [ ] Lovelace dashboard card

### Phase 4 — DNS filtering (NOT STARTED, final phase)
- [ ] Select model: AdGuard Home API / dnsmasq nftset / per-client nft DNS redirect
- [ ] Read `dns_profile` and apply per-profile/MAC filtering in the backend
- [ ] Connect the existing LuCI field to real backend logic
- [ ] Runtime DNS filtering tests

## Knowledge graph

The interactive project knowledge base is `.understand-anything/knowledge-graph.json`.

⚠️ Analyzed at commit `66c2701` (see `meta.json`), before block_router and kernel
scheduling. It also describes the deleted ucode variant and `familywarden.sh`.
Run `/understand` again before relying on those details.

**ALWAYS read it before coding.** It contains:
- 44 nodes (files, functions, configs), with purpose and complexity
- Five architecture layers: daemon, frontend, build, test, documentation
- 25 edges (contains, related, documents) describing dependencies
- Ten guided tour steps explaining the architecture

### How to use it
1. **Before editing:** read the graph to find existing functions, architecture layers,
   and callers. Avoid duplication.
2. **When adding functions:** follow current naming patterns. The historical graph
   mentions `handle_<method>`; current shell ubus handlers use `ubus_<method>`.
   LuCI uses `build*`, `show*`, `refresh*`, and `injectStyles` in view files.
3. **Before writing code:** ask whether it belongs in backend or frontend, whether it
   should use ubus or direct UCI, and which nftables rules must change.

Example before implementing DNS filtering:
- Understand how nftables rules are added.
- Locate backend firewall functions.
- Trace how LuCI calls the backend through ubus.

## Technical notes

### OpenWrt config_get syntax

```sh
# WRONG:
config_get name "${section}" name "${default}"

# CORRECT:
config_get result_var "${section}" option "${default}"
# After the call, $result_var contains the value.

# Or print the value directly:
config_get "${section}" option  # Prints to stdout
```

### config_list_foreach pattern

```sh
# WRONG (config_list_get does not exist):
devices=$(config_list_get "${profile}" device)

# CORRECT:
devices=""
_list_callback() {
    devices="$devices$1 "
}
config_list_foreach "${profile}" device _list_callback
```

---

*Project: FamilyWarden*

*Started: 2026-04-20*

*Last updated: 2026-10-16*
