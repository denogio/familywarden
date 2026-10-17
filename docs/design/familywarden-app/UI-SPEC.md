# FamilyWarden Android: UI specification

Status: planning. Target: Jetpack Compose + Material 3, minSdk 30, targetSdk 35,
application ID `io.github.denogio.familywarden`.

Mockups live on the design canvas, page **FamilyWarden app** (light and dark for every
screen). This document is the contract for building them as stateless Compose UI. The
state types are in `FamilyWardenUiState.kt` next to this file.

## 0. Labels used everywhere

| Label | Meaning |
|---|---|
| **Example data** | Illustrative value: hostnames (`phone-id.dns.example.net`), policy names, categories, profile names, versions. Never a product default. |
| **Future** | A concept with a reserved place. Not functionality, never an empty screen. |
| **Needs router support** | Depends on a planned backend extension. The UI must handle "router doesn't support this" as a real state. |

## 1. Information architecture

### Destinations

A fixed, small set of destinations, each with a visibility predicate. No plugin
framework; adding a destination is adding an enum value and a predicate.

| Destination | Visible when | Answers |
|---|---|---|
| Overview | Setup complete | Is this phone protected, and is anything for me to do? |
| DNS | Paired | What exactly is enforced, and is the resolver working here? |
| Ulysses | Feature `On` or `LockedOn`, **or** any cached pact is active | What is my pact doing to hotspot, and when does it change? |
| Family *(Future)* | `capabilities.administrator == true` (always false in v1) | Managing other profiles and devices. Not designed. |
| Settings | Always | Features, pairing, device and profile, device owner, diagnostics, removal |

Before setup is complete there is no bottom bar: the onboarding flow (Device owner,
Pairing, DNS) takes the whole screen.

### Pattern: Material 3 NavigationBar (bottom bar)

- 3 items for an ordinary managed device, 4 with Ulysses, at most 5 with Family. Five is
  the Material limit; anything beyond goes into Settings or under Family, never a 6th tab.
- With Ulysses absent the app is complete: Overview, DNS, Settings each have real content.
- No empty tabs, no "coming soon" destinations. Future admin is a documented concept only.
- Order is stable: Overview, DNS, Ulysses, Family, Settings. Items appear in place; they
  never reorder.

Why not a drawer: three to five destinations, used one-handed, rarely visited. A bottom
bar keeps the current state one tap away and makes it obvious when Ulysses is present.

### Roles are separate

| Capability | Source | Effect |
|---|---|---|
| Managed device | Paired with a router | DNS protection and its status |
| Captain | Router-confirmed in a signed reply | Ulysses may be enabled |
| Administrator *(Future)* | Router | Family destination |

Captain does not imply administrator and the reverse. Nothing local can grant either.

## 2. State model: separate axes

The UI receives one `AppUiState`. It is composed of independent axes; no axis is
derived from another inside the UI layer.

| Axis | Type | Values |
|---|---|---|
| Setup | `SetupProgress` | NotDeviceOwner(readiness), NotPaired, ConfirmKey, ApplyingDns, Complete |
| Capabilities | `Capabilities` | association (Unknown, RouterUnsupported, Pending, Confirmed, Mismatch), captain, administrator |
| Ulysses feature | `UlyssesFeature` | Off, Checking, Ineligible(reason), Eligible, On, LockedOn |
| Policy | `PolicyState` | NotReceived, Verified(receivedAt, hostname, policyName?), Rejected(reason) |
| Android application | `DnsApplication` | NotApplied, Applying, Applied(locked), Failed(reason) |
| Resolver health | `ResolverHealth` | Untested, Testing, Reachable(testedAt), Unreachable(lastOkAt), CertificateInvalid(presentedName?, checkedAt) |
| Router sync | `RouterSync` | Never, Syncing, Reachable(lastSyncAt), Unreachable(lastVerifiedAt), BadSignature(lastVerifiedAt), OtherError(message, lastVerifiedAt) |
| Pact | `UlyssesState` | pact (null or Ongoing/Until, releaseAt), hotspot (NotRestricted, Allowed(nextLockAt), Blocked(nextOpenAt)), schedule, sailPage |
| Removal | `RemovalState` | Idle, Checking, Allowed(confirmedAt), RefusedPact, RefusedUnreachable, RefusedUnsupported |

### Truthfulness rules

1. Four facts are always shown separately on Overview: **Policy verified**, **Applied by
   Android**, **Resolver reachable**, **Router reachable**. There is no single
   "Protected" badge.
2. The Overview headline is derived from Policy, Application and Resolver only (table
   below). Router sync never changes the headline; it is its own fact row.
