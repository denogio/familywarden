# FamilyWarden — Roadmap for de resterende features

*Oprettet 2026-10-14, udgangspunkt `v0.7.3` (backend) / `0.4.0` (LuCI).*

Rækkefølgen er valgt sådan, at hver fase bygger på den forrige, og at API'et er
stabilt, før Home Assistant begynder at afhænge af det.

| # | Feature | Størrelse | Afhænger af | Version |
|---|---------|-----------|-------------|---------|
| 1 | ~~Ulysses-pagt (selvbindende lås)~~ ✅ | M | — | backend 0.8.0, luci 0.5.0 |
| 2 | Traffic per profil (nlbwmon) | S | — | backend 0.9.0, luci 0.6.0 |
| 3 | Dansk oversættelse (i18n) | S | 1+2 (strenge er faste) | luci 0.6.x + `luci-i18n-familywarden-da` |
| 4 | Home Assistant (HACS) | L | 2 (`pact_until` i API) | eget repo, 0.1.0 |
| 5 | DNS-filtrering | M–L | — | backend 1.0.0 |

## Arbejdsregler (alle faser)

- **Definition of done = adfærds-bevis.** Hver ny backend-adfærd får en case i
  `scripts/test-integration.sh` (rigtig fw4/nftables i OpenWrt-container), *før* fasen
  er færdig. Rettelser i regelgenereringen starter med en test, der fejler.
- Hver ubus-metode skal annonceres i `list`, dispatches i `call`, stå i begge ACL-filer
  og svare med **ét** JSON-objekt (`test-syntax.sh` og `test-integration.sh` håndhæver det).
- Versionsbump i Makefile → merge → `git tag vX.Y.Z` → CI udgiver release.
- Én feature-branch og én PR per fase; CI skal være grøn før merge.

---

## Fase 2 — Traffic per profil

**Status:** frontenden findes allerede (`showTrafficModal` i `overview.js`, kalder
`traffic` og forventer `{available, daily, day, week, month}` eller
`{available, period, period_start}`). Backend-metoden mangler.

**Design**
- `ubus_traffic <profile>`: hvis `nlbw` mangler → `{"available":false}`.
- Ellers `nlbw -c json -g mac` → summér rx+tx for profilens MAC'er.
- nlbwmon tæller per *periode* (typisk måned), ikke per dag. Derfor:
  - Standard: `{available:true, daily:false, period:<bytes>, period_start:<dato>}`
    ud fra den aktive database.
  - Dag/uge kræver flere databaser (`nlbw -c json -t YYYY-MM-DD`) — kun hvis
    nlbwmon er sat op med daglige intervaller (`option commit_interval`/`database_interval`).
    Afgør i fasen om det er værd at understøtte, eller om UI'et kun viser "denne periode".
- `nlbwmon` bliver en *valgfri* afhængighed (står ikke i `DEPENDS`).

**Opgaver**
- [ ] `ubus_traffic` + `list`-skema + `call`-dispatch + ACL (read)
- [ ] Integrationstest: uden `nlbw` → `available:false`; med en fake `nlbw` på PATH,
      der returnerer kendt JSON → korrekt sum for en profil med to MAC'er
- [ ] Verificér formatet fra `nlbw -c json` på rivendell, før parseren skrives
- [ ] Playwright: modalen viser tal eller "unavailable"-teksten (ikke stille lukning)

