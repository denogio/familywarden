# Control interface prototype — review

Date: 2026-10-25. Reviewed `~/Downloads/familywarden-control-prototype.html`
(self-contained, `.fwa` namespace, no framework/CDN) against
`~/Downloads/fw-app.css` (light + dark tokens, layout, components,
responsive breakpoints) and `docs/design/control-interface-prompt.md`.

Method: 20 Playwright renders covering every view × role
(Robin admin, Kim parent, Jordan captain) × data state
(normal, loading, stale, partial, empty) × host (neutral, LuCI, phone width)
× theme, plus the approve dialog. Screenshots: `/tmp/fw-prototype-review/`.

Note: `FamilyWarden (4).html` is a Figma-style board whose visual content is
base64-embedded; its text sections (assessment/IA, design system, capability
and API gap list, phased plan, first review) match the prototype structure,
but pixel-level comparison against it is not practical. The prototype plus
the design prompt are the effective reference.

## Verdict

Strong, implementable reference. The information architecture, role scoping,
state honesty, and host-adapter story all hold up in rendered form. It is
close enough to production intent that the remaining work is decision-level,
not redesign-level.

## What works well

- **Information architecture.** Rail: Family (+children), Requests (badge),
  Devices, then HOUSEHOLD: Ulysses, Administration. One page per concern, no
  hidden features.
- **Role scoping is visible and correct.**
  - Kim (parent): no Administration item; the DNS "needs attention" row is
    absent from her Needs-you list.
  - Jordan (captain): Ulysses only; his own family view is not shown.
  - Robin (admin): everything, including the DNS row with
    "Open administration".
- **"Needs you" inbox.** Mixes requests, new-app decisions, device apply
  failures, and DNS health into one triage queue with one appropriate action
  per row. This is the right shape for the overview.
- **Honest device states.** Per-device delivery pipeline
  (Saved → Available → Fetched 17:38 → Applied 17:38), "Offline · state
  unknown" with "Not fetched · offline since 15:40", "Failed to apply" with
  reason. No invented liveness.
- **Data states.** Loading = skeleton; stale = banner + every action replaced
  by "Unavailable while offline"; partial = info banner scoped to the DNS
  timeout only; empty = "No children yet" with CTA (and the rail correctly
  hides child names).
- **Ulysses.** "At the mast" voyage card (port call, cooling-off, delivered
  to phone), frozen siren list, "Still in your hands" list with
  "Adjust at port" disabled outside the port window. Not-bound state:
  "The sea is calm" with a 3-step explanation and captain tabs
  (Your voyage / Jordan / Kim). Consistent with the pact model.
- **Child detail.** Tabs Today / History / Rules / Devices; screen time vs
  Internet at home kept strictly separate, each labeled with its enforcement
  source ("On managed devices" / "Enforced by the router"); internet ticket
  chips +15/+30/+60; "Block until reset".
- **Approve dialog.** Presets (15 / 30·asked / +1 h / +2 h / Custom) plus
  custom 5–240 min in 5-min steps, and a full impact list: shared budget
  scope, 2-hour expiry, home-internet effect, "does not reopen an app whose
  own limit is used up", saved-then-fetched delivery semantics. This matches
  several agreed grant semantics, but the shared-grant scope and expiry wording
  require reconciliation with the implemented contract (see the update below).
- **Administration.** Health cards (core version + policy revision, router
  integration, DNS service with "Needs attention", enrollment/security with
  pairing state) and People-and-access with role summaries and "Edit access".
- **LuCI host.** Router chrome around the app area with "signed in through
  the router's session" — exactly the A20 thin-adapter story (no iframe, no
  new password/port).
- **Phone width.** Rail collapses to a horizontal strip with the pending
  badge preserved.

## Nits and smaller issues

1. **"Approve…" truncation.** Row buttons render as "Approve…"
   (Requests, LuCI light). Either shorten to "Review" or widen the button;
   a truncated verb in the primary CTA reads as a bug.
2. **Loading skeleton doesn't cover the rail.** Child names (Alex/Sam) are
   visible during Loading, which a real first load would not know yet.
   Skeletonize the rail children too (or accept it as a prototype
   convenience and note it).
3. **Stale state layout shift.** Action buttons are replaced by plain
   "Unavailable while offline" text; the row's right edge moves. Prefer
   disabling the buttons and keeping labels, or reserving the space, so the
   layout is stable between states.
4. **Badge semantics.** The Requests badge counts time + new-app requests
   (3) but the overview's "Needs you" also contains device-failure and DNS
   rows that are not requests. Consider whether the badge should track
   "needs you" total instead, or stay request-only (then the copy should
   say "3 requests").
