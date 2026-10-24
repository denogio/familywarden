# Family views design (child, adult, parent)

`design.html` is the Claude Design export from 2026-10-24, archived **unchanged**
(sha256 prefix `94a3b06b2483a2eb`). It was made from
[`../role-app-design-prompt.md`](../role-app-design-prompt.md) and
[`../role-app-brief.md`](../role-app-brief.md). Open it in a browser; it unpacks itself
offline (embedded Noto fonts, OFL licence kept in the file).

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
| The first parent to answer decides; the other sees who | Not decided before | Proposed by design |
| A grant's 2 hours start on **delivery** to the phone | D3: valid **2 hours after approval** | **Conflict — user to decide** |
| The adult pool refills at **04:00** | D8: the day starts at **midnight** in the router's timezone; A17 pool adjustable | **Conflict — user to decide** |

## "Later" tags vs. current state

The boards mark total daily limit, Unlimited (always-allowed) apps, Bonus and near-limit
warnings as later. These were built in P7b after the design round (router `3def6df`,
Android `6b99ca5`, LuCI `f4d231f`) and are being verified. Still later: usage upload to the
parent's phone, requests/approvals, parent role and notifications, shared budget, Lock
now, School time, website history.

All people, phones and apps on the boards are example data.