**Beslutning til dig:** byg den (anbefalet; det er lille, og UI'et findes) eller fjern
traffic-knappen.

---

## Fase 1 — Ulysses-pagt ✅ (0.8.0, branch `feature/ulysses`)

**Endeligt design (besluttet 2026-10-14)** — erstatter PLAN.md § 14:
- **Stående lås** (`option lock '1'`): alle ændringer af profilen (tidsplan, enheder, policy,
  slå fra, låse op) træder først i kraft **24 t efter seneste ændring**. Låsning gælder straks.
- **Ad hoc-pagt** ("lås mig til 06:00"): blokerer profilens enheder til et klokkeslæt; kan kun
  forlænges. Håndhæves af kernen (`meta time`).
- Allow/ticket afvises for låste profiler og under pagt; Block/Reset virker.
- Nødudgang: `familywarden sailing-home <profil>` som root over SSH (kræver at fuldføre en linje fra Josh Garrels' "Ulysses").
- Testet: 37 integrations-checks + UI i LuCI-container (Playwright).

Mangler før merge: test på rivendell (Save & Apply → procd-trigger → pending; reboot-persistens).

<details><summary>Oprindelig plan (historisk)</summary>


Fuldt designet i `PLAN.md` § 14 (besluttet med dig: hele profilen fryses, eneste
escape er root over SSH, og pagten skal overleve reboot). Planen følges med disse
justeringer til den nuværende arkitektur:

- State i `/etc/familywarden.pacts` (persistent, mode 600), format `profile=expiry_epoch`.
- **Håndhævelse i kernen, ikke kun i UI:** under en pagt emitteres hverken
  allow-overrides eller ticket-elementer for profilen, så en lækket ubus-adgang ikke
  kan åbne den. Blokerede `block/allow/reset/ticket` svarer
  `{"error":"locked by Ulysses pact","locked_until":<epoch>}`.
- Pagter udløber, når `compile_and_apply` kører og GC'er dem. Men en *blok*-pagt skal
  slippe af sig selv på klokkeslættet, også uden en reload. Løsning: oversæt
  pagtens udløb til en kerne-evalueret regel (`meta time < "<ISO-tid>"`), så kernen
  selv slipper den, ligesom med schedules. Afklares/verificeres i containeren først.
- `ubus_pact` må kun **forlænge**, aldrig forkorte. Der er ingen `unpact`.

**Opgaver**
- [ ] Backend: `pact_*`-helpers, `gc_expired_pacts`, `ubus_pact`, afvisning i
      overrides, `pact_until` i `list`/`status`
- [ ] Regelgenerering: ingen allow/ticket for pacted profil; tidsbegrænset drop
- [ ] ACL: `pact` (write) i begge ACL-filer
- [ ] UI: "Bind til masten"-knap med klokkeslæt-vælger og advarsel; frosset profil
      med lås-banner og nedtælling
- [ ] Integrationstest: pagt → overrides afvises; ingen accept-regler; pagtfil overlever
      `/etc/init.d/familywarden restart`; forkortelse afvises; udløbet pagt GC'es
- [ ] QEMU: reboot-test (procd + rigtig boot kan ikke testes i containeren)

**Beslutning til dig:** er designet fra juni stadig det, du vil have?

---

</details>

---

## Fase 3 — Dansk oversættelse

De gamle Lua-oversættelser forsvandt med Lua-frontenden. Moderne LuCI bruger `.po`-filer.

**Opgaver**
- [ ] Sørg for at alle synlige strenge i views går gennem `_()` (tjek profiles/overview/info)
- [ ] `luci-app-familywarden/po/templates/familywarden.pot` (genereres med LuCI's
      `scripts/i18n-scan.pl`) + `po/da/familywarden.po`
- [ ] `luci.mk` bygger så automatisk `luci-i18n-familywarden-da` — tilføj den til
      CI-artifacts, release og `test-apk.sh` (`.lmo` installeres under `/usr/lib/lua/luci/i18n/`)
- [ ] CI-check: `.po` parser (`msgfmt --check`), og ingen strenge mangler i forhold til `.pot`

---

## Fase 4 — Home Assistant (HACS)

**Repo:** eget repo, `denogio/hass-familywarden`. HACS kræver én integration per
repo med `custom_components/familywarden/` i roden.

**Transport:** ubus over HTTP (`POST http://<router>/ubus`, JSON-RPC). Login med
`session.login` og et særskilt rpcd-login (`/etc/config/rpcd`), som kun har ACL'en
`familywarden` — ikke root. Sessionen fornyes ved udløb.

**Entiteter per profil**
- `switch.<profil>_internet`: on = `allow`, off = `block`. Attributter: policy, override,
  override_expires, pact_until
- `sensor.<profil>_status`: allowed/blocked (følger schedule live)
- `button.<profil>_reset`: tilbage til schedule/policy
- `sensor.<profil>_traffic` (hvis fase 1 er `available`)
- Services: `familywarden.ticket` (profil, minutter) og `familywarden.pact`
- Under en aktiv pagt er switch/button utilgængelige, og attributten viser `locked_until`

**Teknik:** config flow (host, bruger, password, verify SSL), `DataUpdateCoordinator`
med polling af `list` hvert 30. sekund og opdatering straks efter hver handling,
diagnostics. Lovelace-kort er valgfrit; et eksempel-dashboard i YAML er nok i v0.1.

**Test/CI:** `pytest-homeassistant-custom-component` med mocket ubus, `hassfest` +
HACS-validering i GitHub Actions, release ved tag (HACS læser GitHub-releases).
Ende-til-ende mod `openwrt/rootfs`-containeren med rpcd, hvis rpcd kan køre dér
(skal undersøges), ellers mod QEMU.

**Forudsætning i dette repo:** en vejledning (README) i at oprette rpcd-brugeren +
evt. en `uci-defaults`-hjælper.

---

## Fase 5 — DNS-filtrering

**Modelvalg (anbefaling: per-klient DNS-redirect i nft).**

| Model | + | − |
|-------|---|---|
| **nft DNAT per MAC** til filtrerende resolver | Passer til den deklarative arkitektur (endnu en chain i includet), kernen håndhæver, ingen ekstra tjenester | Filtreringen sker hos en ekstern resolver |
| dnsmasq + nftset | Lokale bloklister | dnsmasq kan ikke differentiere per klient uden flere instanser; tungt |
| AdGuard Home API | Rige bloklister, per-klient-regler | Kræver AGH på routeren eller i netværket; ekstern afhængighed og API-state |

**Design (nft-model)**
- `option dns_profile` (feltet findes allerede i LuCI) → en resolver:
  `family` = Cloudflare 1.1.1.3 / 1.0.0.3 (+ IPv6), `strict` = CleanBrowsing Family,
  `custom` = `option dns_server`.
- Ny chain `familywarden_dstnat` (hook prerouting, priority dstnat): udp/tcp 53 fra
  profilens MAC'er → DNAT til resolveren (ip og ip6).
- Mod omgåelse: drop tcp/udp 853 (DoT) fra profilens MAC'er. dnsmasq svarer NXDOMAIN
  på `use-application-dns.net`, så Firefox slår DoH fra. Kendt begrænsning: hårdkodet
  DoH i apps kan ikke stoppes uden en DoH-blokliste (evt. senere som nft-set).
- Ticket/override påvirker *ikke* DNS-filteret (filteret er en grundindstilling, ikke
  en straf). Bekræft med dig.

**Opgaver**
- [ ] Generator for `familywarden_dstnat` + oprydning før `fw4 reload`
- [ ] UI: `dns_profile` bliver en dropdown + `dns_server`-felt ved `custom`
- [ ] Integrationstest: DNAT-regler per MAC for ip og ip6, 853-drop, ingen regler
      uden `dns_profile`, gyldigt include
- [ ] Runtime-test på rivendell: `nslookup` af en voksenside fra en profil-enhed
      → blokeret svar

**Beslutning til dig:** modelvalg, og om ticket skal slå DNS-filteret fra.

---

## Løbende / teknisk gæld

- [ ] Kør `/understand` igen — knowledge-graphen er fra `66c2701`
- [ ] QEMU i CI (GitHub-runnere har KVM): procd-trigger, reboot-persistens, Playwright.
      Det er det eneste, containeren ikke kan dække (PLAN.md § 15 Lag 1, punkt 2, 5 og 9)
- [ ] `PLAN.md` og `new-thoughts.md` er historiske — arkivér dem til `plans/archive/`
- [ ] LuCI i OpenWrt-container (ubusd + rpcd + uhttpd, stub for `system`) virker til UI-test —
      kan blive et Playwright-job i CI uden QEMU
- [ ] `PKG_MAINTAINER`/copyright i Makefiles står stadig som "Dennis Højgaard <github@dehoej.dk>"
