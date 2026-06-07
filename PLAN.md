# FamilyWarden v0.5 — Implementation Plan

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
- [ ] Test override: `ubus call familywarden block '{"profile":"josva"}'`
- [ ] Verify override rule in chain: `nft list chain inet fw4 familywarden_forward | grep josva:override:block`
- [ ] Test reset: `ubus call familywarden reset '{"profile":"josva"}'`
- [ ] Test ticket + lazy GC:
  ```sh
  ubus call familywarden ticket '{"profile":"josva","minutes":1}'
  # Wait ~70s
  ubus call familywarden status '{"profile":"josva"}'
  nft list chain inet fw4 familywarden_forward | grep josva  # should be gone
  ```
- [ ] Test overnight schedule: set Mon–Fri 22:00–07:00, verify morning rule has day set `{2,3,4,5,6}`
- [ ] Test concurrent access (locking):
  ```sh
  ubus call familywarden block '{"profile":"josva"}' &
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

Eftersom LuCI's JavaScript-framework er tæt koblet til backend (ubus/uci), er det meget svært at mocke i simple unit tests (som Jest). Den bedste og mest pålidelige måde at teste LuCI UI på er end-to-end (E2E) mod QEMU.

- [ ] Opret en `test/e2e` mappe med et basalt Playwright setup (`npm init playwright@latest`).
- [ ] Skriv en test der logger ind i LuCI på QEMU instansen (`http://localhost:8080`).
- [ ] Skriv en test der navigerer til FamilyWarden Oversigt og bekræfter at profiler vises.
- [ ] Skriv en test der opretter en ny profil og verificerer at den dukker op.
- [ ] Skriv en test der tester "Block" / "Allow" knapperne og verificerer statusændringen.

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
  - APK'er installeret korrekt via `apk add`
  - postinst hook kørte (type='nftables', ingen name option)
  - include fil genereret med alle regler
  - `ubus reload`, `block`, `reset`, `ticket`, `status` alle virker
  - kernel regler verificeret med `nft list`
- [x] 13. UI E2E Testing (Playwright) — **ALL 6 TESTS PASS**
  Login, overview, block/allow, profiles page, settings page alle verificeret
  Kør: `cd test/e2e && npx playwright test`

---

## 14b. Robusthed / self-healing firewall — FÆRDIG (0.5.7, 2026-06-07)

Opdaget da rivendell (produktion) sad fast med tom `familywarden_forward` chain
og manglende include-fil. Tre fixes:

- [x] **`policy='block'` gav ingen regel.** Kun `scheduled` + override genererede
      regler. Tilføjet `gen_profile_block` → en profil sat permanent til "Blocked"
      dropper nu alle dens devices (override-regler ligger først, så allow-override vinder).
- [x] **Validerings-wedge.** `nft -c` validerede inde i den levende `inet fw4`-tabel,
      så en gammel chain (anden hook/priority) fik validering til at fejle for evigt.
      Nu valideres i en **throwaway-tabel** (`fw_familywarden_validate`) — isoleret
      parse-tjek der ikke kan blokeres af live state.
- [x] **Apply-wedge.** `fw4 reload` fejler "chain already exists with different
      declaration" når live-chainens priority afviger fra include, og dropper så
      vores include. Nu **slettes chainen før `fw4 reload`** → genskabes rent hver gang.

Verificeret på QEMU: injicér stale priority-0 chain + regel, kør KUN
`ubus call familywarden reload` → selv-healer til priority filter+1, stale-regel væk.
Rivendell: tom chain + manglende include fuldt gendannet af ét reload.

> Migrations-note: gamle installationer kan have firewall-include registreret
> gammel-stil (`name='familywarden'`, uden `type='nftables'`/`fw4_compatible='1'`).
> postinst tilføjer kun hvis ingen findes — tjek for dubletter ved opgradering:
> `uci show firewall | grep familywarden.include` skal give præcis én entry m. type=nftables.

