# FamilyWarden LuCI design reference

Original user-supplied Claude Design exports, preserved unchanged:

- `design.html`: complete multi-board export (27 HTML boards).
- `reference.html`: standalone Overview reference.
- `fw-luci.css`: reference stylesheet, including mock LuCI chrome. Do not ship wholesale.
- `ticket-update.html`: subsequent user-supplied Overview revision separating Override
  and Internet ticket zones, with explicitly approved additive ticket behavior.
- `overview-refinements.html`: later layout/status refinement reference.
- `button-intents.html`: exact teal/slate/gold button styling and segmented minutes reference.
- `reference-updated.html` and `fw-luci-updated.css`: latest standalone HTML/CSS pair;
  verified that the CSS is embedded verbatim in the HTML. These supersede earlier
  button-style references; mock LuCI theme rules remain reference-only.
- `override-badge.html`: subsequent approved bold Reset override and dashed manual-override badge reference.

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

## Accepted Overview visual refinement

- Cards fit their own content; action zones top-align. Profiles helper text sits below
  its heading. MAC addresses stack below names when a card is narrower than 460 px.
- Ticket minutes and short expiry appear once in the status line; same-day expiry omits
  date/seconds. Cancel ticket remains a real bordered button beside the minute group.
- SVG check/ban/moon icons distinguish allowed, policy/manual blocks and scheduled blocks.
  The captain note uses the mast SVG; Pact actions have their own labeled zone and the
  restriction explanation precedes controls. Block remains permitted by backend semantics.
- Schedule days collapse into consecutive ranges; Every day/All day labels retain the
  exact underlying `00:00–23:59` range in the tooltip. Times follow days directly.
- Native LuCI buttons carry scoped intent classes: teal filled Allow, slate filled Block,
  gold Pact, teal-outline segmented +5/+10/+30 and slate-outline Cancel ticket. Minute
  controls have explicit accessible names. Reset override now uses bold text and an
  ink outline as in the subsequent badge reference; manual overrides carry a small
  dashed hand-SVG badge beside the status. Active tickets carry the same badge labeled
  "Timed override", while manual Allow/Block use "Override".
  The explanatory text beneath ticket buttons has been removed at the user's request.
- Narrow action-zone stacking follows the card's own 440 px container breakpoint.
- The supplied stale-state board is a button reference only: it does not override our
  stale-data safeguards or imply that Block is forbidden during a pact.
- User accepted this Overview checkpoint; Profiles is the next design workstream.
  CI is explicitly skipped for this checkpoint to respect the requested Playwright pause.
- Preview on localhost:18091 contains these changes. Screenshots in the repository
  precede this refinement; no new screenshots were generated after Playwright was paused.
- Syntax/structure checks and `git diff --check` pass. Earlier targeted layout/status
  checks passed, but the full 29-test run was interrupted. The subsequent intent-color
  changes have not been browser-tested: **Playwright is paused at the user's request**.

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

## Profiles, Phones, Settings and Info

All FamilyWarden pages now share `familywarden/ui.js` (lighthouse header, SVG icons,
stylesheet loader), the framed page surface and tokens from `overview.css`, and
`pages.css` for the native-form pages. LuCI's own fields, dialogs and Save & Apply stay.

- **Profiles:** profile chips, live status card with icons/override badges and intent
  buttons, explicit live-vs-staged note, pact lock panel, DNS divider inside the one
  native section, styled day checkboxes, stacked schedule rows on narrow screens.
- **Phones:** pending requests as cards with large grouped fingerprints and Compare and
  approve / Reject; approved phones as a table that stacks on mobile; separate,
  explicit dialogs for "Allow removal for 1 hour" and "Remove enrollment" (which does
  not uninstall or erase the phone's cached protection). Last policy fetch is labelled
  as a fetch, not applied state.
- **Settings:** General, Phone pairing (for all phones, not only Ulysses), DNS · AdGuard
  Home, Ulysses. Login/sync are marked "applies immediately"; plain-HTTP AdGuard URLs
  get an honest warning; the router key is grouped (it is the 16-hex usign id, not a
  SHA-256 as drawn in the mockup).
- **Info:** cache mismatch as a notice with Reload, versions as a key/value list.

Validation: syntax checks and `test/luci-ui.test.cjs` pass; e2e tests updated for the
new fingerprint text and framing isolation but **not run** (Playwright paused by the user).
Reviewed manually in the disposable preview only.
