# FamilyWarden Android — consolidated design prompt

Use this in the existing Claude Design conversation, where you already designed
Ulysses Mast and the FamilyWarden lighthouse identity.

---

Evolve your existing Ulysses Mast design into the broader **FamilyWarden Android app**.
You already know the Ulysses concept and created the FamilyWarden lighthouse logo.
Reuse that work; do not redesign the brand or repeat the product discovery from scratch.

We are still planning. Produce the design and a clear UI specification, not a claim
that the backend supports every screen. Target **Jetpack Compose + Material 3**.

## Product direction

- App name: **FamilyWarden**.
- Approved Android application ID: `io.github.denogio.familywarden`.
- FamilyWarden is the umbrella product. **Ulysses is an optional feature**, not the app's
  default identity or permanent main screen.
- Initial rollout: one person's own Android phone, with enforced DNS protection and
  optional Ulysses self-binding. The existing app uses Android Device Owner.
- Leave room for future family/profile management and functionality currently handled
  by Switchly. Its exact feature set has not been specified: do not invent it or show
  speculative controls as implemented. Avoid a layout that assumes one user will
  always have only self-binding features.

## Roles, capabilities, and visibility

Do not conflate these concepts:

1. **Managed device:** sees its own protection and DNS status.
2. **Captain:** may use Ulysses if configured as captain by the paired router.
3. **Administrator:** may eventually manage other profiles/devices. This is a separate
   capability; an administrator need not be captain, and captain does not imply admin.

Design navigation around available capabilities, not a permanently fixed list of features.
Only the initial managed-device/captain scope needs fully specified working screens.
Future administration can be an explicitly labeled navigation concept, not fake functionality.

**Ulysses must be opt-in under Settings → Features.**
- Off by default: no Ulysses navigation item and no Ulysses card on Overview.
- Enabling starts an eligibility/setup flow; a local toggle alone cannot make the user captain.
  Router-confirmed device/profile association is required and is a planned backend extension.
- Enabled and eligible: Ulysses appears in navigation and Overview.
- An active pact cannot be bypassed or hidden by disabling the feature. Lock the toggle,
  explain why, and keep its status accessible until the pact ends.
- Pairing and removal restrictions remain enforced during a pact. Hiding a button is
  presentation, not the security mechanism.

## Initial screens and information hierarchy

Propose an appropriate phone navigation pattern for these destinations. It must feel
complete with Ulysses absent and balanced when the feature is enabled.

### Overview

Answer: **Is this phone protected, and is anything I need to act on?**
- DNS protection: applied, setup incomplete, applying, or error.
- Resolver reachability separately from enforcement/configuration status.
- Router sync state and last verified sync; Sync now.
- Optional Ulysses card only when applicable: pact and hotspot status, next real change.
- Do not imply hotspot is restricted when Ulysses is disabled.

### DNS protection

- Always active while paired, independent of Ulysses and allowed internet hours.
- **Strict Android Private DNS: DNS-over-TLS, not a generic DoH URL.**
- Show configured provider hostname, assigned filtering policy, applied/locked state,
  and a connection test.
- Aggressive practical filtering is wanted, but exact categories/lists are not decided.
  Use clearly marked example data; do not promise all ads, HTTPS paths, VPNs, or app-level
  DoH can be blocked.
- No casual disable button, opportunistic mode, or automatic unfiltered fallback.
- Clearly explain unreachable resolver, invalid certificate, a network blocking TCP 853,
  and captive-portal limitations. Do not offer an unapproved bypass action.
- No invented query counts, browsing history, analytics, or unsupported per-domain exceptions.

### Ulysses — optional

Reuse the states you designed: no pact, bound/allowed, bound/blocked, finite/ongoing
pact, sailing-home countdown, and failed sync with cached policy retained.
Show the schedule and next actual hotspot transition when known.
For the initial scope, pact creation/amendment and release requests stay on the router.
The app may offer access to the sail page when reachable; do not invent authenticated
in-app pact administration or ask for the router password in a new app form.

### Settings and setup

- Features: Ulysses opt-in/eligibility, enabled, and locked-during-pact states.
- Pairing: scan QR, manual address, fingerprint confirmation.
- Device/profile association and capabilities, including unknown/pending/mismatch states.
- Device Owner setup/readiness, app version, and compact diagnostics.
- Controlled removal: explain requirements, confirm, and handle rejection/unreachable router.
  Do not claim removal is possible just because a cached state shows no pact.
- Onboarding: ownership readiness → pairing/key confirmation → apply and verify DNS.
  Do not promise account removal is lossless or Device Owner enrollment never needs reset.

## Status truthfulness and offline behavior

These are different facts and must not collapse into one green "Protected" badge:
- Policy received and verified.
- Policy successfully applied by Android.
- DNS endpoint reachable now.
- Router reachable for sync now.

DNS can work on mobile data while the home router is unreachable. The last verified
configuration remains cached until a successful sync; **no 24/48-hour stale-cache
escalation**. Finite pacts/releases can expire locally. A newly created router policy
cannot be enforced on the phone until received.

## Brand and accessibility

- Use your lighthouse for FamilyWarden and the existing mast/sail for Ulysses.
- Preserve the night-sea/gold palette, restrained nautical motif, and serif headlines.
- Respect the lighthouse variants you designed: adaptive launcher, monochrome themed,
  notification, and simplified small-size marks.
- Calm and respectful: chosen protection, not punishment. Red means a genuine problem,
  not merely that a self-chosen pact is holding.
- English UI text; light/dark themes; edge-to-edge insets; TalkBack semantics; accessible
  contrast; 200% font scaling; no color-only status indicators.
- Use illustrative hostnames such as `phone-id.dns.example.net`, never hard-code a
  developer's personal domain into the design or product defaults.

## Deliverables

1. A concise information architecture and navigation rationale, including extension
   to future administrator capabilities without empty tabs or a plugin framework.
2. High-fidelity light/dark mockups: ordinary user with Ulysses hidden; eligible opt-in;
   enabled captain; active pact with feature toggle locked; DNS healthy/unreachable/
   certificate error; router offline but DNS healthy; pairing/setup/removal flows.
3. Component/state specification suitable for stateless Compose UI: separate role,
   feature visibility, policy application, resolver health, router sync, and pact state.
4. One 200%-font example and a small reusable component/token inventory.
5. Explicitly label example data, future concepts, and backend-dependent actions.

Reuse the existing brand and Ulysses work. Focus on a coherent FamilyWarden product,
not adding more cards to the old Mast screen.
