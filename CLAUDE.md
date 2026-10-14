# familywarden — OpenWrt Parental Control Plugin

## Documentation sources (context7)

Brug disse library-IDs direkte (spring resolve-library-id over):

# Repos (kildekode/implementering)
- OpenWrt core / UCI / netifd / nftables:  /openwrt/openwrt
- ubus IPC / JSON-RPC / C API:             /openwrt/ubus
- LuCI feed / pakke-kildekode:             /openwrt/luci

# Website (genereret JS API-reference — præcise signaturer)
- LuCI client-side JS API (rpc/uci/form/ui/network):  /websites/openwrt_github_io_luci

## Valg af kilde for familywarden
- LuCI frontend (rpc.declare, CBI-klasser, widgets) → /websites/openwrt_github_io_luci
- ubus backend i C (ubus_add_object, blobmsg-policy) → /openwrt/ubus
- UCI/nftables/system-integration                    → /openwrt/openwrt

Angiv altid et topic ved opslag (fx "rpc.declare signatur", "TypedSection",
"ubus method registration") frem for bare repo-navnet.

## Status (2026-10-14)

Versioner: `familywarden` **0.8.0**, `luci-app-familywarden` **0.5.0** (Ulysses, branch `feature/ulysses`). Seneste release: v0.7.3.
Repo: https://github.com/denogio/familywarden — releases bygges af CI (se "CI / release").

### Arkitektur (siden 0.5.x)
- **Ingen daemon og ingen cron.** Al backend-logik ligger i rpcd-pluginet
  `familywarden/usr/lib/rpcd/familywarden` (installeres som `/usr/libexec/rpcd/familywarden`).
- **Deklarativ pipeline** (`compile_and_apply`): UCI + RAM-state (`/tmp/familywarden.state`)
  → generér `/var/etc/familywarden.include` → `nft -c` validering i throwaway-tabel
  → atomisk `mv` → `nft delete` af gamle chains/set → `fw4 reload`.
- **Kernen evaluerer tidsplaner og tickets**: schedules bliver til `meta hour`/`meta day`-regler,
  og tickets er elementer i timeout-settet `familywarden_ticket`. Ingen recompile ved tidsgrænser.
- **Init script** (procd) kører `$BIN reload` ved boot og via `procd_add_reload_trigger`
  ved UCI-ændringer. En idle `sleep`-instans holder servicen i live, så triggeren virker.
- **rpcd-svar skal være ét JSON-objekt.** `compile_and_apply` printer selv `{"status":"ok"}`
  eller en fejl; handlers må ikke echo'e noget ekstra oveni (fixet i 0.7.2).
- **apk-scripts**: postinst må ikke tjekke `"$1" = configure` (opkg-konvention, apk sender det
  aldrig). Brug `[ -n "$IPKG_INSTROOT" ] && exit 0` for at springe image-builds over.

### Hvad er lavet?
✅ **rpcd plugin** - ubus API: list, status, block, allow, reset, ticket, reload, version
✅ **nft forward-chain** (`familywarden_forward`, hook forward prio 1) - override, policy=block, scheduled (accept-vindue + default drop)
✅ **nft input-chain** (`familywarden_input`, 0.7.x) - `block_router='1'` per profil dropper TCP til routeren (LuCI/SSH) mens profilen er blokeret; DHCP/DNS virker stadig. Samme model som forward (accept per vindue + trailing drop, delt `_sched_accept_rules`) — fixet i 0.7.3, før låste flere vinduer samme dag routeren
✅ **Selvhelende firewall** - stale chains slettes før fw4 reload; ugyldigt include afvises og sidste gode ruleset bevares
✅ **Ulysses (0.8.0)** - se afsnittet "Ulysses" nedenfor
✅ **Pakke-scripts** - postinst tilføjer firewall-include én gang (virkede ikke under apk før 0.7.2), prerm fjerner det
✅ **LuCI views (JS)** - overview (kort, per-dag highlight), profiles (inline schedules via TableSection), settings, info (versioner)
✅ **Makefiles** - OpenWrt apk-builds; `scripts/build-apk-docker.sh` bygger med OpenWrt SDK i Docker
✅ **`scripts/deploy.sh`** - kopiér kildefiler (backend + LuCI) direkte til en router, samme stier som Makefiles
✅ **CI/CD** - GitHub Actions: test → build → apk-smoke-test, release ved tag

