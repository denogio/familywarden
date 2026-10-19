# App control: Family Link and Switchly alternative — planning

Status: **planning only** (2026-10-19). Nothing is built or deployed from this document.
Decisions below are the user's unless marked *proposed*.

## Goal

Per-app time control on Android phones, self-hosted by each household on its own network:

- Children: daily/weekly limits per app or category, downtime/bedtime, approval of new
  apps, usage overview — and the gap Family Link leaves: **ad hoc extra time**. A child
  asks for "15 more minutes of app X today"; a parent approves or denies in the app,
  without editing that weekday's standing limit.
- Adults who want it: self-limitation (Switchly-like), using the Ulysses idea that
  limits can only be tightened while a pact lasts.

## Confirmed decisions (2026-10-19)

1. **Self-hosted per household.** No hosted service run by the maintainer. Requiring
   OpenWrt is fine, but router-specific parts must sit behind an adapter so OPNsense and
   pfSense can be added later.
2. **New backend in Go or Rust** for app control, enrollment/policy and requests. Router
   features (firewall per MAC, lockdown, Ulysses hotspot) remain router adapters.
3. **Replaces Family Link** on the children's phones, with extended functionality; no
   coexistence layer (Family Link has no API to build on).
4. **Parents approve in the FamilyWarden app** (a parent role on the parent's phone).
5. **Notifications are FamilyWarden app notifications**, not a separate push app.
6. **Go** for the core (2026-10-19).
7. **Device Owner on children's phones where possible** (robust: suspend apps, block
   uninstall/factory reset/safe boot/new users). A weaker no-reset mode (accessibility
   service + usage access + device admin, as many commercial apps do) is possible later
   but bypassable; not first.
8. **Adult self-limit scope:** social media and browsers, limited to what is necessary:
   app suspension by schedule and/or daily quota, the same services blocked in DNS
   (catches web versions and in-app browsers), pact semantics (only tighten).
9. Test children's phone: **Pixel 7 Pro, Android 17.**

## Proposed architecture

- **Core daemon** (Go or Rust, one static binary): profiles, phones, keys, signed policy,
  app rules, requests/grants, usage summaries. Runs on the router (OpenWrt package) and
  is the authority. Not reachable from the internet.
- **Router adapters:** OpenWrt first (the existing rpcd/fw4 code can stay the firewall
  adapter at first); OPNsense/pfSense later (FreeBSD, pf). DNS/AdGuard sync moves into
  the core since it is not router-specific.
- **Android app:** one app with roles from the signed binding — managed child device,
  parent (approver), adult self-limit. Enforcement on the phone as Device Owner:
  `setPackagesSuspended` for blocked/exhausted apps, usage stats for counting, offline
  enforcement from the last signed policy (as Ulysses does today).
- **Messages:** request and approval are signed by the phone keys (child key / parent
  key, both approved in LuCI) and carry request ID, scope (app, minutes, day) and expiry,
  so an approval can't be replayed or widened. The child's phone can verify an approval
  directly; the router learns about it afterwards for the log.
- **Transport (proposed):** the app keeps its own connection to a relay with an
  ntfy-compatible protocol; payloads are end-to-end encrypted and signed, so the relay
  can't read, forge or approve. Households choose their own relay or a public one.
  No FCM (it would need the maintainer's Firebase credentials in every installation).

## Open decisions

| ID | Question | Proposal | State |
|---|---|---|---|
| A1 | Go or Rust? | Go | **Decided** |
| A2 | Notification transport | App-held connection to an ntfy-compatible relay, E2E encrypted; own relay or public | Open |
| A3 | Children's Google accounts without Family Link | Under 13, Google accounts must be supervised. Options: no Google account (apps installed by a parent / alternative store), or a supervised account whose Family Link limits are unused. Coexistence of a supervised account with our Device Owner is untested | Must be tested first |
| A4 | Adult self-limit: how to get access when really needed? | The user is administrator, so self-approval is no friction. Options: delay before extra time applies (e.g. 10 min), approval by another person, or a small daily pool | Open |
| A5 | Children's phones | Pixel 7 Pro, Android 17 | **Decided** |
| A6 | Core vs existing shell backend | Core takes identity, policy, DNS and app control; shell stays the OpenWrt firewall adapter until ported | Open |
| A7 | Request abuse and limits | Rate limit requests per child; approvals expire; optional "auto-deny after N" | Open |
| A8 | Usage data and privacy | Keep summaries per app/day on the router only, short retention, no per-site logs | Open |
| A9 | Device Owner setup for others | QR provisioning on a factory-reset phone instead of adb | Open |

## First gates before any feature code

1. **User's own Pixel 10** (already Device Owner with FamilyWarden, no reset needed):
   app suspension behavior, whether the app can grant itself usage access as Device
   Owner (or needs one tap), and battery cost of an app-held relay connection (A2).
2. **Pixel 7 Pro (child, needs a factory reset):** Device Owner via QR provisioning and
   whether a supervised child account can coexist with it (A3). If not, the phone is
   used without a Google account and apps are installed another way.

## Not in scope for now

iPhone (no equivalent API outside Screen Time), web dashboards, a maintainer-run service,
per-website history.
