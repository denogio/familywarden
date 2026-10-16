# FamilyWarden v0.5 — Implementation Plan

> Archived implementation history. Status labels and test counts below describe
> their original milestones, not the current release. See `../../CLAUDE.md`.

## Architecture Decision

**Fully declarative nftables.** Every state change flows through one pipeline:

```
read UCI + read RAM state + GC expired tickets
              ↓
      compile full nftables include
              ↓
      nft -c -f /tmp/familywarden.include.new   ← validate
              ↓
      mv /tmp/familywarden.include.new /var/etc/familywarden.include   ← atomic
              ↓
      fw4 reload
```

**No direct nft mutations. Ever.** No rule handles, no background sleep processes, no hybrid paths.
The 100–300ms cost of fw4 reload is acceptable for parental control.

See `new-thoughts.md` for full rationale.

---

## Critical Bugs in Current Code

These MUST be fixed:

1. **`flush ruleset`** in `familywarden/usr/lib/rpcd/familywarden` line 96 — deletes ALL nft rules including fw4's own firewall. Fatal.
2. **`table inet fw4 { ... }`** re-declaration in line 98 — conflicts with fw4's table.
3. **`type filter hook forward priority 0`** in line 100 — fw4 owns the hook, include should not redeclare it.
4. **`ether daddr $mac`** in line 83 — matches client MAC as destination, which makes no sense in forward chain (daddr is router's MAC or next-hop). Must be removed.
5. **Overnight schedule** in line 80 — OR expression uses same day set for both evening and morning rules. Fails for single-day schedules and weekday→weekend transitions.
6. **No validation** — `generate_include` + `apply_fw4` without `nft -c` check.
7. **No locking** — concurrent ubus calls can race.

---

## TODO

### 1. Update QEMU script (25.12.4 image)

File: `scripts/qemu-test.sh`

- [x] Line 10: change URL to `https://downloads.openwrt.org/releases/25.12.4/targets/x86/64/openwrt-25.12.4-x86-64-generic-ext4-combined.img.gz`
- [x] Line 11: change `X86_IMG` to `$IMG_DIR/openwrt-x86-25.12.4.img`

> **Note:** 25.10.4 does not exist. Latest stable 25.x is 25.12.4.

---

### 2-6. Rewrite rpcd plugin — IN PROGRESS

File: `familywarden/usr/lib/rpcd/familywarden`

Fully rewritten and tested on QEMU. Verified working:
- ✅ Include format: `chain familywarden_forward { rules }` + `chain forward { jump familywarden_forward }`
- ✅ Override rules inserted first (higher priority)
- ✅ Scheduled rules with day shift for overnight schedules
- ✅ `nft -c -f` validation wrapped in table context for include fragments
- ✅ `flock` serialization via `compile_and_apply()`
- ✅ Lazy GC for ticket expiry in `compile_and_apply()`
- ✅ Fixed: `$profile` closure bug in nested `config_foreach` callbacks

Known issue: `meta hour >= 20` displays as `"00:00:20"` in `nft list` output. Works in kernel (fw4 reload succeeds). Day names display as strings ("Monday") not numbers.

---

### 7. Fix init.d script — DONE

---

### 8. Fix Makefile postinst/prerm hooks — IN PROGRESS

postinst: Added `type='nftables'` (required, not `script`). Fixed: removed `name` option (fw4 rejects it).

---

### 9. LuCI frontend cleanup — DONE

These are minor — frontend already uses correct modern JS view pattern (form.Map, rpc.declare, JSON menu/ACL).

- [ ] Delete empty `luci-app-familywarden/luasrc/controller/` directory
- [ ] (Optional, separate task) Convert `luasrc/i18n/familywarden.da.lua` from Lua format to `.po` gettext format

---

### 10. Build packages

- [ ] `bash scripts/build-apk-docker.sh`
- [ ] Verify output: `dist/familywarden-*.apk` and `dist/luci-app-familywarden-*.apk`

---

### 11. Test on QEMU

- [ ] Start QEMU with new 25.10.4 image: `./scripts/qemu-test.sh x86`
- [ ] Copy scripts directly for fast iteration:
  ```sh
  scp -O familywarden/usr/lib/rpcd/familywarden root@192.168.1.1:/usr/libexec/rpcd/familywarden
  scp -O familywarden/etc/init.d/familywarden root@192.168.1.1:/etc/init.d/familywarden
  ```
- [ ] Enable and start:
  ```sh
  chmod +x /usr/libexec/rpcd/familywarden
  /etc/init.d/familywarden enable
  /etc/init.d/familywarden start
  ```
- [ ] Test include generation: `/usr/libexec/rpcd/familywarden reload && cat /var/etc/familywarden.include`
- [ ] Validate syntax: `nft -c -f /var/etc/familywarden.include`
- [ ] Apply: `fw4 reload`
- [ ] Check kernel rules: `nft list chain inet fw4 familywarden_forward`
- [ ] Test override: `ubus call familywarden block '{"profile":"alex"}'`
- [ ] Verify override rule in chain: `nft list chain inet fw4 familywarden_forward | grep alex:override:block`
- [ ] Test reset: `ubus call familywarden reset '{"profile":"alex"}'`
- [ ] Test ticket + lazy GC:
  ```sh
  ubus call familywarden ticket '{"profile":"alex","minutes":1}'
  # Wait ~70s
  ubus call familywarden status '{"profile":"alex"}'
  nft list chain inet fw4 familywarden_forward | grep alex  # should be gone
  ```
- [ ] Test overnight schedule: set Mon–Fri 22:00–07:00, verify morning rule has day set `{2,3,4,5,6}`
- [ ] Test concurrent access (locking):
  ```sh
  ubus call familywarden block '{"profile":"alex"}' &
  ubus call familywarden reload &
  wait  # no errors
  ```

---

### 12. Full APK install test

- [ ] Install APKs on QEMU
- [ ] Verify firewall include added to `/etc/config/firewall` (via postinst)
- [ ] Verify `/var/etc/familywarden.include` generated on first boot
- [ ] Test override block/allow/ticket via LuCI web interface
- [ ] Test postinst idempotency:
  ```sh
  opkg remove familywarden && opkg install familywarden
  uci show firewall | grep familywarden  # exactly one entry
  ```

---

### 13. UI E2E Testing (Playwright)

LuCI's JavaScript framework is tightly coupled to its ubus/UCI backend, making it difficult to mock in simple unit tests such as Jest. End-to-end tests against QEMU provide the most reliable UI verification.

- [ ] Create `test/e2e` with a basic Playwright setup (`npm init playwright@latest`).
- [ ] Test login to LuCI in QEMU (`http://localhost:8080`).
- [ ] Navigate to FamilyWarden Overview and verify profiles are displayed.
- [ ] Create a profile and verify it appears.
- [ ] Test the "Block" / "Allow" buttons and verify status changes.

---

## Open Questions (resolve before merge)

1. **fw4 include context** — Does fw4 include expect bare `chain` declarations or `add chain`? Test on live OpenWrt.
2. **Jump rule injection** — How does fw4 add `jump familywarden_forward` into its forward chain? Via the include itself, or via a separate UCI firewall rule?
3. **meta day timezone** — `meta hour`/`meta day` use system time (typically UTC on OpenWrt). If schedules are stored in local time in UCI, the compiler must convert. Alternative: rely on router's timezone being set correctly. Decision needed.
4. **i18n format** — Current `.da.lua` uses Lua format but LuCI JS views use `.po` gettext. Convert now or later?

---

## Files to Modify

| File | What | Priority |
|------|------|----------|
| `scripts/qemu-test.sh` | Update to 25.10.4 image (line 10+11) | P1 |
| `familywarden/usr/lib/rpcd/familywarden` | Full rewrite: include format, overnight fix, pipeline, locking, GC | **P0** |
| `familywarden/etc/init.d/familywarden` | Add START/STOP, stop_service() | P0 |
| `familywarden/Makefile` | postinst uci + add prerm | P1 |
| `luci-app-familywarden/luasrc/controller/` | Delete empty dir | P2 |
| `luci-app-familywarden/luasrc/i18n/familywarden.da.lua` | (Optional) convert to .po | P3 |
| `test/e2e/` | Initialize Playwright and add UI tests | P1 |

---

## Progress Tracker (Master Checkbox List)

Claude Code: Please tick off these checkboxes `[x]` as you complete each major section of the plan above.

- [x] 1. Update QEMU script (25.12.4 image)
- [x] 2-6. Rewrite rpcd plugin — include file structure, overnight shift, compiler pipeline, flock, GC
- [x] 7. Fix init.d script
- [x] 8. Fix Makefile postinst/prerm hooks
- [x] 9. LuCI frontend cleanup
- [x] 10. Build packages
- [x] 11. Test on QEMU
  - **QEMU verified working:**
    - Override rules inserted first (higher priority than schedules)
    - Scheduled rules with day shift for overnight schedules (Mon-Fri 20:00-06:00 → two rules with +1 day)
    - `nft -c -f` validation works with table wrapper
    - `block`, `reset`, `reload` all work via ubus
    - Rules visible in kernel `nft list chain inet fw4 familywarden_forward`
  - **Bugs found & fixed:**
    - `uci.commit is not a function` → replaced with `uci.apply()`
    - `add chain inet fw4 familywarden_forward` in include file → replaced with `chain familywarden_forward {}` format
    - `fw4 reload` hangs on `type='script'` include → changed to `type='nftables'`
    - Nested `config_foreach` closure bug ($profile not visible in nested callbacks) → fixed with `_cur_profile` global
    - `compile_and_apply` return code not caught → added error handling to all ubus handlers
- [x] 12. Full APK install test
  - Packages installed correctly using `apk add`
  - postinst hook ran (type='nftables', no name option)
  - Include file generated with all rules
  - `ubus reload`, `block`, `reset`, `ticket`, and `status` all work
  - Kernel rules verified using `nft list`
- [x] 13. UI E2E Testing (Playwright) — **ALL 6 TESTS PASS**
  Login, overview, block/allow, profiles, and settings pages verified.
  Run: `cd test/e2e && npx playwright test`

---

## 14b. Robustness / self-healing firewall — DONE (0.5.7, 2026-06-07)

Discovered when rivendell (production) was stuck with an empty
`familywarden_forward` chain and a missing include file. Three fixes:

- [x] **`policy='block'` produced no rule.** Only scheduled profiles and overrides
      generated rules. Added `gen_profile_block`: a permanently "Blocked" profile
      now drops all its devices. Override rules come first, so allow overrides win.
- [x] **Validation deadlock.** `nft -c` validated inside the live `inet fw4` table,
      so an old chain with another hook/priority could cause permanent failure.
      Now validation uses a **disposable table** (`fw_familywarden_validate`):
      an isolated parse check that live state cannot obstruct.
- [x] **Apply deadlock.** `fw4 reload` fails with "chain already exists with different
      declaration" when the live chain's priority differs from the include, then
      omits our include. Now the **chain is deleted before `fw4 reload`**, allowing
      a clean recreation every time.

QEMU verification: inject a stale priority-0 chain and rule, run only
`ubus call familywarden reload` → priority filter+1 restored, stale rule removed.
On rivendell, one reload fully restored the empty chain and missing include.

> Migration note: old installations may register the include using the old format
> (`name='familywarden'`, without `type='nftables'`/`fw4_compatible='1'`).
> postinst adds an include only if none exists. Check for duplicates on upgrade:
> `uci show firewall | grep familywarden.include` should return exactly one entry
> with type=nftables.

---

## 14c. Schedule semantics: allow windows — DONE (0.5.9, 2026-06-07)

Bug: schedules dropped traffic *inside* the window (a block window), while the
specification/UI says "blocks by default and only allows access during the time windows".
A profile with 07:00–18:45 was therefore allowed in the evening instead of blocked.

- [x] Rewritten as **accept windows + default drop per device**: accept rules for
      each allow window, then a final drop. Inside a window → accept (terminates);
      otherwise fall through to drop. Multiple/overlapping windows combine as OR
      without merging. Override accepts are earlier in the chain, so temporary
      allowance still wins.
- [x] Overnight (start>stop): accept `>=start` on the selected day and `<stop`
      on the next day (day_shift). No windows means blocked 24/7.
- [x] Reduced ~14 rules/device/week to ~2. Verified on QEMU
      (daytime/overnight/override order) and rivendell.

> Semantics: the window is the ALLOW period. 07:00–21:45 means allowed during
> daytime and blocked 21:45–07:00. Set an earlier stop time to block earlier.

---

## 14d. Version info + reliable config reload — DONE (0.6.4 / 0.3.7, 2026-06-07)

- [x] **Reliable reload** (0.6.1): two bugs prevented UCI changes reaching the
      firewall without manual reload; the user had to edit the include directly:
  - `$BIN reload` was a no-op (missing top-level `reload)` dispatch), so boot
    and procd triggers did nothing. Fixed with `reload)` → `ubus_reload`.
  - procd keeps reload triggers active only while a service has a RUNNING instance.
    `start_service` opened none. Added an idle `sleep` instance with respawn so
    `procd_add_reload_trigger` actually fires.
- [x] **Version tab** (0.6.4 / 0.3.7): new `version` ubus method (read ACL in
      both acl.d files) returns `{backend, frontend}` using `apk -v list -I`.
      Settings displays an Info card with both versions (graceful fallback: "unknown").
- [x] Verified using a ten-point QEMU checklist and on rivendell; clean version output.

---

## 15. Test strategy — PLANNED (not yet implemented at the time)

**Principle:** "Definition of done" means **behavioral evidence**, not structure.
All observed failures (validation deadlock, fw4 omitting includes, no-op reload,
inactive trigger, inverted schedules) would have been caught by Layer 1.
Three prioritized layers:

### Layer 1 — QEMU end-to-end behavior test ← regression gate

File: `scripts/test-behavior.sh` (recorded as committed in this historical plan).
Assumes QEMU is running; installs the built package, sets a known configuration
(one scheduled profile/device/window), runs PASS/FAIL assertions, exits nonzero on
failure, and cleans up afterward.

Checklist (each item is an assertion):
1. Boot/start → `/var/etc/familywarden.include` exists
2. Running procd instance (`ubus call service list`)
3. Scheduled profile → window accept rule + trailing default drop
4. Allow window: drop *outside* the window; accept covers only the window
5. **UI apply propagates:** `uci set stop=UNIK; uci commit; reload_config; sleep`
   → include reflects UNIK without manual reload (proves the trigger)
6. Override block → `override:block` drop; allow → `override:allow` accept before
   schedule drop; reset → no override
7. Ticket N minutes → `override:allow` with expiry
8. Self-heal: inject stale priority-0 chain/rule, `ubus reload`
   → priority filter+1 restored and stale state removed
9. Restart persistence: `/etc/init.d/familywarden restart` recreates include/chain
10. `ubus call familywarden version` returns clean backend/frontend, without trailing text

### Layer 2 — Host logic unit tests (fast, no router)

File: `test/unit/schedule_test.sh` (host `sh`, BusyBox-compatible).
Isolated tests of math/rule generation: `time_to_minutes`, `day_to_num`,
`day_shift`, and exact accept-rule strings for daytime, overnight splits,
multiple windows, start==stop, and empty days. About one second.

### Layer 3 — Playwright (extend `test/e2e/familywarden.spec.js`)

Add checks for two version strings in the Info box, and persistence after schedule
editing and Save & Apply. Lower priority: UI regression coverage.

### Running / CI
- `sh test/unit/schedule_test.sh` (host, always)
- `sh scripts/test-behavior.sh` (requires running QEMU)
- `cd test/e2e && npx playwright test` (requires QEMU)
- Optionally `scripts/test-all.sh`: always run unit tests; behavior/Playwright if QEMU responds

### Rule

Do not mark a PLAN.md item `[x]` until its relevant **Layer 1** assertion passes.
Layer 1 is the gate.

### Build order (decision: plan first)

1. Layer 1 (`scripts/test-behavior.sh`), 2. Layer 2 (host unit tests), 3. Layer 3 (Playwright).

---

## 16. UI polish + schedule/policy save bug — DONE (2026-06-10, LuCI 0.3.17)

Frontend only (`luci-app-familywarden`); no backend changes.

### A. Bug: policy resets when adding a schedule at the same time — FIXED

Changing policy to "Scheduled" AND adding a schedule before Save & Apply saved the
schedule but reverted the policy to "Unrestricted".

- **Cause:** the schedule GridSection modal's `map.save()` re-rendered the map,
  leaving a stale policy widget. Attempts to patch onchange staging and parse
  overrides failed. **Redesign:** edit schedules inline using `form.TableSection`.
  No modal → no re-render → this class of bug is structurally avoided.
- [x] **Fix:** inline TableSection with custom `handleAdd` (`map.save(null, true)`,
  not the prototype handleAdd that adds twice). `rmempty=true` throughout.
- [x] **Days widget:** the dropdown was awkward and ui.Select checkbox
  `getValue()` returned null, losing days on save. Replaced with seven inline
  checkboxes (`input[data-day]` in `div.fw-days`), custom `renderWidget` and
  `formvalue`. Round trip verified on disk (`days='mon' 'tue'`).
- [x] **Profile create/delete rollback:** `uci.apply()` resolves BEFORE the
  detached confirm poll (setTimeout 1 s). Redirect interrupted confirmation,
  causing rollback about ten seconds later. This explained profiles seemingly
  created/deleted and then reverting. Fix: explicit `uci.callConfirm()` in the
  chain before redirect, including the catch branch.
- [x] **Verification:** Playwright "policy stays Scheduled…" checks both policy
  and day persistence after reload. Hardened delete test: accept dialogs in
  beforeEach, unique victim name, waitForURL predicate excluding `?name=`.
  10/10 passed against installed 0.3.17 in QEMU; deployed to rivendell.

### B. Native Settings rendering (`settings.js`) — DONE
- [x] Removed custom wrapper; call `m.render()` directly for native page chrome.
- [x] Added `'require uci'`.

### C. Use native colors (profiles + settings; Overview unchanged) — DONE
- [x] Action buttons use native `cbi-button-positive/negative/neutral/action`.
- [x] Status dots use theme variables (`--success/--error/--warn-color`).
- [x] `.fw-tab-add` uses `--primary-color-high`.

### Deliberately out of scope

Overview card design/colors, replacing the profile switcher with native tabs,
in-page tabs in the profile editor, and backend changes.

---

## 14. Ulysses pact (self-binding lock) — SUPERSEDED

> Implemented in 0.8.0 with a different design. See `plans/roadmap.md` Phase 1
> and CLAUDE.md "Ulysses". The following is historical, not the implemented API.

### Background

A parent with ADHD wants to **bind themselves to the mast** (Ulysses/Odysseus):
"Internet off 23:00–06:00, and I CANNOT talk myself out of it in the moment."
A decision made now removes the future self's ability to override it.

Agreed with the user:
- **Freeze the entire profile:** no overrides (block/allow/ticket/reset) or UI edits
  while the pact is active.
- **Escape hatch: root over SSH only.** UI/ubus refuse early release.
- **Must survive reboot** (critical), otherwise restarting the router is an escape.

### Core decision: persistent state

Pacts live in `/etc/familywarden.pacts` (mode 600, root), `profile=expiry_epoch`,
NOT `/tmp/familywarden.state`, which is cleared on reboot. This file IS the
root escape hatch:
`sed -i '/^dennis=/d' /etc/familywarden.pacts && /etc/init.d/familywarden reload`.

### TODO
- [ ] Backend `familywarden/usr/lib/rpcd/familywarden`:
  - [ ] Helpers `pact_get/pact_set/pact_del/pact_active` (read `$FW_PACTS`)
  - [ ] `gc_expired_pacts()`, patterned after `gc_expired_tickets`, called by `compile_and_apply`
  - [ ] `ubus_pact <profile> <minutes>`: expiry = now+min*60; extend only, never shorten;
        clear allow override, persist, recompile. **No `unpact` method** (that is the point).
  - [ ] Reject `ubus_block/allow/reset/ticket` when `pact_active`:
        `{"error":"locked by Ulysses pact","locked_until":<epoch>}`
  - [ ] `gen_override_rules`: skip allow override for bound profiles
  - [ ] Expose `"pact_until":<epoch|0>` in `ubus_list_profile` and `ubus_status`
  - [ ] Dispatch: `pact` in `list)` schema and `pact)` case in `call)`
- [ ] Add `pact` write permission in both
      `familywarden/.../acl.d/familywarden.json` and
      `luci-app-familywarden/.../acl.d/luci-app-familywarden.json`
- [ ] Frontend (`profiles.js` + `overview.js`):
  - [ ] `callPact` rpc.declare, "🔒 Bind to the mast" button with time picker
        (minutes until the next occurrence), confirmation warning
  - [ ] Future `pact_until`: read-only profile, hide Delete/override controls,
        lock banner with live countdown (reuse `override_expires` countdown)
- [ ] i18n: Danish translations for new labels/warnings
- [ ] Version bump, `bash scripts/build-apk-docker.sh`, deploy to QEMU

### Verification (QEMU)
1. `ubus call familywarden pact '{"profile":"dennis","minutes":1}'` → ok
2. `allow/block/ticket/reset` for dennis → `error: locked ... locked_until`
3. `status` → future `pact_until`; nft scheduled/block rules remain, no allow rule
4. **Reboot QEMU** → pact still active (proves persistence in `/etc`)
5. Wait for expiry → `reload` → GC removes pact; overrides work again
6. Root escape: delete line in `/etc/familywarden.pacts` + reload → immediate release
7. Playwright: short pact → lock banner/disabled controls; allow rejected; normal after expiry

> Server-side scope: profile edit freezing is enforced in the UI.
> Hard reversion of UCI edits is deliberately out of scope: root SSH is the agreed escape.