3. Router unreachable is **neutral**: normal away from home. The last verified policy stays
   in force until the next successful sync. **No 24/48 hour stale escalation.**
4. A policy created on the router is not shown as active on the phone until received and
   verified. The UI never shows router-side intent as phone-side fact.
5. Finite pacts and sailing-home releases expire on the phone at their time, without a sync.
6. Hotspot is only described as restricted when Ulysses is On/LockedOn and a pact exists.
   With Ulysses off, the app says nothing about hotspot.
7. Red (problem tone) is reserved for genuine faults: resolver unreachable, certificate
   invalid, application failed, bad signature, association mismatch. A pact holding is
   the night tone, never red.

### Overview headline

| DnsApplication | ResolverHealth | Tone | Headline |
|---|---|---|---|
| Applied | Reachable, Untested, Testing | Calm | DNS protection is on |
| Applied | Unreachable | Problem | Websites can't load on this network |
| Applied | CertificateInvalid | Problem | DNS certificate not valid |
| Applying | any | Plain + pending | Applying DNS protection |
| NotApplied | any | Attention | Finish setting up DNS protection |
| Failed | any | Problem | Android didn't apply DNS protection |

PolicyState.Rejected (bad signature on the policy itself) keeps the previous verified
policy applied and adds a Problem fact row; it does not change the headline.

### Fact rows on Overview

| Row | Ok | Neutral | Pending | Problem |
|---|---|---|---|---|
| Policy verified | Verified · time | – | Waiting for first policy | Rejected (bad signature) |
| Applied by Android | Strict Private DNS, locked | – | Applying | Failed (reason) |
| Resolver reachable | Answered over TLS · tested time | Untested | Testing | Unreachable / Certificate invalid |
| Router reachable | Last sync time | Out of reach · last verified time | Syncing | Bad signature |

## 3. Screens

### Overview (boards A1 to A4)
Hero (headline table), fact list, Ulysses summary card (only when the Ulysses destination
is visible), Sync now. Router offline shows a neutral row plus one line explaining the
cached policy (A2). The Ulysses card uses the night surface and states hotspot status and
the next real transition; it links to the Ulysses tab (A3).

### DNS protection (B1 to B3)
- Hero: application state ("Applied and locked").
- Resolver card: hostname (mono), mode "Strict · DNS-over-TLS, port 853", certificate
  status, last test, **Test connection** action.
- Filtering policy card, tagged **Example data**: policy name and category chips. Honest
  limit line: DNS can't see inside pages; apps with their own encrypted DNS and VPNs can
  bypass it.
- Locked note: always on while paired, independent of Ulysses and internet hours.
- Not present by design: disable button, opportunistic mode, unfiltered fallback,
  per-domain exceptions, query counts, history, analytics.
- Unreachable (B2): likely causes listed as possibilities, not diagnoses: port 853
  blocked, captive portal, resolver down. Guidance is "use mobile data" or "tell whoever
  runs your FamilyWarden". No bypass.
- Certificate invalid (B3): expected vs presented name, "this is not normal", no skip.
  Resolver states also put a small marker on the DNS tab; the marker is backed by the
  tab's accessible label ("DNS, needs attention").

### Ulysses (C1, C2)
Reuses the Mast states: no pact, bound/allowed, bound/blocked, finite vs ongoing,
sailing-home countdown, failed sync with cached pact retained. New: a 24 hour schedule
bar (with a text summary for TalkBack) and **Open sail page**, which opens the router's
page in the browser and is disabled with an explanation when the router is unreachable.
Pact creation, amendment and release requests stay on the router. No router password
field, no in-app pact admin.

### Settings (D1 to D4)
- **Features → Ulysses**: Off (switch off), Checking, Ineligible(reason), Eligible (sheet
  with the router's answers, D2), On, LockedOn (D3: switch disabled with lock icon and
  reason, status still reachable via Ulysses tab and Overview).
- **Router pairing**: scan QR, manual address, fingerprint confirmation (same flow as E2).
- **Device and profile** *(Needs router support)*: device, profile, association state,
  capability list (D4). Association states: Confirmed, Unknown, Pending, Mismatch.
- **Device owner**: readiness and status.
- **Version and diagnostics**: app version, compact copyable report (no browsing data).
- **Remove FamilyWarden**: see removal below.

### Onboarding (E1 to E3)
1. **Device owner readiness**: accounts present, USB debugging, device owner; adb command
   with copy. Honest notes: removing an account takes its synced data off the phone and
   re-adding may not restore everything; a failed enrollment can require a factory reset.
