# Design prompt: Ulysses Mast (Jetpack Compose)

Use with the screenshots in `docs/images/mast-screens.png` (current UI: not paired /
bound by a pact / released).

---

Design the Android app **Ulysses Mast** and deliver it as **Jetpack Compose + Material 3**
code. Attached are screenshots of the current, purely functional UI (plain Android
views). Keep the behaviour and information; redesign the presentation.

## What the app is

Ulysses Mast is part of FamilyWarden, a parental-control system for OpenWrt routers. Its
"Ulysses" feature is for an adult who wants to **bind themselves to the mast**, like
Odysseus passing the sirens: they choose a *pact* in advance (e.g. "no internet after
21:30") that they cannot talk themselves out of in the moment.

The router enforces the pact on the home network. This app closes the last loophole:
the phone's **mobile hotspot**. It is installed as *device owner* on the captain's phone,
and while they are bound by a pact and blocked by their schedule it disables hotspot and
tethering. Most of the time it runs silently in the background. The user opens it rarely:
to pair with the router, to check status, to sync, or to remove it once there is no pact.

**Tone:** calm, respectful, a little mythic. This is a commitment the user *chose*, not a
punishment. No shaming, no gamification, no alarm-red everywhere. The nautical / Odyssey
motif (mast, anchor ⚓, sea, the stars, sailing home) may be used with restraint, never
kitsch. The router's "sail page" uses a deep night-sea palette (gradient #1b2a3a → #070b10,
text #d8e1ea, muted #8aa0b4, gold accent #c9a227). The app may echo that as its brand.

## Platform and constraints

- Jetpack Compose with Material 3. Single activity, one main screen plus dialogs or bottom
  sheets. No navigation library needed.
- minSdk 30, targetSdk 35, **edge-to-edge** (handle system-bar insets).
- Light and dark theme. Use your own brand colour scheme; dynamic colour optional.
- Accessibility: WCAG AA contrast, TalkBack content descriptions, layouts that survive
  200 % font scale, never rely on colour alone (the blocked/allowed state also needs an icon
  and text).
- Keep dependencies minimal: Compose BOM, material3, activity-compose, and
  material-icons-extended at most. No image-loading or other heavy libraries. The APK size
  matters a little.
- Strings in English, in `strings.xml` (Danish translation comes later).

## Information available (do not change the logic)

```kotlin
// From the router (cached until the next successful sync)
data class MastState(
    val serverTime: Long, val captain: String,
    val pactActive: Boolean, val pactType: String,  // "until" | "ongoing"
    val pactUntil: Long,                             // epoch, 0 when ongoing
    val releaseAt: Long,                             // "sailing home" requested: free at this epoch, 0 = no
    val rules: Rules)                                // schedule: allow windows like Mon–Sun 06:00–21:30
enum class Reason { NOT_PAIRED, NO_CAPTAIN, NO_PACT, CAPTAIN_ALLOWED, BLOCKED }
data class Decision(val block: Boolean, val reason: Reason)
// Also available: isDeviceOwner, router host, key fingerprint (16 hex chars),
// lastSync (epoch), lastError ("router unreachable" | "bad signature" | "stale reply" | …),
// Policy.nextChange(...) = when the blocked/allowed state next flips (e.g. today 21:30).
```

Actions: **Sync now**, **Scan pairing code** (Google code scanner → QR from the router's
web UI), **Pair manually** (enter router address → confirm key fingerprint), **Remove
Mast** (only possible when a fresh sync confirms there is no pact; otherwise it is
refused). Pairing and removal are disabled while bound.

## States to design (each as a mock-up and a `@Preview`)

1. **Not device owner.** Setup needed. Show the adb command
   `adb shell dpm set-device-owner me.hojgaard.ulysses.mast/.MastAdminReceiver` (copyable)
   and a short explanation.
2. **Device owner, not paired.** Primary call to action: scan the pairing code from
   FamilyWarden → Settings → Ulysses. Secondary: pair manually.
3. **Pairing confirmation.** Router address and key fingerprint, with "check that it
   matches the one in LuCI".
4. **Paired, no pact.** Hotspot allowed. Quiet, "free sailing".
5. **Bound, currently allowed.** Pact active, but inside an allowed window. Show when
   blocking starts (next change, e.g. "Hotspot locks at 21:30").
6. **Bound, blocked now.** The key state: hotspot and tethering are locked, until when
   (next change, e.g. "until 06:00"), ongoing pact versus "bound until <date>".
7. **Sailing home.** Release requested on the router: free at <time>, with a countdown.
8. **Sync problems.** "Router unreachable" is normal away from home: neutral, show the last
   sync time, the cached pact still applies. "Bad signature" is not normal: show a clear
   warning.
9. **Remove Mast.** The refusal message while bound, and the confirmation dialog when
   allowed.

Also show the secondary details compactly: captain, router and fingerprint, last sync,
device owner status.

## Deliverables

1. Mock-ups of all states in light and dark, plus a short design rationale (layout,
   hierarchy, motif).
2. Design tokens: colour scheme (light/dark), typography, shapes, spacing.
3. Compose code:
   - `ui/theme/Color.kt`, `Type.kt`, `Theme.kt` (`MastTheme`)
   - a `MastUiState` model (sealed interface or data class) mapped from the data above
   - stateless composables per section and state, plus `MastScreen(state, actions)`
   - `@Preview` for every state, light and dark, including 200 % font scale for the
     blocked state
   - notes on wiring it into the existing `MainActivity` (it already has the logic:
     refresh/sync, pairing, release), and the Gradle dependency changes
4. Keep business logic out of the UI layer. The composables only render state and call
   action lambdas.
