# FamilyWarden LuCI — Claude Design continuation prompt

Paste the text below into the existing Claude Design conversation used for the
FamilyWarden Android design. No need to start a new brand exploration.

Suggested attachments, if they are not already available in that thread:
- `docs/screenshots/overview-dark.png` and `docs/screenshots/ulysses.png` (implemented app).
- `docs/design/familywarden-app/design.html` (archived visual reference, not current behavior).
- Sanitized current LuCI screenshots if available; do not attach production names,
  credentials, real device identifiers or router URLs.

---

Continue the **FamilyWarden** work in this conversation. You previously designed our
Android app and lighthouse identity. Now design its companion **OpenWrt LuCI interface**
so both feel like the same product without turning router administration into a phone app.

Reuse the brand and earlier design language. Do not redesign the logo, start discovery
from scratch, or overwrite the Android boards. Add a separate **FamilyWarden — LuCI** page.

## 1. Goal and scope

FamilyWarden is a self-hosted parental-control system. The current router interface
manages household profiles, network access schedules, temporary internet access,
Android phone enrollment, AdGuard Home DNS protection and optional adult Ulysses pacts.

This task is **visual/interaction design, not feature implementation**. Make the
existing product coherent with Android, focusing first on **Overview**. Propose the
same design system for Profiles, Phones, Settings and Info.

It remains a LuCI package under **Services → FamilyWarden**, inside the user's existing
router interface. Preserve the surrounding LuCI header, navigation, account controls,
page structure and native form action footer. Show enough neutral LuCI chrome to make
that integration credible, but do not design a replacement OpenWrt theme.

Do not add an independent sidebar, mobile-app bottom bar, new frontend framework,
cloud account/subscription flow or a standalone dashboard application.

## 2. Translate the existing Android visual language

Keep the lighthouse as the umbrella identity, and the mast/sail only for Ulysses.
Use night-sea surfaces, mist text, restrained gold accents, calm serif headlines,
rounded cards and clear status rows. Avoid nautical decoration behind dense forms,
excessive hero space, giant metrics, glass effects and gradients on every panel.

Actual implemented Android reference colors:
- Night `#1B2A3A`, deep night `#070B10`, dark page `#0A1018`, dark surface `#121C27`.
- Dark line `#2A3B4D`, mist `#D8E1EA`, muted mist `#B4C4D3`.
- Gold `#C9A227`, bright gold `#E0BD4A`.
- Light page `#EEF2F6`, surface `#FFFFFF`, ink `#0F1B27`, muted ink `#4A5D70`, line `#CDD6DF`.
- Success teal `#1F6A64` light / `#7CC7BD` dark.
- Genuine-fault rust/coral `#8F2F0C` light / `#FFB38A` dark.

Treat these as semantic brand references, not unconditional CSS overrides. Keep native
LuCI form controls recognizable. Gold on white is not acceptable small-text contrast.
Use system serif for a few brand/headline lines, system sans for forms/body, monospace
for fingerprints/hostnames. No downloaded fonts, icon CDNs or external asset requests.

Blocked-by-schedule is an expected state, not a red alarm. A pact holding is calm and
intentional, not a punishment. Red/problem styling is for actual failures.

## 3. Existing navigation and behavior

Keep these real destinations and their meaning:

### Overview — primary detailed design

Answer: **Who currently has internet access, what rules apply, and what needs attention?**

Existing data/actions:
- Profiles with name, policy (Unrestricted / Scheduled / Blocked), effective network
  Allowed/Blocked state and temporary override where applicable.
- Assigned MAC devices, optionally resolved friendly names.
- Weekly allow windows with days, start/stop and next access transition when known.
- Timed internet-access tickets and remaining time.
- Captain pact: fixed end or ongoing, amendment window, sailing-home release time
  when requested, last policy fetch when supplied.
- Add Profile and open profile editor.
- Immediate quick actions: Block, Allow, Reset override, and +5/+10/+30-minute
  internet tickets where allowed. These are **whole-profile network access**, not app time.
