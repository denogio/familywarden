# FamilyWarden Android — design brief for child, adult and parent views

Status: **historical input for the 2026-10-24 design round**, not current feature
availability. Use [UI-SPEC.md](family-views/UI-SPEC.md) and
[Android API readiness](../android-api-readiness.md) for current contracts and remaining
integration work. Mockups are English-only; Android resources provide EN/DA. Not an implementation
claim. Decisions referenced: product principle and D9/D10 in
[`plans/app-control.md`](../../plans/app-control.md); A13 (one app, role-based views),
A14 (child sees own history, read-only), A15 (240 min max request), A16 (app categories),
A17 (adjustable adult pool), A2 (native notifications on the parent's phone).

The existing design (`docs/design/familywarden-app/`, `UI-SPEC.md`) covers one managed
phone: DNS protection, Ulysses and removal. This round adds the family views around it.
Keep its brand, tokens, truthfulness rules and EN/DA requirement.

## 1. Principle: copy Family Link

Copy Google Family Link's structure, names and flows wherever possible; add only what the
user asked for. If a design choice is open, Family Link's choice is the default.

### Family Link reference (as of 2026-10)

Parent app (redesign rolled out in v2.47, March 2025) —
[9to5Google](https://9to5google.com/2025/03/20/family-link-redesign-android/),
[Google blog](https://blog.google/innovation-and-ai/technology/families/family-link-updates-february-2025/),
[Stillna walkthrough](https://stillna.app/tutorials/en/family-link-for-kids/):

- Child switcher as a **pill at the top left**; three bottom tabs: **Screen time**,
  **Controls**, **Location**.
- **Screen time** tab, in order: **Time spent today** (total + most used apps, tap for a
  breakdown) → **Device** card with **Lock** and **Bonus time** → **Time limits** (daily
  limit *and* app limits behind one entry) → **Schedules** (Downtime, School time, *Tonight*).
- Daily limit: **Weekly schedule** per day plus **Apply to all days**. Limits apply
  **per device**.
- App limits: per app **Set limit**, **Allowed / Blocked**, **Unlimited time**. No categories.
- **Controls** tab: account, Google Play (approvals), YouTube, Chrome and web, Search,
  Photos, location settings, signed-in devices.

Child's device ([help](https://support.google.com/families/answer/7103340),
[schedules](https://support.google.com/families/answer/15938652),
[kids FAQ](https://support.google.com/accounts/community-guide/390673031)):

- Notification when time is almost up. At the daily limit the phone **locks**: no unlock,
  no notifications; calls can be answered, emergency calls work.
- **Downtime** (night lock): calls and Unlimited apps keep working.
- **School time**: own home screen with only allowed apps; notifications quiet; calls/texts.
- New app download: child taps **Ask**, parent gets Approve/Deny.
- *Asking for more time:* Google's kids FAQ describes an **Ask for more time** button on the
  near-limit notification; a 2026 walkthrough says that on Android only iPhone has it and
  Android kids ask out loud. Sources disagree. FamilyWarden has it regardless (below).

### Deliberate differences

| Topic | Family Link | FamilyWarden |
|---|---|---|
| Ask for more time | Total only, platform-dependent | **Total or one specific app** (user addition), up to 240 min, parent may approve offline from their phone |
| Budget | Per device | **Shared per child** across devices (D4) |
| At the limit | Device locks | Ordinary apps **paused** (Android dialog title is fixed "Blocked by work policy", body is ours); FamilyWarden itself stays open as the child's "time's up" screen |
| Categories | None | Yes (A16), mainly for Ulysses rules |
| Extra | — | Router internet block/ticket, DNS filtering, adult self-limits (Ulysses) |
| Location | Yes | **Not planned** |

## 2. One app, views by role (A13)

Roles come from the router-signed policy, never from a local switch. One phone can hold
several (the user's own phone: **adult** under Ulysses **and** **parent** of children).

| Role | Device Owner | Main questions it answers |
|---|---|---|
| **Child** (managed) | Yes | How much time do I have? Why is this app paused? Can I ask for more? What did I use? |
| **Adult** (managed, self-limits) | Yes | My time today, my pool, Ulysses state, my protection |
| **Parent** | No | How are my kids doing? Give bonus, lock, approve requests and new apps, set limits |
| **Administrator** | — | Enrol phones, router and DNS settings: **LuCI first**; the app shows a link/state only |

Navigation is built from capabilities (as in `UI-SPEC.md`): a parent-only phone shows the
family views; a child phone never shows parent controls.

## 3. Screens to design

### Child (Family Link child experience + our additions)

- **Today** (home): remaining total time (ring/bar), state chip (*Allowed* / *Downtime until
  07:00* / *Time's up* / *Paused by a parent*), next change, per-app rows for limited apps
  (*12 of 30 min*), always-allowed apps listed as usable. Offline/stale state stated plainly.
- **Ask for more time**: choose *Total* or an app → minutes (presets, max 240) → optional
  note → sent. One open request at a time (A7): show *Waiting for Dad*, expiry (1 h),
  *Approved: +30 min for YouTube until 18:40* (2-hour lifetime, D3), *Denied*.
- **Paused app**: Android shows its fixed dialog; our body text + a FamilyWarden
  notification *YouTube is paused: daily limit reached* with action **Ask for more time**.
- **New app waiting**: *New app needs approval* with **Ask** (A12: may have launched once).
- **Near-limit notification**: *5 minutes left on YouTube today* (D10).
- **History** (A14, read-only): per day, last 30 days — app time and visited websites
  (domains, with the caveat that it is DNS-based and not complete).
- **Settings**: phone/router status, DNS protection (existing screens), no editing.

### Adult (existing Ulysses + DNS design, extended)

- **Today**: own time, per-app/category limits, **daily pool** remaining with **Use from
  pool** (A4/A17; under a pact only the pool, no self-approval).
- Ulysses and DNS screens as already designed. Category grouping (e.g. *Productivity*) in
  rule summaries (A16).

### Parent (Family Link parent app structure)

- **Child switcher pill**; tabs **Screen time**, **Controls**, **Requests** (instead of
  Location; badge for pending items; native notifications per A2).
- **Screen time**: *Time spent today* + most used apps → **Device** card(s) with **Lock**
  and **Bonus time** (+15/+30/+60, total or one app, today only) → **Time limits**
  (daily: *All days* / *Weekdays + weekend* / per day; app limits; Unlimited/always
  allowed; Blocked) → **Schedules** (Downtime with *Tonight*; School time later).
- **Requests**: extra time (app or total, requested minutes, approve with fewer, deny),
  new apps (approve/deny). Shows expiry and *approved by the other parent*.
- **Controls**: new-app approval setting (`suspend_new`), DNS protection level, internet at
  the router (block now / allow / ticket — our addition), the child's phones and their
  sync/applied state (*Up to date* vs *Not applied yet*).

## 4. Data truthfulness: what exists now vs later

Design every screen, but mark which data is real today. Never show invented numbers.

| Data | Now | Later |
|---|---|---|
| Per-app limits, blocked, downtime on the phone | Signed policy v3, enforced (piloted) | — |
| Applied state of a phone (*up to date*) | Router: `apps_applied` vs `apps_rev` | — |
| Child's own time today (on the child's phone) | Counted locally | — |
| Child's time on the **parent's** phone, history | — | Usage upload (P6), retention (A8) |
| Total limit, always allowed, bonus, warnings | — | P7b |
| Requests, approvals, parent role, shared budget | — | P8 |
| Native notifications on the parent's phone | — | P9 (transport A2 open: FCM vs UnifiedPush) |
| Website history (domains) | — | From AdGuard Home (P6/P12) |
| Lock now, School time | — | P10 / later |

States the parent views must show: never synced, stale (last sync time), child phone
offline, policy not yet applied, request expired, approved but not yet delivered.

## 5. Open design questions for the design round

1. Child "Time's up" experience: FamilyWarden full-screen status vs relying on paused
   icons and the Android dialog. (We cannot show Family Link's system lock screen.)
2. Parent approving on the child's own phone (Family Link allows a parent password there).
3. How an adult who is also a parent switches between *My phone* and *Family* without
   mixing self-limits with child management.
4. Tone for children (ages ~8–15): friendly, non-shaming, no surveillance language.

## 6. Next step

Turn this brief into a Claude Design prompt (like
[`familywarden-app-design-prompt.md`](familywarden-app-design-prompt.md)): reuse the
lighthouse brand and Material 3 tokens, boards per role in EN and DA, light/dark, large
font, and the states in §4.
