# Family views: first Android implementation slice

The unmodified `design.html` remains the visual reference. This document maps the
2026-10-24 child/adult implementation to actual available data. It is **not** a
claim that the complete role design or P10 has shipped.

## Implemented

- App-controlled phones open **Today**: night-sea hero in both themes, daily
  remaining time, five-minute warning state, time's-up, downtime and unknown usage.
- App rows show the signed daily limits (including today's bonus), counted minutes,
  configured blocks and always-allowed apps. Essential system packages are omitted
  from restrictive rows when known to the collector.
- Time's-up and downtime focus on the calm status and still-usable apps. This is
  ordinary app UI, never an Android lock screen or a replacement for its enterprise
  suspension dialog.
- Non-captain app-controlled phones use **Today / History / Settings**. DNS details
  and the resolver test remain available in Settings. Captains retain **Today / DNS /
  Ulysses / Settings** when Ulysses is enabled or forced by a pact.
- **History** shows up to 30 days of locally recorded total screen time, with an
  explicit incomplete-coverage disclaimer. No web/domain history, cross-phone
  totals, per-day app drill-down or usage upload is implied.
- The captain's Today view also shows existing pact/hotspot status; it does not
  invent a discretionary pool or categories.
- Pairing, fingerprint approval, address repair, signed removal checks, diagnostics,
  DNS tests and the Ulysses sail flow remain in the existing components.
- Complete EN/DA resources and plural resources. **User-approved navigation change
  (2026-10-25):** bottom navigation uses icons only at every font scale. Each icon
  retains its localized accessible name, native selected state/indicator and touch
  target; the visible top heading identifies the selected page. App-controlled
  Today always says Today rather than the brand name. Setup retains the brand.
  This supersedes the export's labeled tabs and the former large-text scrollable
  tab fallback. Body content still scrolls vertically.

## Data and authority

`AppControlService` publishes one immutable, in-process `AppUsageReport` after its
existing sampling/enforcement pass. The UI never queries usage events or writes the
usage journal. There is still exactly one collector. `MainActivity` maps signed app
rules plus that report through the pure `screenTimeState` mapper.

A numeric current count requires matching policy revision, router timezone, local
calendar day, reliable collection, current usage permission and an observation no
older than two normal service intervals. Missing data is **— / Not counted**, never
an invented zero or a full budget. A zero is valid only after a reliable sample.
This freshness bound is for **display only**: saved signed policy does not expire,
and router outages do not lift restrictions. Counts are sampled, not an extrapolated
second-by-second countdown or a perpetual enforcement attestation.

Downtime comes from the trusted weekly policy independently of usage access.
Its next change skips overlapping boundaries; midnight/reset and all schedule times
use the router timezone. A new budget does not imply that downtime has ended.

Navigation uses the signed captain flag, the presence of app rules and the signed
`approves` scope. **Requests** (2026-10-25): children ask for app or screen time from
Today/Requests and see pending/approved/declined/expired status and remaining extra
minutes; phones with `approves` scope list pending requests and approve fewer minutes or
decline. "Use this phone as a parent" pairs an ordinary phone without Device Owner.
Local mode never grants authority: the router rechecks scope.

### Parent presentation split (2026-10-25)

The updated Android board `FamilyWarden (5).html` is the presentation reference;
its labeled tabs remain superseded by the user's icon-only navigation decision.

- Parent-only phones open **Screen time**, with **Controls / Requests** alongside it.
  Requests notifications open the Requests page instead.
- A managed phone with signed `approves` scope has a **Family / My phone** switch.
  Own pages and family pages retain separate selections. Family never shows own
  request buttons or pact controls; My phone never lists children's approvals.
  Being an administrator or captain alone does not enable Family mode.
- Screen time renders signed summaries for the selected assigned profile, with report dates,
  incomplete-data notes and an explicit last-sync/not-live disclaimer. Missing
  reports are text, not an invented zero-minute count.
- The child selector on Screen time and Controls resolves the saved profile ID against
  the latest signed `approves` scope on every render. A removed selection falls back
  to the first currently assigned profile (or none), never stale sibling data. Selecting
  another child resets that page's scroll. Selection survives page/mode changes;
  new phone enrollment resets it. Native radio-group sheet options grow with text
  and have at least 48 dp touch targets.
- Requests remains the combined inbox for **all** assigned children, independent of
  the child selector. It contains time and new-app decisions only. Its badge counts scoped
  pending time requests (excluding known-expired ones) plus pending new apps.
  The Family button carries the same badge while viewing My phone.
- Controls honestly explains that rule, schedule, home-internet and DNS editing
  still happens on the router. Parent-only connection settings remain here;
  no unsupported Lock, Bonus or policy-edit action is made clickable.
- Danish family headings are **Skærmtid / Kontrol / Godkendelser**; the child's
  own request page retains its existing wording. Every destination has a visible
  heading and localized accessible icon name.

This is a presentation split, not a grant, authorization, pact or transport change.

### Child Today and Ask flow aligned with `FamilyWarden (5).html` (2026-10-28)

Visual port of the child boards; no policy, authorization or enforcement change.

- Today: gold-tinted hourglass/moon badge in the night card, compact durations
  ("1 h 22 min of 2 h used"), the next downtime change in the hero line, router-local
  weekday and time under the visible **Today** heading, a plain rounded meter.
- Rows: decorative letter tiles (stable colour per package, two letters where first
  letters collide) and a right-aligned "12 min of 30 min"; **Always OK** as a tile grid.
- Time's up and Downtime keep what still works inside the night card; Time's up uses
  the gold primary Ask button and "A parent can say yes from their phone."
- Missing usage access is a warning card with the existing "Allow usage access" action.
- Navigation icons: sun (Today), bars (History), sliders (Settings), still icon-only.
- The child's own Requests tab is gone (child tabs Today · History · Settings; adult
  Today · DNS · Ulysses · Settings). Today shows **Your request** with a status chip
  (Waiting / Approved / Not this time / Closed), the retry card for an uncertain
  delivery and apps waiting for a parent. **Ask for more time** opens two full-screen
  steps: what for (all apps or one app, with time left when known), then how much
  (15 min, 30 min, 1 h, 2 h, 4 h, other amount) with an optional note and Send.
