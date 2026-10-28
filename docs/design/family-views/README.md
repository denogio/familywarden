# Family views design (child, adult, parent)

`design.html` is the Claude Design export from 2026-10-24, archived **unchanged**
(sha256 prefix `94a3b06b2483a2eb`). It was made from
[`../role-app-design-prompt.md`](../role-app-design-prompt.md) and
[`../role-app-brief.md`](../role-app-brief.md). Open it in a browser; it unpacks itself
offline (embedded Noto fonts, OFL licence kept in the file).

Implementation mapping and verification: [`UI-SPEC.md`](UI-SPEC.md). The first
child/adult UI slice, role-separated family navigation, native requests/approvals,
and signed usage summaries are implemented. The pool and complete prototype
editing/history flows are not. The latest visual pass uses the separate
2026-10-25 export `FamilyWarden (5).html` (ten boards, adding adult Ulysses);
`design.html` above remains the original nine-board archive.

## Boards

1. Roles and navigation · Family Link mapping
2. Component and state specification (stateless Compose)
3. Child · Today states (allowed, near limit, time's up, downtime, paused by a parent,
   usage access missing) · light, dark, 200 % font
4. Child · Ask for more time (total or one app; waiting, approved, denied, expired)
5. Child · notifications, paused app, new app, history, settings
6. Adult · Today, daily pool under a pact, My phone ⇄ Family
7. Parent · Screen time, Bonus time sheet, stale/offline/never-synced phones
8. Parent · Time limits (all days / weekdays + weekend), app limits, Downtime
9. Parent · Requests, Controls, notifications

Tabs: child **Today · History · Settings**; adult **Today · DNS · Ulysses · Settings**;
parent **Screen time · Controls · Requests**. A phone that is both adult and parent has
two modes, **My phone** and **Family**, each with its own tab bar.

## Design assumptions and how they relate to decisions

| Assumption in the design | Decision so far | State |
|---|---|---|
| The first parent to answer decides; the other sees who | First-answer-wins is implemented; attribution is available where signed | Follow current contract, not a speculative second answer |
| A grant's 2 hours start on **delivery** to the phone | D3: valid **2 hours after approval** | Archived-board deviation; implementation follows D3 |
| The adult pool refills at **04:00** | D8: **router-local midnight**; A17 pool adjustable | Archived-board deviation; pool itself remains future work |

## "Later" tags vs. current state

The boards mark total daily limit, Unlimited (always-allowed) apps, Bonus and near-limit
warnings as later. These were built in P7b after the design round (router `3def6df`,
Android `6b99ca5`, LuCI `f4d231f`) and are being verified. Usage upload, requests/approvals, parent role and notifications, and shared-budget
offset enforcement now exist; cached offsets do not bound offline overspend. See
`UI-SPEC.md` for actual presentation and [API readiness](../../android-api-readiness.md)
for native integration and permission boundaries. A signed retained-day history client
is prepared, but the complete parent history screens are not implemented.
Still later in the native Android presentation: the full parent editing/history
flows and adult pool. School time and website history remain unavailable.

All people, phones and apps on the boards are example data.
