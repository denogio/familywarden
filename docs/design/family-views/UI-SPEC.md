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
- Screen time renders signed summaries for assigned profiles, with report dates,
  incomplete-data notes and an explicit last-sync/not-live disclaimer. Missing
  reports are text, not an invented zero-minute count.
- Requests contains time and new-app decisions only. Its badge counts scoped
  pending time requests (excluding known-expired ones) plus pending new apps.
  The Family button carries the same badge while viewing My phone.
- Controls honestly explains that rule, schedule, home-internet and DNS editing
  still happens on the router. Parent-only connection settings remain here;
  no unsupported Lock, Bonus or policy-edit action is made clickable.
- Danish family headings are **Skærmtid / Kontrol / Godkendelser**; the child's
  own request page retains its existing wording. Every destination has a visible
  heading and localized accessible icon name.

This is a presentation split, not a grant, authorization, pact or transport change.

## Explicitly deferred

- Off-home parent notifications (P9); LAN-only notifications exist while syncing.
- Child switching and full parent controls (rules, lock now). Screen time currently
  lists all assigned profiles; the design's child selector and policy editors are
  not implemented. New-app approvals stay on Requests; usage is on Screen time.
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