- The optional note is sent in the existing signed request (`note`, router rule:
  printable, at most 140 characters); parents already receive and display it.

Deliberate deviations from the board, with reasons:

- Icon-only navigation and the **Today** heading (user decision of 2026-10-25) stay.
- Downtime keeps **Ask for more time**: an approved grant does work during downtime in
  the enforcement model, so hiding the button would remove a working path.
- **Other amount** (1–240 min) is offered next to the presets so short asks remain
  possible; the native request test uses one minute.
- Parent names in "Robin or Kim can say yes" are not shown: the signed policy carries
  parent *device* names, not people, so the copy stays generic.
- The near-limit line does not promise a pause clock time: that would extrapolate a
  countdown from sampled usage. "Recorded on this phone" stays below the hero.
- Tiles are letter tiles as on the boards, not launcher icons.

Verification: JVM tests, lint and debug build; `test-screen-time-ui.sh` and
`test-requests-ui.sh` (light, dark, Danish at 200 %, icon-only navigation checks and
the two-step ask flow) on a disposable Android 15 emulator; `test-requests-emulator.sh`
31/31 with the real flow against a disposable router; `test-mast-emulator.sh` 98/98.
`scripts/capture-android-previews.sh` renders named previews for board comparison.

## Explicitly deferred

- Off-home parent notifications (P9); LAN-only notifications exist while syncing.
- Full parent controls (rule editors, lock now and detailed device application state).
  The child selector is a presentation filter, not a new policy-editing or device-
  inventory API. New-app approvals stay on Requests; usage is on Screen time.
- Adult categories and daily discretionary pool (P11).
- Bounded shared-budget allocations and domain history. Authenticated usage uploads
  and shared usage offsets exist; cached offsets do not bound offline overspend.
- Lock now, Tonight and School time.

No disabled placeholder for an unsupported action is presented as usable. Existing
decisions remain: request grants expire two hours after approval; the pool's day
boundary is router-local midnight, not the export's proposed 04:00/delivery assumptions.

## Verification

Parent split checkpoint (2026-10-25): 197 JVM tests, lint, debug APK and release
Kotlin compilation pass on an immutable source snapshot. Six offline navigation
validator tests pass and run in Android CI. The grouped Android 15 preview checks
pass for parent requests/controls/usage, managed My phone ↔ Family, child/adult
navigation, light/dark themes and Danish at 200% font. The final pending-enrollment
fixture separately confirms that a parent awaiting approval sees neither a false
empty scope nor cached child usage. Test emulators were removed after completion;
no physical device, production router or enforcement rule changed. These are UI
checks, not a repeated full native request/enforcement lifecycle run.

Child selector checkpoint (2026-10-25): 199 JVM tests, lint/debug APK/release Kotlin
compilation and syntax checks pass on an immutable snapshot. Android 15 preview
interaction checks confirm sibling usage isolation, the combined inbox, selection
across pages/modes and immediate fallback after scope revocation. EN and Danish
200%-font radio options expose selected state and at least 48 dp targets. A separate
Danish replay verifies the exact `Godkend` action (not the `Godkendelser` heading).
The APK used for UI replay was checked against all tracked Android build inputs;
only the test harness changed after compilation. No large board screenshots were
loaded into the conversation. Test emulators were removed; no deployment occurred.

Earlier checkpoints:

- 139 JVM tests pass (8 new mapper tests: unknown/stale/revoked counts, bonus/day
  handling, overlaps, overnight/DST boundaries, essential exceptions and history bounds).
- `testDebugUnitTest lintDebug assembleDebug compileReleaseKotlin` pass in the Android
  SDK container with one worker and a 3 GiB hard limit. No new dependency.
- `scripts/test-screen-time-ui.sh emulator-5558` passes using Compose's existing
  **debug-only PreviewActivity** and fictional fixtures on Android 15. Covers light,
  dark, near limit, time's-up, downtime, unavailable usage, adult pact, History/Settings
  navigation and Danish at 200% text with vertical scrolling.
- Icon-only navigation checks use UIAutomator on the debug previews: no visible
  navigation text, localized accessible names, selected state, touch targets of at
  least 48 dp and a visible page heading. Child, adult and parent navigation are
  covered in EN and Danish at 200% text, with light/dark fixtures.
- The focused render check is not a new end-to-end enforcement test. The previous
  signed-policy P7b emulator run passed; it was not repeated for every UI edit.
- The initial Android 17 rendering attempt exceeded its 5 GiB container limit. A fresh
  Android 15 AVD completed the checks at about 3.1 GiB inside a 4.5 GiB hard cap. Both
  test emulators were stopped afterwards; no physical device or production router changed.
