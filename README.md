# FamilyWarden - OpenWrt Parental Control

[![CI](https://github.com/denogio/familywarden/actions/workflows/ci.yml/badge.svg)](https://github.com/denogio/familywarden/actions/workflows/ci.yml)

Profilbaseret parental control til OpenWrt: tidsplaner, internet-blokering per enhed
og tidsbegrænsede "tickets" — eksponeret via ubus, så LuCI og Home Assistant kan styre det.

Tidsplaner og tickets håndhæves af kernen (nftables `meta hour`/`meta day` og et
timeout-set), så der kører ingen daemon eller cron på routeren.

## Installation

Hent de to `.apk`-filer fra seneste [release](https://github.com/denogio/familywarden/releases)
(OpenWrt 25.12+, arkitekturuafhængige):

```sh
apk add --allow-untrusted ./familywarden-*.apk ./luci-app-familywarden-*.apk
```

Herefter: **Services → FamilyWarden** i LuCI.

## Struktur

```
familywarden/                     # Backend-pakke
├── usr/lib/rpcd/familywarden     # ubus API + nft include-compiler (→ /usr/libexec/rpcd)
├── etc/init.d/familywarden       # procd: regenerér ved boot + UCI-ændring
├── etc/config/familywarden       # UCI-konfiguration
└── usr/share/rpcd/acl.d/         # ACL

luci-app-familywarden/            # LuCI-frontend (JS views)
scripts/                          # build, test, deploy
test/e2e/                         # Playwright-tests mod QEMU
```

## Udvikling

```sh
./scripts/test-syntax.sh          # statiske checks
./scripts/test-integration.sh     # rigtig fw4 + nftables i OpenWrt-container (docker)
./scripts/build-apk-docker.sh     # byg apk'er med OpenWrt SDK → dist/
./scripts/test-apk.sh             # installér/afinstallér de byggede apk'er i container
./scripts/deploy.sh root@router   # kopiér kildefiler direkte til en router
```

CI kører test → build → apk-smoke-test på hver push. Et tag `vX.Y.Z`, der matcher
`PKG_VERSION` i `familywarden/Makefile`, udgiver en GitHub-release med apk'erne.

## API

```sh
ubus call familywarden list
ubus call familywarden status '{"profile":"josva"}'
ubus call familywarden block  '{"profile":"josva"}'
ubus call familywarden allow  '{"profile":"josva"}'
ubus call familywarden reset  '{"profile":"josva"}'
ubus call familywarden ticket '{"profile":"josva","minutes":30}'
ubus call familywarden pact   '{"profile":"josva","minutes":420}'
ubus call familywarden reload
ubus call familywarden version
```

## Ulysses — bind dig selv til masten

- **Lås** (profil-indstilling): ændringer af en låst profil træder først i kraft 24 timer
  efter seneste ændring. Allow og tickets afvises; Block virker altid.
- **Pagt** (🔒-knappen): internettet slukkes til et valgt klokkeslæt og kan ikke ophæves
  eller forkortes fra LuCI eller Home Assistant — kun forlænges.
- Nødudgang, kun som root over SSH: `familywarden sailing-home <profil>` — og du skal kunne
  fuldføre linjen fra Josh Garrels' "Ulysses"

## Licens

Apache 2.0
