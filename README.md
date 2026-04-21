# FamilyWarden - OpenWrt Parental Control

Profile-baseret parental control til OpenWrt med tidsplaner, internet-blokering og DNS-filtrering — eksponeret via ubus API så Home Assistant kan styre det.

## Status

🚧 Under udvikling - MVP på vej

## Struktur

```
familywarden/                 # Daemon pakke
├── usr/bin/familywarden.sh   # Hovedscript (shell)
├── etc/init.d/familywarden   # Init script
├── usr/lib/rpcd/familywarden # Ubus plugin
├── usr/share/rpcd/acl.d/     # ACL for HTTP adgang
└── etc/config/familywarden   # UCI konfiguration

luci-app-familywarden/        # LuCI webinterface
└── luasrc/                   # Lua MVC struktur
```

## Kompilering

```bash
# OpenWrt SDK
cd familywarden
make package

# LuCI app
cd luci-app-familywarden
make package
```

## Installation

```bash
opkg install familywarden_*.ipk
opkg install luci-app-familywarden_*.ipk

# Aktiver
/etc/init.d/familywarden enable
/etc/init.d/familywarden start
```

## API Eksempler

```bash
# List profiler
ubus call familywarden list

# Blokér profil
ubus call familywarden block '{"profile":"josva"}'

# Giv 30 min ticket
ubus call familywarden ticket '{"profile":"josva","minutes":30}'
```

## Licens

Apache 2.0
