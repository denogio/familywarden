# familywarden — OpenWrt Parental Control Plugin

## Status (2026-06-04)

### Hvad er lavet?
✅ **Pakkestruktur** - familywarden/ og luci-app-familywarden/ mapper oprettet
✅ **UCI config format** - defineret i scope dokumentet
✅ **Shell-daemon** - `familywarden.sh` med alle funktioner (tidligere bugs fixet)
✅ **Init script** - procd-baseret service
✅ **rpcd plugin** - ubus API integration (shell + ucode variant)
✅ **ACL fil** - til HTTP adgang
✅ **LuCI controller** - routing + AJAX endpoints
✅ **LuCI views** - overview, profiles (m. schedules), settings (JS/client-side)
✅ **Cron-baseret schedule-evaluering** - automatisk evaluering hvert minut
✅ **Makefiles** - rigtige OpenWrt-pakkebuilds for begge pakker
✅ **Danske oversættelser** - i18n fil
✅ **Deploy scripts** - til installation på kørende system + QEMU-test
✅ **UI reload-mønster** - `uci.apply().then(callReload)` efter save på alle views

### Tidligere bugs — NU FIXET
✅ **config_get syntaks** - bruger nu korrekt `config_get <var> <section> <option>`
✅ **config_list_get** - erstattet med `config_list_foreach` + callbacks
✅ **Log statements** - parentes-problem håndteret

### Test status
- ✅ Statisk verifikation: alle scripts parser (`bash -n`), alle påkrævede funktioner + ubus-endpoints findes (`scripts/test-syntax.sh`)
- ⚠️ shellcheck giver kun støj-advarsler (SC1091 OpenWrt-includes, SC3043 `local` i ash, mindre quoting)
- ✅ **QEMU runtime-test bestået** (2026-06-04): `list` + `status` returnerer korrekt JSON
- ✅ **Firewall verificeret**: nft-chain `familywarden_forward` matcher status; scheduled-blokering dropper Josvas MAC med `familywarden:josva` comment
- ✅ **Override verificeret**: `block`/`allow` tilføjer/fjerner nft-regler korrekt på runtime
- ✅ **Ticket + cron verificeret** (2026-06-04): 1-min ticket fjernede drop-reglen, og cron-`eval` ryddede override + genblokerede automatisk ved udløb
- ⏳ Mangler: webinterface i browser (Save & Apply → reload)

### DNS-filtrering — IKKE IMPLEMENTERET (kun UI-stub)
⚠️ Visionen nævner DNS-filtrering, men det er **ikke bygget**:
- Daemonen (`familywarden.sh`) læser aldrig `dns_profile` og gør intet med DNS
- LuCI har et `dns_profile`-felt der gemmer værdien i UCI — men den bruges ingen steder
- i18n-strenge + udkommenteret UCI-eksempel findes, men ingen backend-logik
- Nuværende blokering er ren MAC-baseret nft drop (internet on/off), ikke DNS
- Mulige modeller (ikke valgt endnu): AdGuard Home API, dnsmasq nftset, eller per-klient DNS-redirect i nft

### Næste skridt
1. Test webinterface i browser mod QEMU (http://192.168.1.1/) — Save & Apply → reload
2. Fase 3: Home Assistant HACS-integration
3. Fase 4 (sidst): DNS-filtrering — beslut model og byg backend

---

## Scope Document

### Vision

En native OpenWrt-pakke der giver profilbaseret parental control med tidsplaner, internet-blokering og DNS-filtrering — eksponeret via ubus API så Home Assistant (og andre klienter) kan styre det.

Tænkt som det der mangler i OpenWrt-økosystemet: noget der matcher stock firmware parental controls (som Cudy, ASUS, TP-Link) men med åben API og fuld kontrol.

---

### Pakkestruktur

```
familywarden/
├── familywarden/              # Daemon (shell) — kører på routeren
│   ├── usr/bin/familywarden   # Hovedscript
│   ├── etc/init.d/familywarden
│   ├── usr/lib/rpcd/familywarden
│   └── usr/share/rpcd/acl.d/
├── luci-app-familywarden/     # LuCI frontend — webinterface
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
    option action 'allow'
```

---

## 2. Daemon (`familywarden`)

### Ansvar
- Læser UCI-konfiguration ved opstart og ved `ubus call familywarden reload`
- Evaluerer tidsplaner hvert minut (via procd respawn)
- Tilføjer/fjerner nftables-regler for at blokere/tillade internet per MAC
- Eksponerer ubus-objektet `familywarden`
- Skriver status til `/tmp/familywarden.status` (JSON)

### Firewall-integration (nftables)
```nft
chain familywarden_forward {
    type filter hook forward priority 0; policy accept;
    ether saddr 00:11:22:33:44:55 counter drop comment "familywarden:test"
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
ubus call familywarden reload
```

---

## 4. LuCI Frontend (`luci-app-familywarden`)

### Sider
- **Overview** - Dashboard med profil-status, quick-actions, auto-refresh
- **Profiles** - Profil-editor (enheder, policy)
- **Schedules** - Tidsplan-editor (dage, tidsrum)
- **Settings** - Globale indstillinger

---

## 5. Implementeringsplan

#### Fase 1 — MVP backend (FÆRDIG — runtime-verificeret 2026-06-04)
- [x] UCI-konfigurationsformat defineret
- [x] Shell-baseret daemon med init.d service
- [x] nftables chain management (verificeret på QEMU)
- [x] Tidsplan-evaluering (cron-baseret)
- [x] ubus API fuldt implementeret (verificeret på QEMU)
- [x] rpcd ACL-fil til HTTP-adgang

#### Fase 2 — Brugervenlig (NÆSTEN FÆRDIG)
- [x] LuCI oversigtsside (profilstatus, toggle)
- [x] LuCI profil-editor (enheder, policy)
- [x] LuCI tidsplan-editor
- [x] `ticket` kommando fuldt testet (runtime-verificeret 2026-06-04)
- [ ] Webinterface verificeret i browser (Save & Apply → reload)

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
*Seneste update: 2026-06-04*
