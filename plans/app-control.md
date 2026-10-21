# App control: Family Link and Switchly alternative

Status: **implementation authorized**, branch `feature/app-control`, based on v0.9.1.
No app-control feature code has been written yet. The phased checklist and acceptance
gates live in [app-control-todo.md](app-control-todo.md).
Decisions below are the user's unless marked *proposed*. Production deployment and
physical-device tests still require separate authorization.

## Goal

Per-app time control on Android phones, self-hosted by each household on its own network:

- Children: daily/weekly limits per app or category, downtime/bedtime, approval of new
  apps, usage overview — and the gap Family Link leaves: **ad hoc extra time**. A child
  asks for "15 more minutes of app X today"; a parent approves or denies in the app,
  without editing that weekday's standing limit.
- Adults who want it: self-limitation (Switchly-like), using the Ulysses idea that
  limits can only be tightened while a pact lasts.

## Confirmed decisions (2026-10-19)

1. **Self-hosted per household.** No hosted service run by the maintainer. Requiring
   OpenWrt is fine, but router-specific parts must sit behind an adapter so OPNsense and
   pfSense can be added later.
2. **New backend in Go** for app control, enrollment/policy and requests. Router
   features (firewall per MAC, lockdown, Ulysses hotspot) remain router adapters.
3. **Replaces Family Link** on the children's phones, with extended functionality; no
   coexistence layer (Family Link has no API to build on).
4. **Parents approve in the FamilyWarden app** (a parent role on the parent's phone).
5. **Notifications are FamilyWarden app notifications**, not a separate push app.
6. **Go** for the core (2026-10-19).
7. **Device Owner on children's phones where possible** (robust: suspend apps, block
   uninstall/factory reset/safe boot/new users). A weaker mode without Device Owner (accessibility
   service + usage access + device admin, as many commercial apps do) is possible later
   but bypassable; not first.
8. **Adult self-limit scope:** social media and browsers, limited to what is necessary:
   app suspension by schedule and/or daily quota, the same services blocked in DNS
   (catches web versions and in-app browsers), pact semantics (only tighten).
9. Target compatibility: **Pixel 7 Pro, Android 17**. The son's daily phone is **not**
   a test device; use separately authorized disposable environments or a spare phone.

## Proposed architecture

- **Core daemon** (Go, one static binary): profiles, phones, keys, signed policy,
  app rules, requests/grants, usage summaries. Runs on the router (OpenWrt package) and
  is the authority. Not reachable from the internet.
- **Router adapters:** OpenWrt first (the existing rpcd/fw4 code can stay the firewall
  adapter at first); OPNsense/pfSense later (FreeBSD, pf). DNS/AdGuard sync moves into
  the core since it is not router-specific.
- **Android app:** one app with roles from the signed binding — managed child device,
  parent (approver), adult self-limit. Enforcement on the phone as Device Owner:
  `setPackagesSuspended` for blocked/exhausted apps, usage stats for counting, offline
  enforcement from the last signed policy (as Ulysses does today).
- **Messages:** request and approval are signed by the phone keys (child key / parent
  key, both approved in LuCI) and carry request ID, scope (app, minutes, day) and expiry,
  so an approval can't be replayed or widened. The child's phone can verify an approval
  directly; the router learns about it afterwards for the log.
- **Transport (proposed):** the app keeps its own connection to a relay with an
  ntfy-compatible protocol; payloads are end-to-end encrypted and signed, so the relay
  can't read, forge or approve. Households choose their own relay or a public one.
  This is not decided. FCM is not inherently incompatible with self-hosting; compare
  concrete service/credential dependencies, battery use and reliability before choosing.
  Signing alone does not encrypt payloads.

## Contract decisions (2026-10-21)

The user's answers to the P1 questions. Notes in *italics* are consequences or open
follow-ups recorded with them, not additional decisions.

1. **Split (A6):** the Go core owns identity, signed policy, DNS sync and app control;
   the shell plugin remains the OpenWrt adapter for firewall, schedules, tickets and pacts.
2. **Who may authorize extra time:** the router **and** a parent's phone. A grant signed by
   an approved parent phone is valid on the child's phone directly, also away from home.
   *Requires: approved parent keys delivered inside the router-signed policy; grants bound
   to child, app, minutes and request ID; single use; a short validity window. Revoking a
   parent key reaches an offline child phone only at its next sync.*
3. **Grants vs bedtime:** a parent can give extra time **also after bedtime/outside allowed
   windows** — an explicit parent grant wins over downtime and schedule limits for the
   granted app and minutes. *Adult self-limits under a pact are unaffected: no self-approval
   beyond the daily pool.*
   - **Grant lifetime:** unused granted minutes do not expire at midnight; a grant is valid
     for **2 hours after approval**.
   - **Network:** an app grant also opens the child's **internet at the router** (a
     profile ticket) for the same period, so an online app works after bedtime at home.
