# FamilyWarden Android — family views design prompt (child, adult, parent)

Use this in the existing Claude Design conversation that produced the FamilyWarden app
design (lighthouse brand, Overview / DNS protection / Ulysses / Settings). Background and
sources: [`role-app-brief.md`](role-app-brief.md).

---

Extend your existing **FamilyWarden** Android design with the family views: what a
**child**, an **adult** and a **parent** see. Keep your brand, tokens, Material 3 /
Jetpack Compose target, truthfulness rules and accessibility rules from the previous
round. Do not redesign the existing DNS protection, Ulysses or Settings screens; place
them inside the new structure.

## Guiding principle: copy Google Family Link

FamilyWarden replaces Google Family Link in a self-hosted home. **Copy Family Link's
structure, names and flows as closely as possible**; deviate only where listed below.
When a choice is open, do what Family Link does.

Family Link today (parent app redesign, 2025):
- Child switcher as a **pill at the top left**; bottom tabs **Screen time**, **Controls**,
  **Location**.
- **Screen time** tab, top to bottom: **Time spent today** (total + most used apps, tap for
  breakdown) → **Device** card with **Lock** and **Bonus time** → **Time limits** (daily
  limit and app limits behind one entry) → **Schedules** (Downtime, School time, *Tonight*).
- Daily limit by **weekly schedule** with **Apply to all days**. App limits per app:
  **Set limit**, **Allowed / Blocked**, **Unlimited time**.
- Child: a notification when time is almost up; at the limit the phone locks (calls and
  emergency calls work); **Downtime** locks the phone except calls and Unlimited apps;
  new app downloads need the parent (**Ask** → parent **Approve / Deny**).

## Deviations from Family Link (decided)

1. **Ask for more time for one specific app**, or for the total — the user's main addition.
   Up to **240 minutes** per request; **one open request** per child; a request expires
   after **1 hour**; an approved grant is valid for **2 hours** and also opens the child's
   home internet for that time. A parent may approve from their phone even when away.
2. **Budgets are shared per child** across their devices (Family Link: per device).
3. **At the limit, apps are paused, not the whole phone locked.** Android then shows its
   own dialog with a fixed title *"Blocked by work policy"* (cannot be changed) and our
   short body text. FamilyWarden itself always stays openable.
4. **No Location tab.** The third parent tab is **Requests**.
5. Additions beyond Family Link: internet block/allow/ticket at the home router, DNS
   filtering (existing screens), **app categories** for grouping (mainly Ulysses rules),
   adult self-limits (Ulysses) with an **adjustable daily pool**.
6. New apps (when the parent turns on approval): they are paused as soon as installed;
   in rare cases one may open briefly before the pause. Never promise "blocked before use".

## One app, views by role

One APK. The roles a phone has come from the home router's signed policy, never from a
local switch. A phone can have several roles: the household admin's own phone is an
**adult** (under Ulysses) **and a parent**. Children's and adults' phones are managed
(Device Owner); a parent-only phone is not.

### Child

- **Today** (home): remaining time today (prominent), state (*Allowed*, *Downtime until
  07:00*, *Time's up*, *Paused by a parent*), next change, rows for limited apps
  (*12 of 30 min*), unlimited apps shown as usable.
- **Ask for more time**: choose *Total* or an app → minutes (presets, max 240) → optional
  note → sent; then *Waiting for Dad · expires 18:40*, *Approved: +30 min for YouTube
  until 20:15*, *Denied*, *Expired*.
- **Notifications**: *5 minutes left on YouTube today*; *YouTube is paused: daily limit
  reached* with action **Ask for more time**; *New app needs approval* with **Ask**.
- **Time's up** state: FamilyWarden opens to a calm full-screen status (what is still
  usable: calls, unlimited apps; when time resets; Ask for more time). This is our answer
  to Family Link's lock screen, which we cannot show. Propose how it feels, not a fake
  system lock.
- **History** (read-only, last 30 days): app time per day; visited websites as domains,
  with a short honest note that this comes from DNS and is not complete.
- **Settings**: protection and router status only (existing screens), nothing editable.
- Tone for ages ~8–15: friendly, plain, non-shaming. No surveillance or punishment
  language; the rules are the family's, not the app's.