- Captain-only Make / Extend / Amend pact, subject to backend permissions.
- Notices for FamilyWarden disabled and unassigned devices blocked by default policy.

Rework the hierarchy rather than reproducing today's small text and many colored chips.
Make the profile name and access state scannable, actions clear and schedules readable.
Show a household view with fictional Alex, Sam and an adult captain Morgan. Profiles
are not necessarily people: a TV/devices profile must also fit the design.

IMPORTANT: today's counters say Online/Offline but count Allowed/Blocked profiles.
Use **Internet allowed / Internet blocked**, not presence/connectivity claims. A DHCP
lease is not proof a device is online. Make `unknown` and failed refresh explicit;
never default missing data to green or zero.

The current Overview does not request DNS/phone-list data. You may propose a compact
AGH sync or pending-phone notice assembled from existing APIs, but identify it in the
handoff as **additional frontend wiring**, not an already displayed fact.

### Profiles

Keep native editable forms, grouped into:
- Identity, enabled state, access policy and assigned MAC devices.
- Router-access blocking while internet is blocked.
- Weekly internet allow windows, edited **inline** (days/start/stop/add/remove),
  not moved into a new modal-only schedule editor.
- Strict DNS on the profile's approved phones; supported protections: global filter
  lists, malware/phishing protection, adult-site protection, SafeSearch, blocked services.
- Stable DNS identity/hostname and AGH sync state where known.

Explain that scheduled mode blocks outside allow windows. Preserve validation, pending
changes and native Save / Save & Apply / Reset. These are not instant-save toggles.

For a frozen captain, show the lock reason and applicable end/amendment window. DNS
remains editable with copy explaining additions now and deferred loosening under the
pact. Do not imply every editable field takes effect immediately, or that phone bindings
can change in the amendment window.

### Phones

Separate **Waiting for approval** from **Approved phones**. Pending enrollment is the
most important task here, not a generic online-device list.

Pending: model/name, full grouped key fingerprint, last request, Approve / Reject.
Approval dialog: prominently compare the fingerprint with the phone, assign a profile
and optionally administrator status. Do not reduce trust approval to a tiny icon.

Approved: name, profile, administrator flag, full fingerprint, last policy fetch,
active removal-approval deadline when any, and pact-bound state.

Keep these distinct:
- Edit enrollment.
- **Allow removal for 1 hour**: allows the phone's own removal flow, still forbidden
  while an applicable pact is active.
- **Remove enrollment**: stops new policy delivery; it does NOT uninstall the app or
  erase its last cached protection. Explain that consequence in confirmation copy.

Pact-bound phone actions are frozen. Fingerprints must be readable/copyable on narrow
screens; do not silently truncate their only accessible representation.

### Settings

Visually organize existing functions into:
- General: enabled/default policy, with pact lock explanation where applicable.
- Phone pairing: QR, router key fingerprint, instructions and link to Phones approval.
  Pairing serves all FamilyWarden phones, not only the captain; it should not feel like
  a Ulysses prerequisite.
- DNS / AdGuard Home: DoT domain, AGH API address, credential state, set/change login
  dialog, last sync, sync errors, per-profile DNS identities, Sync now.
- Ulysses: captain selection and cooling-off period; locks during an active pact.

Use an organization implementable with existing LuCI sections; do not invent a new
navigation system. Distinguish immediate RPC actions (credentials/sync) from staged
UCI edits. Stored passwords are never displayed or read back. Plain HTTP AGH URLs need
an honest LAN-only credential-exposure warning, not a secure/lock badge.

### Info

Compact diagnostics: installed backend, installed frontend, running JS version and
browser-cache mismatch warning. No CPU/traffic/history charts or invented telemetry.

## 4. Truthfulness and security constraints

The Android design separates policy, Android application, resolver health and router
reachability. Preserve that reasoning, but **do not copy phone-only facts into LuCI**.