4. **Budgets are shared per child** across all the child's devices. *Offline devices cannot
   know what the others spent: each device enforces the remaining shared budget it last
   received, so two devices offline at once can overspend by at most that remainder.
   Reconciled at sync; overspend is reported, never silently ignored.*
5. **What counts as use:** foreground time with the screen on. Background audio with the
   screen off (e.g. Spotify) does **not** count. With the screen on, the visible foreground
   app counts, and **picture-in-picture counts** for the PiP app.
8. **Day boundary:** a new day starts at **midnight in the router's timezone**, also when the
   child travels to another timezone.
6. **Retention (A8):** 30 days for per-app activity and for visited websites. *Websites are
   available as **domains** from AdGuard Home's per-client query log, not full URLs or page
   content; DNS misses apps with their own encrypted DNS. Data stays in the household
   (router/AdGuard host), deleted after 30 days. The earlier "no per-site logs" proposal
   is superseded.*
7. **Requests (A7):** no rate limit; a child has at most **one open request** at a time;
   an unanswered request **expires after 1 hour**.

## Open decisions

| ID | Question | Proposal | State |
|---|---|---|---|
| A1 | Go or Rust? | Go | **Decided** |
| A2 | Notification transport | App-held connection to an ntfy-compatible relay, E2E encrypted; own relay or public | Open |
| A3 | Children's Google accounts with our Device Owner | Separate ordinary, unsupervised Google account per child; warn about Google's account rules in README | **Decided** |
| A4 | Adult self-limit: how to get access when really needed? | A small daily pool the user may spend without approval; nothing beyond it during a pact | **Decided** |
| A5 | Children's phones | Pixel 7 Pro, Android 17 | **Decided** |
| A6 | Core vs existing shell backend | Core takes identity, policy, DNS and app control; shell stays the OpenWrt firewall/pact adapter until ported | **Decided** |
| A7 | Request abuse and limits | No rate limit; one open request per child at a time; a request expires after 1 hour | **Decided** |
| A8 | Usage data and privacy | 30 days for app activity **and** visited websites (domains), on the household's own infrastructure | **Decided** (details below) |
| A9 | Device Owner setup for others | Like Tabaa: the app pairs with the phone's own *Wireless debugging* (user types the pairing code into a notification) and makes itself Device Owner, no computer; adb from a PC as alternative; QR provisioning only for new or reset phones | Open |
| A10 | Support a light tier without Device Owner? | Yes, after the strict tier; bypass-evident with parent alerts | Open |
| A11 | Android developer verification (2027) | Register the package as a verified developer, or rely on limited distribution / ADB | Open |

## Research: what is possible without Device Owner (2026-10-19)

Commercial and open-source blockers without Device Owner (Opal, Qustodio, Kidslox,
OpenParental, Flint) all compose the same primitives, because Android has no
"block this app" API for ordinary apps:

| Need | Without Device Owner | With Device Owner |
|---|---|---|
| Detect foreground app | AccessibilityService (instant) or UsageStats polling (~1 s) | Same |
| Block an app | Cover it (accessibility overlay) and send Home; re-triggers continuously | `setPackagesSuspended`: greyed out, can't start |
| Daily limits, usage overview | UsageStats (user grants Usage access) | Same; owner can avoid the manual grant (to verify) |
| Websites | Accessibility reading the URL bar (fragile per browser), or a local `VpnService` DNS filter (takes the only VPN slot) | Locked strict Private DNS (built) |
| Prevent uninstall/disable | Not possible; at most block Settings screens via accessibility and alert the parent | `setUninstallBlocked`, user restrictions |
| Bypass | Turn off accessibility, revoke Usage access, safe mode, force stop, factory reset, OEM battery killers; Android 17 "Advanced Protection" (opt-in) revokes accessibility for non-accessibility apps | Factory reset only, which can also be restricted |

Consequences:

- Without Device Owner the honest model is **bypass-evident, not bypass-proof**
  (OpenParental's wording): every disable or "went dark" becomes a parent alert.
- **Sideloading friction:** since Android 13, sideloaded apps need "Allow restricted
  settings" before accessibility can be enabled; Android 15 extends this to usage access,
  device admin and display-over-apps. Each household must do this by hand.
- **Android developer verification** affects FamilyWarden either way: from 2027 apps on
  certified devices must be registered by a verified developer (regional start 30 Sep
  2026 in BR/ID/SG/TH). Options: full registration of `io.github.denogio.familywarden`
  (identity verification), "limited distribution" (up to 20 devices, no ID check), or
  users' "advanced flow" for unregistered apps. ADB installs are unchanged, so the
  current QR/adb Device Owner setup keeps working.

Proposed conclusion: **two tiers.**

- **Strict (Device Owner)** — default for children's phones; already built and proven.
- **Light (no reset)** — accessibility + usage stats + bypass alerts, for phones that
  can't be reset, older teens, or adults who want soft limits. Later, after strict works.

Sources: Tabaa "Device Owner explained" (account/app/network/device levels); Android Authority on Android 15 restricted settings; Google Play
AccessibilityService policy; Android developer verification guide (updated 2026-08-18);
OpenParental README; Flint "Android technical strategy" research.

## Children's Google accounts (research 2026-10-19)

Device Owner via adb does **not** need a factory reset: sign out of all accounts, grant
Device Owner, sign back in; apps and data stay (done this way on the user's Pixel 10;
also how Tabaa sets up children's and adults' phones). Only QR provisioning on the setup
wizard needs a reset. The open question is
the child's **Google account**: under 13 (also in Denmark) a Google account must be
supervised by Family Link, and signing it in turns on Family Link supervision, which
makes itself a device administrator ("Family Link parental admin"). Google documents
nothing about doing that on a phone with another Device Owner. Reports conflict: a
child account could not use Family Link on an Intune fully managed device (unresolved);
on Samsung kiosk devices (Hexnode) Family Link was blocked by Knox policy ("Security
policy prevents turning on device administrators") and worked once allowlisted. Our
app does not forbid other device admins, so coexistence is plausible but unproven.

| | Setup | Pros | Cons |
|---|---|---|---|
| A | FamilyWarden Device Owner + supervised child account, Family Link limits left off | Play Store, Gmail as usual | Unknown if Google allows it; two systems on the phone |
| B | FamilyWarden Device Owner, no Google account | Most robust; only our rules | No Play Store with the child's account (parent installs, or Aurora Store); some apps need Google sign-in |
| C | No Device Owner: light tier + Family Link | No account removal | Bypassable (see above) |

**Tested 2026-10-20 (emulator, Android 17 with Google Play, FamilyWarden as Device
Owner, no accounts): option A is not possible.** Signing in the supervised account makes
Family Link ask Android to become **Profile Owner** of the primary user
(`android.app.action.SET_PROFILE_OWNER`; "This device will be managed and monitored by
Family Link parental controls"). A user that already has a Device Owner cannot get
another Profile Owner, so the activation never completes and setup loops ("Waiting for
profile owner activation result"); the account is not added. This follows from Android's
one-owner rule, not from the emulator. **Family Link and FamilyWarden as Device Owner
are mutually exclusive in this setup.** The earlier no-Google-account fallback is
superseded by the separate ordinary-account decision below.

**Decided 2026-10-20: each child uses a separately created Google account** on their
phone (not a Family Link supervised account), for Play Store and Google apps.
FamilyWarden replaces Family Link's controls, including **usage collection**: the phone
records per-app usage (UsageStats) and reports daily summaries to the router; parents see
them in the FamilyWarden app and LuCI. Notes:

- **Confirmed after vendor research:** use separate ordinary accounts and document
  the risk in README. An unsupervised account for a child below Google's applicable
  minimum age may violate Google's rules and lead to restrictions or suspension.
  Minimum ages vary by country; check Google's current requirements. A separate or
  parent-owned account is not a guaranteed exception. Do not recommend a false date
  of birth. This is a project setup choice, not evidence of Google's approval.
- New apps are suspended until a parent approves them; Play Store can be suspended
  outside approved installs.
- Usage access: Device Owner has no API to grant it, but setup already uses adb, which can
  run `appops set <package> GET_USAGE_STATS allow`. To verify on the user's Pixel 10.

Vendor evidence: Tabaa documents removing accounts during Device Owner setup and
signing back in afterwards, but does **not** explain supervised-child-account handling.
Earlier claims here that Tabaa recommends a parent's account were inference, not a
vendor recommendation. TimeLimit explicitly documents the Family Link conflict and
an unsupported recovery-file workaround, which is not our recommended setup.
Sources: https://tabaa.net/en/children/ and https://timelimit.io/en/.

**Decided 2026-10-20: the son's Pixel 7 Pro is not a test device** (he uses it daily).
A must be tested elsewhere (emulator with Google Play, or a spare phone) before it ever
reaches his phone. Earlier plan, kept for reference: test A on the Pixel 7 Pro without a reset: remove the supervised account (needs
the parent's password; turns Family Link off on the phone) and any others → Device Owner
via adb → sign the supervised account back in. If Family Link can't supervise next to
our Device Owner, B is the robust default (or remove our app to give the phone back to
Family Link).

## Device experiments (separate authorization required)

1. **User's own Pixel 10** (already Device Owner with FamilyWarden, no reset needed):
   app suspension behavior, whether the app can grant itself usage access as Device
   Owner (or needs one tap), and battery cost of an app-held relay connection (A2).
2. **Not on the son's phone:** emulator with Google Play or a spare phone: Device Owner
   via adb and account compatibility (A3). The supervised-account emulator attempt
   looped during activation; ordinary separate accounts are now the chosen setup.
   Further tests require explicit authorization; do not use the son's daily phone.

## Not in scope for now

iPhone (no equivalent API outside Screen Time), web dashboards, a maintainer-run service,
full-URL or page-content history (domain history is in scope, see decision 6).
