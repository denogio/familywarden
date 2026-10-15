# Ulysses Mast — telefonen som en del af masten

Ulysses Mast er en lille Android-app til kaptajnens telefon. Mens du er bundet af en
pagt og blokeret efter din tidsplan, spærrer den **hotspot og tethering** (Wi-Fi, USB,
Bluetooth). Et hotspot, der allerede kører, slukkes. Resten af tiden gør appen intet.

Appen er *Device Owner* på telefonen. Det er Androids egen mekanisme til firmatelefoner,
og det er den eneste måde, en app kan spærre hotspot på mod ejerens vilje.

## Sådan virker den

- Appen henter pagt og tidsplan fra routeren på `http://<router>:7700/cgi-bin/mast`.
  Den port er også åben under en lockdown.
- Svaret er signeret med routerens nøgle (usign/Ed25519). Appen afviser alt, der ikke er
  signeret af den router, den er parret med, og svar der er ældre end det sidste.
- Appen gemmer det sidste svar og vurderer selv tidsplanen. Derfor virker den også, når
  telefonen ikke kan nå routeren, fx på arbejdet. Om dagen er hotspot tilladt. Om aftenen
  under en pagt er det spærret. En pagt slutter først, når routeren siger det, eller når en
  tidsbegrænset pagt udløber.
- Mens du er blokeret, er adb også slået fra. Mens telefonen er parret, er uret låst til
  netværkstid, så du ikke kan stille dig ind i et tilladt tidsrum.
- Android beskytter selv en Device Owner-app. Den kan ikke afinstalleres, dens data kan ikke
  slettes, og den kan ikke tvangsstoppes. Den eneste udvej er en fabriksnulstilling.

## Installation

Du skal bruge en computer med `adb` og telefonen med USB-fejlfinding slået til
(*Indstillinger → Om telefonen →* tryk 7 gange på *Buildnummer*, derefter
*System → Indstillinger for udviklere → USB-fejlfinding*).

### 1. Fjern konti midlertidigt

Android tillader kun at udnævne en Device Owner, når der ikke er konti på telefonen.

**Dine data forsvinder ikke.** At fjerne en konto er ikke det samme som at nulstille
telefonen. Billeder, filer, apps og SMS bliver liggende. Det, der er synkroniseret med
kontoen (kontakter, kalender, Gmail), forsvinder fra telefonen, men ligger stadig i
skyen og kommer tilbage, når du logger ind igen.

Tjek det her, før du går i gang:

- [ ] **Totrinskoder**: Bruger du en authenticator-app, så tag backup eller eksportér
      koderne først.
- [ ] **WhatsApp, Signal og lignende**: Hvis de står under *Adgangskoder og konti*, så tag
      en chat-backup først.
- [ ] **Google Wallet**: Kortene skal formentlig lægges ind igen.
- [ ] **Privat område eller arbejdsprofil**: De skal fjernes, og *deres* data forsvinder.

Fjern derefter alt under *Indstillinger → Adgangskoder og konti*. Det gælder ikke kun
Google-kontoen, men også password-managere, Signal og lignende.

`adb shell dumpsys account` viser, hvad der stadig er tilbage.

### 2. Installér og udnævn

Genstart telefonen og vent et minut. Android tjekker konti i baggrunden efter opstart.
Hent derefter `ulysses-mast-<version>.apk` fra den seneste
[release](https://github.com/denogio/familywarden/releases):

```sh
adb install ulysses-mast-0.8.0.apk
adb shell dpm set-device-owner me.hojgaard.ulysses.mast/.MastAdminReceiver
```

Får du *"already some accounts on the device"*, så vent lidt og prøv igen.
`adb logcat -b system | grep -i account` viser, hvilken konto der er i vejen.

### 3. Log ind igen

Læg dine konti ind igen. Det må du gerne nu, for Device Owner er sat.

### 4. Par med routeren

1. Åbn LuCI på hjemmenettet: *FamilyWarden → Settings → Ulysses → Ulysses Mast*.
2. Åbn **Mast** på telefonen og tryk **Scan pairing code**.
3. Scan QR-koden, og tjek at fingeraftrykket er det samme i appen og i LuCI.

Kaptajnens kort i LuCI viser nu "📱 Mast synced …".

**Når du laver en pagt**, så tjek at "Mast synced" er opdateret bagefter. Så ved du, at
telefonen har fået pagten. Er telefonen på hjemme-Wi-Fi, sker det inden for 15 minutter,
eller med det samme hvis du trykker **Sync now** i appen.

## Fjern Mast igen

**Remove Mast** i appen virker kun, når routeren lige har bekræftet, at der ikke er en
pagt. Appen fjerner så alle spærringer og holder op med at være Device Owner, og
derefter kan den afinstalleres som en almindelig app.

Under en pagt er eneste vej ud en fabriksnulstilling, eller sail-siden på port 7700
efterfulgt af karenstiden.

## Opdatering

```sh
adb install -r ulysses-mast-<ny version>.apk
```

Det virker kun uden for dine blokerede timer, for adb er slået fra, mens du er blokeret.
Opdateringer skal være signeret med samme nøgle som den installerede app. Nøglen ligger i
GitHub-secrets (`MAST_KEYSTORE_B64`) og lokalt i `~/.config/familywarden/mast-release.jks`.
**Mistes nøglen, kan appen kun erstattes efter en fabriksnulstilling.**

## Test

- `android/`: `./gradlew testDebugUnitTest` (Policy, usign, parring)
- `scripts/test-mast-emulator.sh <router-apk-dir>`: Android 15-emulator med debug-APK'en som
  Device Owner mod en FamilyWarden-router i Docker. Testen dækker pagt, frigørelse, ny pagt,
  genstart uden router, og at `pm clear` og afinstallation afvises. I CI kører den via
  *workflow_dispatch* med `mast_emulator`.