2. **Pairing and key**: scan (or manual address), show address and fingerprint, require an
   explicit "It matches" before trusting.
3. **Apply and verify DNS**: each step shown separately (policy verified, applied by
   Android, resolver test). If the resolver test fails on the current network, setup still
   completes and the DNS tab explains why.

### Removal (F1, F2)
Requirements, checked live: router answers now **and** confirms no active pact. A cached
"no pact" is never enough. Outcomes: Allowed → confirm dialog (F1); RefusedUnreachable
(F2); RefusedPact (the "pact still holds" dialog from Mast); RefusedUnsupported if the
router lacks the endpoint. Confirm copy does not promise lossless re-enrollment.

## 4. Ulysses visibility and locking

| Feature | Pact active (cached) | Nav item | Overview card | Features switch |
|---|---|---|---|---|
| Off | no | hidden | hidden | off, enabled |
| Off | yes | **shown** | shown | shows LockedOn: an active pact wins |
| Checking / Ineligible / Eligible | no | hidden | hidden | off, sheet shows progress or reason |
| On | no | shown | shown | on, enabled |
| On / LockedOn | yes | shown | shown | on, **disabled** with lock and reason |

- Enabling requires a router-confirmed captain; a local switch alone never makes anyone
  captain.
- Hiding or disabling a control is presentation only. Enforcement is Device Owner plus the
  router. The app must keep enforcing a cached pact even if the feature preference were
  somehow off.
- Pairing and removal stay refused during a pact at the logic layer, not just by hiding.

## 5. Components and tokens

Stateless composables (see the Components board):
`FwBottomBar`, `HeroCard(tone)`, `StatusFactList`, `UlyssesSummaryCard`, `KeyValueRow`,
`SettingsRow`, `LockedSwitch`, `ScheduleBar`, `Banner(tone)`, `LabelTag(kind)`,
`SetupStepper`, `Countdown`.

Tones: Ok (tertiary), Neutral (surfaceContainerHigh + onSurfaceVariant), Pending
(neutral + progress), Attention (secondary gold), Problem (error). Night is a surface,
not a tone. New tokens on top of the Mast theme: `ok/okTint`, `attention/attentionTint`,
`problem/problemTint` (values on the Components board). Brand: lighthouse small mark in
the app bar, mast for Ulysses.

## 6. Accessibility

- Every status has an icon and words; colour is never the only signal.
- Hero headline: heading + polite live region. Problem banners: assertive.
- Countdown: one semantics node, minute precision.
- Fingerprints read character by character.
- Schedule bar: image role with a text summary; the visible summary line repeats it.
- Key/value rows stack at font scale ≥ 1.5 (board A4 shows 200 %).
- Touch targets ≥ 48 dp. Edge-to-edge via Scaffold insets; bottom bar handles the
  gesture inset.

## 7. Backend dependencies and open questions

| Item | Status |
|---|---|
| Device/profile association and captain flag in a signed router reply | Needs router support |
| Removal endpoint that returns a fresh, signed "no active pact" | Needs router support (Mast logic exists; confirm it covers FamilyWarden) |
| Policy name and categories in the policy payload | Undecided; shown as Example data |
| Sail page URL from the router | Assumed available in the signed state |
| Resolver test method (DoT query to the configured hostname) | To confirm on Android 11+ without root |
| Device admin receiver class name in the adb command | Placeholder `.AdminReceiver` |
| Administrator capability and Family destination | Future, not designed |
| Switchly functionality | Not specified; deliberately not shown |

## 8. Board index

| Board | Shows |
|---|---|
| Navigation and capabilities | Destinations, predicates, 3/4/5 item bars, four facts, labels |
| Components and tokens | Tones, hero surfaces, component list, added tokens |
| A1 | Overview, ordinary user, Ulysses hidden, all healthy |
| A2 | Overview, router offline, DNS healthy on mobile data |
| A3 | Overview, Ulysses enabled, pact holding |
| A4 | Overview at 200 % font |
| B1 / B2 / B3 | DNS healthy / resolver unreachable / certificate invalid |
| C1 / C2 | Ulysses blocked with schedule / sailing home, router out of reach |
| D1 | Settings |
| D2 | Features, Ulysses opt-in eligibility sheet |
| D3 | Features, Ulysses locked during pact |
| D4 | Device and profile, capabilities, association states |
| E1 / E2 / E3 | Onboarding: device owner, pairing and key, apply and verify DNS |
| F1 / F2 | Removal: confirmed and confirm dialog / refused, router unreachable |
