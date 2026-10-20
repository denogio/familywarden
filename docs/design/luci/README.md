# FamilyWarden LuCI design reference

Original user-supplied Claude Design exports, preserved unchanged:

- `design.html`: complete multi-board export (27 HTML boards).
- `reference.html`: standalone Overview reference.
- `fw-luci.css`: reference stylesheet, including mock LuCI chrome. Do not ship wholesale.
- `ticket-update.html`: subsequent user-supplied Overview revision separating Override
  and Internet ticket zones, with explicitly approved additive ticket behavior.

Brief/research: [prompt](../luci-design-prompt.md), [research](../luci-design-research.md).

## Overview implementation checkpoint

Overview is implemented in the existing LuCI view, not as a theme:

- `overview.js` renders real effective profile status, devices, allow windows, tickets
  and captain pact information using existing RPCs and shared pact controls.
- `familywarden/overview.css` is explicitly packaged and scoped to `#fw-dashboard`.
- Native LuCI navigation, dialogs and buttons remain. No mock `.lx-*` chrome ships.
- A blue page surface now encloses Overview and its native tabs, with a subtle border
  and rounded corners. It is selected only while `#fw-dashboard` exists, using CSS
  `:has()` (modern browser required); no persistent global theme/class change.
- Bootstrap's installed `data-darkmode` attribute was checked against its actual CSS
  and used for the branded dark palette. Other LuCI themes still require verification.
- The unused Save/Apply/Reset footer is omitted on this immediate-action page only.
  Native configuration forms on Profiles and Settings are unchanged.
- Block and Reset remain available where the pact/backend semantics permit;
  Allow/tickets are not offered to a bound captain.
- Approved ticket update: Override and Internet ticket are separate action zones.
  Active tickets can be extended with +5/+10/+30 minutes and cancelled independently
  (returning to profile policy). Manual overrides retain Reset override.
- The router API now adds minutes to an unexpired timed allow; otherwise a ticket starts
  from router time now. A dedicated ticket-operation lock prevents concurrent additions
  from losing time. Zero/negative/non-integer durations are refused. Pact refusal remains.
  Hints describe semantics rather than promising a browser-clock expiry estimate.
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
- Blue-frame follow-up: targeted responsive/theme/isolation browser test passes at
  320–1440 px and 200% CSS zoom. Other pages retain their original background even
  with the Overview stylesheet loaded. No enforcement code changed in this follow-up.
- Ticket follow-up: `scripts/test-integration.sh` **192 checks pass**, including active
  extension, concurrent additions, invalid input, expired/permanent-allow/manual-block
  starting points and the existing pact refusal. Regression tests failed before the fix.
- `scripts/test-e2e.sh /tmp/fw-ticket-build`: **25 passed** using newly built packages,
  including real ticket extension and cancellation via the Overview.
- No Android emulator suite: Android was not changed.
- No live router, AdGuard Home, phone or production pact was modified.

Screenshots under `docs/screenshots/luci-overview-*` are disposable fixtures, not
production state. The CSS-zoom check is not a full manual browser/accessibility audit.