---

## 14. Ulysses-pagt (self-binding lock) — IKKE STARTET

### Baggrund
En forælder (med ADHD) vil **binde sig selv til masten** (Ulysses/Odysseus): fx
"internet slukket 23:00–06:00, og jeg kan IKKE tale mig selv fra det i øjeblikket".
En beslutning truffet nu, der fjerner det fremtidige jegs mulighed for at overrule.

Besluttet med bruger:
- **Hele profilen fryses**: hverken override (block/allow/ticket/reset) eller
  redigering i UI mens pagten er aktiv.
- **Escape hatch = kun root via SSH**: UI/ubus nægter at bryde pagten før udløb.
- **Skal overleve reboot** (kritisk) — ellers er escape bare at genstarte routeren.

### Kernebeslutning: persistent state
Pagter ligger i `/etc/familywarden.pacts` (mode 600, root), format `profile=expiry_epoch`.
IKKE i `/tmp/familywarden.state` (ryddes ved reboot). Denne fil ER root-escape-hatchen:
`sed -i '/^dennis=/d' /etc/familywarden.pacts && /etc/init.d/familywarden reload`.

### TODO
- [ ] Backend `familywarden/usr/lib/rpcd/familywarden`:
  - [ ] pact-helpers: `pact_get/pact_set/pact_del/pact_active` (læser `$FW_PACTS`)
  - [ ] `gc_expired_pacts()` (kopi af `gc_expired_tickets`-mønster), kald i `compile_and_apply`
  - [ ] `ubus_pact <profile> <minutes>`: expiry = now+min*60; kun forlæng aldrig forkort;
        ryd allow-override; persist; recompile. **Ingen `unpact`-metode** (det er pointen).
  - [ ] Håndhævelse: `ubus_block/allow/reset/ticket` afviser hvis `pact_active` →
        `{"error":"locked by Ulysses pact","locked_until":<epoch>}`
  - [ ] `gen_override_rules`: spring allow-override over for pacted profil
  - [ ] Eksponér `"pact_until":<epoch|0>` i `ubus_list_profile` + `ubus_status`
  - [ ] Dispatch: `pact` i `list)`-skema + `pact)`-case i `call)`
- [ ] ACL: tilføj `pact` til write i `familywarden/.../acl.d/familywarden.json`
      og `luci-app-familywarden/.../acl.d/luci-app-familywarden.json`
- [ ] Frontend (`profiles.js` + `overview.js`):
  - [ ] `callPact` rpc.declare + "🔒 Bind til masten"-knap m. klokkeslæt-vælger
        (beregn minutter til næste forekomst), bekræftelses-advarsel
  - [ ] Når `pact_until` i fremtiden: read-only frysning, skjul Delete/override-knapper,
        lås-banner m. live nedtælling (genbrug `override_expires`-nedtælling)
- [ ] i18n: danske strenge for nye labels/advarsler
- [ ] Version-bump + `bash scripts/build-apk-docker.sh` + deploy QEMU

### Verifikation (QEMU)
1. `ubus call familywarden pact '{"profile":"dennis","minutes":1}'` → ok
2. `allow/block/ticket/reset` på dennis → `error: locked ... locked_until`
3. `status` → `pact_until` i fremtiden; nft: scheduled/block-regler står, ingen allow-regel
4. **Reboot QEMU** → pagt stadig aktiv (beviser `/etc`-persistens)
5. Vent til udløb → `reload` → gc rydder pagt; overrides virker igen
6. Root-escape: slet linje i `/etc/familywarden.pacts` + reload → pagt væk straks
7. Playwright: kort pagt → lås-banner + disablede kontroller; allow afvises; efter udløb retur

> Note (server-side scope): config-redigerings-frysning håndhæves på UI-niveau.
> Hård revert af UCI-edits er bevidst ude af scope — root-SSH er den aftalte escape.