### Adult (self-limits)

- **Today**: own time, limits per app or **category** (e.g. *Productivity*), **daily pool**
  remaining with **Use from pool** (during a Ulysses pact the pool is the only way past a
  limit; no self-approval beyond it).
- Existing Ulysses and DNS protection screens.

### Parent (Family Link parent app structure)

- **Child pill** at top left (children, and their phones); bottom tabs **Screen time**,
  **Controls**, **Requests** (badge for pending). Native Android notifications for new
  requests and new apps, with Approve / Deny where possible.
- **Screen time**: *Time spent today* + most used apps → **Device** card(s) with **Lock**
  and **Bonus time** (+15 / +30 / +60 / custom; total or one app; today only, does not
  change the schedule) → **Time limits** (daily limit: **All days** or **Weekdays +
  weekend**, per day optional; app limits; Unlimited; Blocked) → **Schedules** (Downtime
  weekly + *Tonight*; School time as a clearly marked later concept).
- **Requests**: extra time (app or total, requested minutes, approve as asked / approve
  fewer / deny; who approved if another parent did), new apps (approve / deny),
  expired items greyed.
- **Controls**: approval of new apps on/off, DNS protection level (existing concepts),
  **internet at home** (block now / allow / ticket for N minutes), the child's phones
  with sync and applied state (*Up to date*, *Not applied yet*, *Last seen 2 h ago*).
- Switching between **My phone** (adult self view) and **Family** (parent views) on a
  phone with both roles: propose a clear, low-friction pattern that never mixes an
  adult's own self-limits into child management.

### Administrator

Enrolling phones, router and DNS server settings stay in the router's web UI (LuCI) for
now. In the app show only an informative entry ("Managed on the router"), no fake forms.

## Truthfulness and data availability

Design every screen, but mark backend-dependent data clearly (e.g. a small "later" tag on
boards). Never invent numbers as if they were measured.

- **Exists now:** per-app limits, blocked apps and downtime enforced on the child's phone
  from the signed policy; the child's own counted time today on their phone; the router
  knows whether each phone has applied the current rules.
- **Later:** child time and history on the **parent's** phone (usage upload), total daily
  limit, unlimited apps, bonus, near-limit warnings, requests/approvals, shared budget,
  parent notifications, website history, Lock now, School time.
- Required states: never synced, stale (with last sync time), child phone offline,
  rules not yet applied on a phone, request expired, approved but not yet delivered to an
  offline phone, usage access missing on the child's phone (counting unavailable, blocks
  still active).

## Brand, language, accessibility

Same as the previous round: lighthouse brand, night-sea/gold palette used warmly for the
child views (still calm, not childish clip-art), light/dark, edge-to-edge, TalkBack,
200% font, no colour-only status. Mockups in **English only**; Danish is implemented
later from string resources (allow for longer labels). Use fictional people and example
data only: children **Alex** (11) and **Sam** (14), parents **Robin** and **Kim**; example
apps YouTube, Minecraft, Spotify, a school app, Maps.

## Deliverables

1. Navigation rationale for the four roles and role combinations (parent + adult on one
   phone), with a short mapping to Family Link screens.
2. High-fidelity light/dark boards:
   - Child: Today (allowed, near limit, time's up, downtime), Ask for more time flow
     (total and per-app, waiting, approved, denied, expired), notifications, new-app ask,
     History (apps, websites), usage access missing.
   - Adult: Today with categories and daily pool; pool use under a pact.
   - Parent: Screen time tab for Alex; Bonus time sheet; Time limits (all days vs weekdays
     + weekend, app limits, unlimited, blocked); Downtime; Requests inbox (pending,
     approved by Kim, expired); Controls incl. internet at home and phone status; parent
     notification with Approve / Deny; My phone ⇄ Family switching.
3. A component and state specification for stateless Compose UI: role/capability set,
   time-limit state per scope (total, app, category), request lifecycle, grant/bonus
   validity, sync/applied/stale states.
4. One 200%-font example per role; English UI copy for all boards.
5. Mark example data, later concepts and backend-dependent actions explicitly.
