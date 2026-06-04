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
- ⏳ Fuld QEMU-runtime test mangler (kør `sudo ./qemu.sh` — kræver root/KVM/tap)
- ⏳ `ubus call familywarden list` ikke verificeret på rigtig/QEMU runtime endnu

### Næste skridt
1. Boot QEMU og verificér `ubus call familywarden list` returnerer profiler
2. Test schedule-evaluering (cron) faktisk skifter nft-regler
3. Test `ticket`-kommandoen end-to-end
4. Test webinterface i browser mod QEMU
5. Start Fase 3 (Home Assistant HACS-integration)

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

#### Fase 1 — MVP backend (NÆSTEN FÆRDIG — mangler runtime-test)
- [x] UCI-konfigurationsformat defineret
- [x] Shell-baseret daemon med init.d service
- [x] nftables chain management (implementeret, runtime-test mangler)
- [x] Tidsplan-evaluering (cron-baseret)
- [x] ubus API fuldt implementeret (runtime-test mangler)
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
*Seneste update: 2026-06-04*
