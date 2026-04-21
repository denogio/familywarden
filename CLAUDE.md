# familywarden — OpenWrt Parental Control Plugin

## Status (2026-04-20)

### Hvad er lavet?
✅ **Pakkestruktur** - familywarden/ og luci-app-familywarden/ mapper oprettet
✅ **UCI config format** - defineret i scope dokumentet
✅ **Shell-daemon** - `familywarden.sh` med alle funktioner (har BUGS)
✅ **Init script** - procd-baseret service
✅ **rpcd plugin** - ubus API integration
✅ **ACL fil** - til HTTP adgang
✅ **LuCI controller** - routing + AJAX endpoints
✅ **LuCI modeller** - profiles, schedules, settings CBI
✅ **LuCI view** - dashboard med auto-refresh
✅ **Danske oversættelser** - i18n fil
✅ **Deploy scripts** - til installation på kørende system

### KENDTE BUGS der skal fixes
🔴 **config_get syntaks** - OpenWrt bruger `config_get <var> <section> <option>` ikke `config_get <section> <option> <var>`
🔴 **Log statements** - parenteser `()` i strings giver syntax error
🔴 **config_list_get** - findes ikke, skal bruge `config_list_foreach` med callback

### Test status
- ✅ Kan køre manuelt: `/usr/bin/familywarden list` 
- ❌ Returnerer tomme profiler (config_get bug)
- ❌ ubus objektet registreres ikke endnu (rpcd skal genstartes)

### Næste skridt
1. Fix config_get syntaks i alle funktioner
2. Fix log statements (escape eller fjern parenteser)
3. Fix device list export
4. Test `ubus call familywarden list`
5. Deploy LuCI UI til router
6. Test webinterface

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

#### Fase 1 — MVP backend (IGANGVÆRENDE)
- [x] UCI-konfigurationsformat defineret
- [x] Shell-baseret daemon med init.d service
- [ ] ~~nftables chain management~~ (afventer daemon fix)
- [ ] Tidsplan-evaluering (cron-baseret)
- [ ] ubus API fuldt implementeret
- [x] rpcd ACL-fil til HTTP-adgang

#### Fase 2 — Brugervenlig (STARTET)
- [x] LuCI oversigtsside (profilstatus, toggle)
- [x] LuCI profil-editor (enheder, policy)
- [x] LuCI tidsplan-editor
- [ ] `ticket` kommando fuldt testet

#### Fase 3 — Home Assistant (IKKE STARTET)
- [ ] HACS custom integration
- [ ] Switch + sensor entiteter per profil
- [ ] Lovelace dashboard-kort

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
*Seneste update: 2026-04-20 18:50*