5. **DNS health duplicated.** The DNS failure appears in Needs you, in
   Devices (Sam's tablet "Failed to apply"), and in Administration health.
   That is acceptable cross-referencing, but the Administration card should
   be the single source of the remediation actions; the other two should
   link there instead of repeating "Change login…/Sync now" affordances.
6. **Phone-width tap targets.** The collapsed strip is tight; verify ≥44 px
   targets for the strip items and the "Add a child" button at 360 px width.
7. **Copy (Danish later).** All copy is English. The production UI needs
   EN/DA string resources from the start (project rule), and several long
   strings ("Adjust the course at the port call", "Loosening DNS: saved now,
   applied when the voyage ends") need Danish length headroom in the layout.

## Decisions still open (for the user)

1. **API contract.** The prototype's mock `core` object is a proposal. Do we
   freeze these request/response shapes as the core RPC surface, or revise
   first? (This is the P1 contract work in `plans/app-control-todo.md`.)
2. **LuCI navigation coexistence.** The prototype keeps the FamilyWarden
   rail inside the LuCI chrome. Real LuCI also has its own left menu; decide
   whether the adapter drops the in-app rail and uses LuCI's menu, or
   renders the app full-width under a LuCI header.
3. **Android alignment.** The separate Android board `FamilyWarden (5).html`
   proposes Screen time / Controls / Requests, with My phone and Family modes.
   Use that as the Android presentation reference, not the web rail. Preserve
   the later user-approved icon-only navigation and page headings.
4. **Scope of this phase.** Confirm that this control UI ships as the shared
   `.fwa` presentation layer with host adapters (neutral standalone first,
   LuCI second), matching the design prompt — and that no business rules
   move into it.
5. **Fictional data fixture.** The prototype's dataset (Alex 11, Sam 14,
   Pixel 8a enrolling, DNS 401) is good; should it become the canonical
   fixture for e2e screenshots and README?

## Next steps

- Resolve the five decisions above (user).
- Freeze API shapes → add the state/contract tests in core.
- Port `.fwa` + `fw-app.css` into the repo (`luci-app-familywarden` or a
  new shared asset location), keeping the one-presentation-layer rule.
- Wire host adapters: standalone first (served by the core), LuCI second
  (existing session/ACL/rpcd, no iframe).
- Re-run the Playwright render set against the production asset to diff.

## Updated files review

Reviewed `familywarden-control-prototype (1).html` and the ten sections of
`FamilyWarden (5).html`. The latter is an **Android design board**, not a
replacement revision of the web board `(4)`. Keep the references separate.

### Web prototype delta

Three source changes compared with the previous prototype:

- Explicitly hide custom-amount fields and validation errors when `[hidden]`.
- Add approved-extra explanation to the screen-time meter tooltip.
- Clarify that app-specific approved extra does not consume the daily budget
  and remains usable after that budget runs out.

The existing 20-scenario Playwright navigation flow completed on the updated
file without reported JavaScript errors. This repeat used text inspection,
not new screenshot inspection; it is not a pixel-level regression test.

### Android presentation direction

- Parent: Screen time / Controls / Requests. Screen time contains usage,
  devices, limits and schedules; Controls contains new-app settings, home
  internet, DNS and device application state; Requests contains scoped
  pending decisions and history across children the parent may manage.
- Managed adult + parent: separate My phone and Family modes. A child selector
  belongs to Family; own limits never appear as child-management controls.
- Child: calm Today states, an explicit request flow, read-only history and
  settings. Missing usage is unavailable, never fabricated as zero.
- Distinguish saved policy, fetched policy and confirmed application. A fetch
  alone must not be labelled delivered/applied. Show timestamps for stale data.
- Ulysses: course → voyage → confirmation; self-service only, with no UI
  escape from pact enforcement. This is a future core capability, not merely
  a restyled activation button.
- Retain the user's newer **icons-only bottom navigation**, localized TalkBack
  names, pending badge and visible page headings. Board labels are superseded.
- Keep 48 dp touch targets, 200% font support and complete EN/DA resources.

### Contract differences — do not implement implicitly

The board is a visual reference, not authority to change product rules:

1. **Grant lifetime:** the board proposes `approvedAt + max(2h, minutes)`.
   The current core uses fixed `protocol.GrantLifetime` (two hours).
   The web's initial 30-minute example does not test the >120-minute case.
2. **Grant scope:** the board says grants are shared across a child's phones.
   Current request grants are bound to the requesting device and key;
   shared daily usage budgets do not make grants shared. Home-internet
   wording must also distinguish delivery target from profile-wide router
   ticket enforcement.
3. **Parent authorization:** “every parent manages every child” conflicts
   with existing explicit approval scopes. Preserve core authorization,
   including self/pact exclusions; never infer permission from visible UI.
4. **Pact scope:** every adult independently setting sail requires core
   self-service and ownership migration. Do not pretend the current single
   captain seam already supports multiple independent voyages.
5. **Cooling-off:** “24 hours” is example copy. Display the configured/frozen
   waiting period, not a hardcoded value. The board's recovery/lyrics wording
   does not authorize changing the existing sail-page recovery flow.
6. **Retention and availability:** year/all-time history, website history,
   adult pools, categories and other future controls need real data/contracts.
   Current router usage retention is 30 days. Do not invent old samples or
   ship clickable mock functionality. Conversely, board “Later” tags are
   historical annotations: some requests/usage features now exist.

Next implementation slice: restructure existing Android parent presentation
around the three concerns while preserving current authorization, grant and
pact behavior. Add unavailable states where backend support is absent; do not
implement new business rules as part of the visual port.

### Review resource budget

Large design-board images caused oversized conversation requests (HTTP 413).
Use extracted text and DOM assertions by default. If visual verification is
necessary, inspect one small crop at a time rather than full multi-screen
boards. No production deployment was performed for this review.
