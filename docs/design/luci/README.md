# FamilyWarden LuCI design reference

Original user-supplied Claude Design exports, preserved unchanged:

- `design.html`: complete multi-board export (27 HTML boards).
- `reference.html`: standalone Overview reference.
- `fw-luci.css`: reference stylesheet, including mock LuCI chrome. Do not ship wholesale.

Brief/research: [prompt](../luci-design-prompt.md), [research](../luci-design-research.md).

## Overview implementation checkpoint

Overview is implemented in the existing LuCI view, not as a theme:

- `overview.js` renders real effective profile status, devices, allow windows, tickets
  and captain pact information using existing RPCs and shared pact controls.
- `familywarden/overview.css` is explicitly packaged and scoped to `#fw-dashboard`.
- Native LuCI navigation, dialogs and buttons remain. No mock `.lx-*` chrome ships.
- Bootstrap's installed `data-darkmode` attribute was checked against its actual CSS
  and used for the branded dark palette. Other LuCI themes still require verification.
- The unused Save/Apply/Reset footer is omitted on this immediate-action page only.
  Native configuration forms on Profiles and Settings are unchanged.
- Block and Reset remain available where the existing pact/backend semantics permit;
  Allow/tickets are not offered to a bound captain. Ticket buttons retain their existing
  semantics; they do not extend an active ticket as the reference's "Add to ticket" suggests.
- Allowed/blocked counts are not called online/offline. Unknown status has no quick actions.
- Missing/malformed list envelopes produce an error; valid empty lists remove old cards.
  Failed refreshes retain explicitly stale previous results and disable quick actions.
- Refresh preserves keyboard focus and pauses while an action/dialog is active.
- No unsupported phone-side enforcement claims, traffic charts, pending app requests,
  new backend endpoints or fake router clock were added. Schedules display effective
  allow windows; browser-clock next-transition estimates are deliberately not carried over.
- The shared pact note now labels the timestamp "Last policy fetch", not "Mast synced".

The pending-phone notice in the mockup would need additional API wiring and is not
included in this first Overview implementation. Full review/implementation of the
other pages remains pending, including the measured Profiles overflow at 390 px.

## Evidence

- Built both OpenWrt packages with `scripts/build-apk-docker.sh`.
- `scripts/test-e2e.sh /tmp/fw-overview-build`: **24 passed** in a fresh disposable
  OpenWrt/LuCI container using the newly built packages (17 existing + 7 Overview tests).
- `scripts/test-syntax.sh`: passed; existing platform-specific shellcheck warnings remain.
- New browser tests cover real block/reset, pact-safe controls, unknown status, initial
  and refresh failures, genuine empty state, focus retention, light/dark, 320–1440 px,
  200% CSS zoom/reflow and stylesheet coexistence with the profile editor.
- Screenshots inspected in actual disposable LuCI: desktop light/dark and 390 px dark.
- No Android emulator suite: Android and backend enforcement were not changed.
- No live router, AdGuard Home, phone or production pact was modified.

Screenshots under `docs/screenshots/luci-overview-*` are disposable fixtures, not
production state. The CSS-zoom check is not a full manual browser/accessibility audit.