### Ulysses (selvbindende lås)
To mekanismer, begge persistente i `/etc/familywarden/` (overlever reboot):
- **Stående lås** (`option lock '1'` per profil): profilen håndhæves fra et snapshot
  (`locked/<profil>.snap`: enabled, policy, block_router, lock, devices, schedules).
  Ændringer i UCI noteres som ventende (`.pending`) og overtages først **24 t efter
  seneste ændring** (`FW_LOCK_DELAY`, konstant med vilje). At låse gælder straks; at
  låse op er en ændring som alle andre. Navnet er kosmetisk og tages altid fra UCI.
- **Ad hoc-pagt** (`ubus call familywarden pact '{"profile":"x","minutes":N}'`, maks 7 dage):
  `pacts`-linje `profil udløb mac...` (MAC'er fanget ved oprettelse). Drop-regel med
  `meta time < <epoch>` øverst i begge chains, så kernen selv slipper den. Kan kun forlænges.
- Låst/pagt: `allow` og `ticket` afvises (`{"error":"locked by Ulysses pact",...}`), og der
  emittes aldrig allow-/ticket-regler. `block` og `reset` virker (strammer kun).
- **Effektiv config**: `lock_sync` (i `compile_and_apply`) skriver `/var/run/familywarden/familywarden`
  = UCI med låste profiler erstattet af snapshot. Alt der håndhæver/rapporterer bruger `fw_load`,
  aldrig `config_load` direkte. LuCI-oversigten viser tidsplaner fra `list` (effektive), ikke UCI.
- **Tick**: init-scriptets procd-instans kører `familywarden tick` hvert minut; recompiler kun når
  en ventende ændring er moden.
- **Nødudgang (kun root/SSH)**: `familywarden unlock <profil>` (symlink i `/usr/sbin`) fjerner
  snapshot, pending og pagt og sætter `lock=0`. Må aldrig eksponeres via ubus (test-syntax tjekker).
- Kendte omveje med root-adgang til LuCI: stop/deaktivér tjenesten under System → Startup,
  afinstallér pakken, eller slet firewall-includet. Låsen er friktion, ikke en sikkerhedsgrænse.

### Kendte huller
- ⚠️ **Global "Enable FamilyWarden"/"Default Policy"** i Settings læses ikke af backenden (død knap).
- ⚠️ **Traffic per profil**: `overview.js` kalder ubus-metoden `traffic` (nlbwmon), men rpcd-pluginet har ingen `traffic`-metode. Modalen lukker bare stille ved fejl.
- ⚠️ **Ingen i18n**: de gamle Lua-filer (controller + `familywarden.da.lua`) er fjernet, og der findes ingen `.po`-filer.
- ⚠️ **Knowledge graph er forældet** (analyseret ved `66c2701`, før block_router og kernel-scheduling). Kør `/understand` igen.

### Test status
- ✅ `scripts/test-syntax.sh`: rpcd/init parser, påkrævede funktioner, endpoints (advertised + dispatched), chain/set-konsistens, Makefile↔init↔rpcd-stier, LuCI-views parser, JSON gyldig.
- ✅ `scripts/test-integration.sh`: kører det rigtige rpcd-plugin mod rigtig fw4 + nftables i
  `openwrt/rootfs`-container. Dækker scheduled (dag + overnat), block, allow, disabled, block_router,
  block/allow/reset/ticket via `call`, ticket-GC, idempotent reload, afvisning af ugyldigt include.
  **Tilføj en case her, når du ændrer regelgenereringen.**
- ✅ `scripts/test-apk.sh`: installerer byggede apk'er i container; tjekker filer, postinst/prerm, at config overlever reinstall.
- ⚠️ shellcheck giver kun støj-advarsler (SC1091 OpenWrt-includes, SC3043 `local` i ash)
- ✅ **Playwright e2e** (`test/e2e/`, mod QEMU på 192.168.1.1): sider loader, block/allow, opret/slet profil, tilføj enhed, opret schedule, policy bevares ved save. Sidste kørsel: bestået.
- QEMU-image ligger i `images/` (git-ignoreret). `npm install` i `test/e2e/` før e2e-kørsel.

### DNS-filtrering — IKKE IMPLEMENTERET (kun UI-stub)
⚠️ Visionen nævner DNS-filtrering, men det er **ikke bygget**:
- rpcd-pluginet læser aldrig `dns_profile` og gør intet med DNS
- LuCI har et `dns_profile`-felt der gemmer værdien i UCI — men den bruges ingen steder
- Udkommenteret UCI-eksempel findes, men ingen backend-logik
- Nuværende blokering er ren MAC-baseret nft drop (internet on/off), ikke DNS
- Mulige modeller (ikke valgt endnu): AdGuard Home API, dnsmasq nftset, eller per-klient DNS-redirect i nft

### CI / release
- `.github/workflows/ci.yml`: push/PR → `test-syntax` + `test-integration` → SDK-build
  (`openwrt/sdk:x86-64-25.12.4`) → `test-apk` → apk'er som artifact.
- Release: bump `PKG_VERSION`, commit, `git tag vX.Y.Z && git push --tags`. Tagget skal matche
  `familywarden/Makefile`, ellers fejler release-jobbet. luci-appen versioneres separat.
- `dist/` indeholder historiske apk'er; nye builds hører hjemme i GitHub releases.

### Næste skridt
Se **`plans/roadmap.md`**: ~~Ulysses~~ (0.8.0), traffic, i18n, Home Assistant, DNS-filtrering.

---

## Scope Document

### Vision

En native OpenWrt-pakke der giver profilbaseret parental control med tidsplaner, internet-blokering og DNS-filtrering — eksponeret via ubus API så Home Assistant (og andre klienter) kan styre det.

Tænkt som det der mangler i OpenWrt-økosystemet: noget der matcher stock firmware parental controls (som Cudy, ASUS, TP-Link) men med åben API og fuld kontrol.

---

### Pakkestruktur

```
familywarden/
├── familywarden/              # Backend (shell) — kører på routeren
│   ├── usr/lib/rpcd/familywarden   # Al logik: ubus API + nft include-compiler
│   ├── etc/init.d/familywarden     # procd: reload ved boot + UCI-ændring
│   ├── etc/config/familywarden
│   └── usr/share/rpcd/acl.d/
├── luci-app-familywarden/     # LuCI frontend (JS views i htdocs/)
├── test/e2e/                  # Playwright mod QEMU
├── dist/                      # Byggede apk'er
└── hass-openwrt-familywarden # Home Assistant HACS integration (senere)
```

---

## 1. UCI Konfiguration (`/etc/config/familywarden`)

```uci
config global 'settings'
    option enabled '1'
    option default_policy 'allow'

config profile 'test'
    option name 'Test Profil'
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

**Schedule-model:** Schedules er rene *allow-vinduer*. I `policy 'scheduled'` er internet
slukket som standard ("block by default"); en profil er kun tilladt når nuværende
tid+dag falder inden for et af dens schedules. Der er ingen `action`-option længere
(droppet 2026-06-04 — block-action gav forvirrende semantik sammen med default-deny).

---

## 2. Daemon (`familywarden`)

Der er ingen kørende daemon længere — se "Arkitektur" under Status. rpcd-pluginet
genererer et fw4-include ved hver ændring, og kernen håndhæver tidsplaner og tickets.

### Ansvar (rpcd-plugin)
- Læser UCI + RAM-state (`/tmp/familywarden.state`: overrides og ticket-udløb)
- Kompilerer `/var/etc/familywarden.include` og kører `fw4 reload`
- Eksponerer ubus-objektet `familywarden`

### Firewall-integration (nftables, fw4-include)
```nft
set familywarden_ticket { type ether_addr; flags timeout; }

chain familywarden_input {        # kun profiler med block_router='1'
    type filter hook input priority 1; policy accept;
    ether saddr @familywarden_ticket tcp dport 0-65535 counter accept
    ...drop-regler med inverterede meta hour/day-grænser...
}

chain familywarden_forward {
    type filter hook forward priority 1; policy accept;
    ether saddr @familywarden_ticket counter accept comment "familywarden:ticket"
    # override → policy=block → scheduled: accept i vinduet, ellers drop
    ether saddr aa:bb:cc:dd:ee:ff meta hour >= 07:00 meta hour < 20:00 meta day { 1, 2, 3, 4, 5 } counter accept comment "familywarden:test:sched-allow"
    ether saddr aa:bb:cc:dd:ee:ff counter drop comment "familywarden:test:scheduled"
}
```

---

## 3. ubus API

```
ubus call familywarden list              # List alle profiler
ubus call familywarden status '{"profile":"test"}'
ubus call familywarden block '{"profile":"test"}'
ubus call familywarden allow '{"profile":"test"}'
ubus call familywarden reset '{"profile":"test"}'
ubus call familywarden ticket '{"profile":"test","minutes":30}'
ubus call familywarden pact '{"profile":"test","minutes":420}'   # Ulysses, kan kun forlænges
ubus call familywarden reload
ubus call familywarden version           # installerede backend/frontend-versioner
```

---

## 4. LuCI Frontend (`luci-app-familywarden`)

### Sider
- **Overview** - Dashboard med profil-status, quick-actions, auto-refresh
- **Profiles** - Profil-editor (enheder, policy, block_router) med inline tidsplaner (dage, tidsrum)
- **Settings** - Globale indstillinger
- **Info** - Installerede versioner (via `ubus version`)

---

## 5. Implementeringsplan

#### Fase 1 — MVP backend (FÆRDIG — runtime-verificeret 2026-06-04)
- [x] UCI-konfigurationsformat defineret
- [x] rpcd-plugin + procd init.d service (oprindeligt shell-daemon, erstattet i 0.5.x)
- [x] nftables chain management via fw4-include (verificeret på QEMU)
- [x] Tidsplan-evaluering i kernen (`meta hour`/`meta day`, erstattede cron)
- [x] ubus API fuldt implementeret (verificeret på QEMU)
- [x] rpcd ACL-fil til HTTP-adgang

#### Fase 2 — Brugervenlig (FÆRDIG)
- [x] LuCI oversigtsside (profilstatus, toggle)
- [x] LuCI profil-editor (enheder, policy)
- [x] LuCI tidsplan-editor
- [x] `ticket` kommando fuldt testet (runtime-verificeret 2026-06-04)
- [x] Webinterface verificeret i browser (Playwright e2e mod QEMU)
- [x] block_router: blokér adgang til routeren for blokerede profiler (0.7.x)
- [ ] Traffic per profil — kun frontend findes, backend mangler

#### Fase 3 — Home Assistant (IKKE STARTET)
- [ ] HACS custom integration
- [ ] Switch + sensor entiteter per profil
- [ ] Lovelace dashboard-kort

#### Fase 4 — DNS-filtrering (IKKE STARTET, sidste fase)
- [ ] Vælg model: AdGuard Home API / dnsmasq nftset / per-klient nft DNS-redirect
- [ ] Daemon læser `dns_profile` og anvender filtrering per profil/MAC
- [ ] Kobl eksisterende LuCI `dns_profile`-felt til reel backend-logik
- [ ] Test DNS-filtrering på runtime

---

## Knowledge Graph

Der findes en interaktiv videnbase over projektet i `.understand-anything/knowledge-graph.json`.

⚠️ Den er analyseret ved commit `66c2701` (se `meta.json`) og kender ikke block_router
eller kernel-scheduling. Den beskriver også ucode-varianten og `familywarden.sh`, som er slettet.
Kør `/understand` igen, før du stoler på den.

**Læs den ALTID før du koder!** Den indeholder:
- 44 noder (filer, funktioner, configs) med formål og complexity
- 5 arkitektur-lag: daemon, frontend, build, test, documentation
- 25 edges (contains, related, documents) der viser afhængigheder
- 10 guided tour steps der forklarer arkitekturen

**Sådan bruger du den:**

1. **Før du ændrer noget** — læs `.understand-anything/knowledge-graph.json` for at forstå:
   - Hvilke funktioner der allerede findes (undgå duplikering)
   - Hvilke lag filen tilhører
   - Hvilke andre filer der bruger den

2. **Når du tilføjer funktioner** — tjek at navngivning følger eksisterende mønster
   - Daemon: `handle_<method>` i `familywarden/usr/lib/rpcd/familywarden`
   - LuCI: `build*`, `show*`, `refresh*`, `injectStyles` i view-filer

3. **Før du kode nyt** — spørg dig selv:
   - Tilhører det daemon-laget eller frontend-laget?
   - Skal det via ubus API eller direkte UCI?
   - Hvilke nftables-regler skal ændres?

** Eksempel på brug:**
```
Før jeg tilføjer DNS-filtrering, vil jeg forstå:
- Hvordan nftables-reglerne pt. tilføjes (se knowledge-graph noder)
- Hvilke funktioner i daemon der håndterer firewall
- Hvordan LuCI frontend kalder daemon via ubus
```

## Tekniske noter

### OpenWrt config_get syntaks
```sh
# FORKERT:
config_get name "${section}" name "${default}"

# KORREKT:
config_get result_var "${section}" option "${default}"
# Efter kaldet: $result_var indeholder værdien

# ELLER for bare at echo værdien:
config_get "${section}" option  # printer til stdout
```

### config_list_foreach pattern
```sh
# FORKERT (config_list_get findes ikke):
devices=$(config_list_get "${profile}" device)

# KORREKT:
devices=""
_list_callback() {
    devices="$devices$1 "
}
config_list_foreach "${profile}" device _list_callback
```

---

*Projekt: FamilyWarden*
*Startet: 2026-04-20*
*Seneste update: 2026-10-14*
