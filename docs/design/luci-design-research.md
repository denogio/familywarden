# LuCI visual alignment — research and handoff

Baseline: `feature/app-control`, inherited from v0.9.1. Design work only.
No live router/AGH changes, browser runtime verification or new UI implementation.
The copy-ready handoff is [luci-design-prompt.md](luci-design-prompt.md).

## Direction

Reuse the Android lighthouse, night-sea/gold palette, restrained serif headlines,
rounded surfaces and calm language. Translate them to a multi-profile router admin
interface inside LuCI, not a stretched Android screen or standalone SaaS dashboard.
Overview is the first implementation target. Other pages need a coherent design
family without rebuilding LuCI navigation, form validation or the UCI apply lifecycle.

## What was inspected

- LuCI `overview.js`, `devices.js`, profile editor, settings, info, navigation JSON
  and shared `familywarden/ulysses.js`.
- Router reply construction for `dns_status`; current device-list UI fields.
- Original Android design prompt, archived UI specification and its safety review.
- Implemented Android `ui/theme/Color.kt`, `Type.kt` and committed Overview screenshot.
- Official LuCI JS documentation through Context7, particularly `view.addFooter`,
  `handleSaveApply`, and `ui.showModal`.

The archived Android specification contains historical backend gaps and unsafe draft
headlines corrected by `familywarden-app/REVIEW.md`. It is a visual reference, not
an authoritative description of today's implementation.

## Current pages and constraints

| Page | Current behavior | Design opportunity |
|---|---|---|
| Overview | Profile cards; allowed/blocked state, policy, MAC devices, schedules, timed overrides, captain pact; refresh every 10 seconds | Branded header, clearer hierarchy, calmer status tones, responsive cards, explicit unknown/stale states |
| Profiles | Profile selector; native UCI fields; inline day/start/stop schedules; strict DNS protections/services | Clear sections, less dense controls, preserve inline schedule editing and native save/apply |
| Phones | Pending/approved tables; fingerprint comparison, profile/admin assignment, removal approval; refresh every 5 seconds | Strong pending approval hierarchy, readable fingerprints, responsive rows and explicit trust/removal language |
| Settings | Global policy, captain/cooling-off, QR pairing, AGH credentials and sync | Separate general settings, pairing and DNS setup visually; pairing is not inherently Ulysses-only |
| Info | Running JS, installed frontend/backend versions; stale browser-cache warning | Small consistent diagnostic panel, not invented telemetry |

Current CSS is injected separately in Overview and Profiles, with many inline styles
elsewhere. A later implementation can use a shared scoped stylesheet/component helper
where genuinely reused. No new frontend framework or theme fork is needed.

## Facts the design must preserve

1. Overview's `Online`/`Offline` labels count allowed/blocked profiles, not connectivity.
   Replace the wording with `Internet allowed` / `Internet blocked`. A DHCP lease is
   also not proof a device is online now.
2. Ordinary scheduled blocking is an expected state, not a red system failure.
3. `traffic` is referenced by the frontend but has no implemented backend method.
   Do not mock working bandwidth totals, daily charts or usage analytics.
4. Existing internet tickets are whole-profile temporary network access, not app minutes.
   App quotas, app approvals, requests, usage summaries and parent-role management are
   future app-control work. Omit them from current navigation and working mockups.
5. Router AGH sync/acknowledgement is not Android application or resolver-test evidence.
   Current phone fields provide identity/binding, fingerprint, last fetch, frozen state,
   administrator flag and removal-until—not phone-side applied DNS or live reachability.
6. Request/fetch timestamps should be labeled `Last policy fetch` / `Last request`, not
   `Phone online`, `Protection verified` or an applied-policy acknowledgement.
7. An active pact locks the captain's profile except permitted amendment semantics.
   Captain phone bindings remain frozen even during the amendment window. DNS controls
   remain editable with an honest warning about tightening now vs deferred loosening.
   Do not imply the UI's locking is the security boundary or guarantee cumulative
   tightening beyond the tested backend behavior.
8. Removing an enrollment is not uninstalling the app. The phone retains its cached
   policy. `Allow removal for 1 hour` is a separate operation, still blocked by a pact.
9. Administrator and captain are independent. The current administrator flag governs
   self-removal; it is not proof that future parent management is implemented.
10. Global/profile DNS enable controls exist in administration, unlike the managed-phone
    screen's no-bypass UI. A visual refresh must not delete these or add new authority.
11. UCI forms have staged Save / Save & Apply / Reset behavior. Enrollment and quick
    actions are immediate RPC operations. Distinguish these visually and in copy.
12. Periodic refresh must not clear focused inputs, open dialogs or pending changes.

## Brand tokens from implemented Android

| Role | Reference values |
|---|---|
| Night | `#1B2A3A` |
| Deep night | `#070B10` |
| Dark page | `#0A1018` |
| Dark surface | `#121C27` |
| Dark line | `#2A3B4D` |
| Mist text / muted | `#D8E1EA` / `#B4C4D3` |
| Gold / bright | `#C9A227` / `#E0BD4A` |
| Light page / surface | `#EEF2F6` / `#FFFFFF` |
| Light ink / muted | `#0F1B27` / `#4A5D70` |
| Light line | `#CDD6DF` |
| Success light / dark | `#1F6A64` / `#7CC7BD` |
| Fault light / dark | `#8F2F0C` / `#FFB38A` |

These are brand references, not a requirement to override every theme color. Validate
contrast in each context; gold on white is not a safe default for small text. Use a
system serif for occasional headings, system sans for controls/body and monospace for
fingerprints/addresses. No external fonts or CDN assets on a router admin interface.

## Handoff / review gates

- Show a representative LuCI shell but change only FamilyWarden content. Theme-specific
  selectors/color variables are not universal APIs; test real installed themes later.
- Desktop plus narrow screens, light/dark, keyboard/focus and 200% zoom.
- Fingerprints must remain fully readable/copyable; long names/hostnames must wrap.
- No global CSS affecting other LuCI pages. Mockups/prototype are references, not runtime code.
- Review generated design against the facts above before implementing Overview.
- Request sanitized current LuCI screenshots if pixel-level comparison is needed;
  this review is source-based and must not be presented as fresh browser validation.

## External references

- https://openwrt.github.io/luci/jsapi/LuCI.view.html
- https://openwrt.github.io/luci/jsapi/LuCI.form.Map.html
- https://openwrt.github.io/luci/jsapi/LuCI.ui.html
- https://openwrt.github.io/luci/jsapi/luci.js.html
- https://openwrt.github.io/luci/jsapi/ui.js.html

The queried generated source confirms that page footers depend on available view
handlers, Save & Apply uses the normal save/apply flow, and one `ui.showModal` instance
is active at a time. The theme search also returned legacy theme tutorials; those
are not a basis for new view implementation.