- Router AGH sync means the router reconciled its client configuration; it does not
  prove Android applied the policy, a phone is reachable, or its current DNS probe passed.
- Last request/policy fetch does not prove applied enforcement. Display unavailable
  phone-side evidence as unavailable, or omit it—never fabricate a green status.
- A phone can keep DNS protection on mobile data while unable to reach its home router.
  Retain cached policy; no arbitrary 24/48-hour expiration or alarming stale countdown.
- Captain and administrator are independent. The current admin flag supports self-removal
  rights; future parent management is not implemented by that flag alone.
- Hiding/locking controls is presentation; the backend enforces permission and pact rules.
- Pact creation can immediately make LuCI/SSH unreachable during a blocked period.
  Preserve a clear warning before confirmation. Supported early exit is the existing
  sail page on port 7700 with authentication and cooling-off—not an Unlock button.
- Never offer an automatic unfiltered DNS fallback, bypass certificate validation or
  one-click end-pact action. Ordinary administrative DNS controls still exist; do not
  remove them just because the managed-phone screen has no disable switch.

## 5. Do not design future features as current functionality

App limits, app/category usage, child extra-time requests, new-app approvals and parent
approval roles are the next development project, **not built yet**. Bandwidth accounting
also has no working backend method despite an old frontend hook.

No working-looking app-time cards, pending app-request badges, browsing history,
bandwidth graphs, arbitrary per-client blocklist groups or per-domain exceptions.
No empty tabs or Coming soon menus. If useful, add ONE separately labeled future
extension sketch, outside the current-product boards. It must not drive today's navigation.

## 6. Responsive and accessible behavior

- Show desktop at roughly 1440 px and narrow viewport at roughly 390 px.
- Demonstrate 200% browser zoom/reflow with no clipped actions or overlapping labels.
- Preserve normal page scrolling and useful density; no full-height nested app shell.
- Keyboard navigation, visible focus, accessible labels, real heading hierarchy and
  text/icon status, not color alone. Aim for WCAG AA contrast and touch-friendly targets.
- Tables may adapt to stacked labeled rows/cards on mobile. Long hostnames, device names
  and fingerprints must wrap without hiding critical information.
- Support the installed LuCI light/dark theme; do not switch theme for unrelated pages.
- Polling must not reset forms, steal focus, close dialogs or cause cards to jump around.
- English mockups only; allow longer translated labels. Use fictional data and example.net
  hostnames; never real children, accounts, keys or deployment addresses.

## 7. Deliverables

1. Short explanation of how the Android visual identity translates to native LuCI.
2. **Detailed Overview** in desktop light/dark and narrow light/dark, with an accessible
   zoom/reflow example. Keep the same plausible fixture data across variants.
3. Additional Overview states: no profiles; normal scheduled blocking; captain pact
   holding; disabled service; failed refresh retaining explicitly stale previous data.
4. Companion layouts for Profiles, Phones, Settings and Info. Give light/dark treatment
   for each; show the key narrow-screen adaptation rather than every possible board.
5. Interaction/state examples: fingerprint approval dialog; pact-frozen profile;
   one-hour removal approval vs removing enrollment; AGH sync failure; staged unsaved form.
6. A compact reusable token/component inventory: page header, profile card, fact row,
   status pill, schedule rows, pending-approval panel, notice, lock explanation and dialog.
7. Implementation handoff table: component → existing LuCI page/behavior → responsive
   rules → state/copy → existing data source vs extra frontend wiring vs unavailable data.
8. If exporting a prototype, provide a self-contained HTML/CSS reference with no external
   dependencies. Scope proposed styles to a FamilyWarden container (`.fw-*`); do not
   globally restyle `body`, headings, inputs, buttons, navigation or unrelated LuCI pages.
   The prototype is a reference, not production LuCI code or a reason to replace forms.

Prioritize a polished, practical Overview and a coherent reusable language over a huge
speculative redesign. Stay recognizably FamilyWarden, and recognizably LuCI.
