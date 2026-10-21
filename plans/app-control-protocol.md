# App control — protocol and data contract (P1 draft)

Status: **draft for the first vertical slice** (LAN: child requests extra time for one app,
parent approves, child uses exactly the approved minutes). Built on the existing v2
enrollment/policy channel (`plans/app-control-baseline.md`). Decisions referenced as
"D1–D8" are the contract decisions in `app-control.md` (2026-10-21).
Field names are proposals until the Go core implements and tests them; compatibility
rules below are binding from the first release that ships them.

## 1. Principals and keys

| Principal | Key | Already exists? | Trusted by |
|---|---|---|---|
| Router | usign Ed25519 (`mast.key`) | yes | every paired phone (pinned at pairing) |
| Phone | Ed25519 per device (`PhoneKey`) | yes | router, after fingerprint approval |
| Parent phone | the same phone key | yes | children's phones, **via the router-signed policy** |

Roles are attributes of an approved device binding, set by an administrator in LuCI:

- `role: "child" | "adult" | "parent"` — `child` and `adult` are *subject to* app rules;
  `parent` approves. A device can be `parent` and also bound to an adult profile.
- `approves: [profile_id…]` — the child profiles a parent phone may grant for.
- `administrator` (existing) keeps its meaning: self-removal rights. It does **not** grant
  approval power, and `parent` does not imply administrator or captain.

A parent phone needs no Device Owner; it only signs. Revocation = removing the parent role
in LuCI; a child phone learns it at its next successful sync (D2 consequence).

## 2. Policy v3 (router → phone, signed reply as today)

v3 is a strict superset of v2; unchanged v2 fields keep their meaning.

```json
{
  "v": 3, "server_time": 1792540000, "nonce": "…", "device": "…",
  "enrollment": "approved", "timezone": "Europe/Copenhagen",
  "binding": { "device_id": "…", "profile_id": "alex", "role": "child", "…": "v2 fields" },
  "apps": {
    "rev": 42,
    "day": "2026-10-21",
    "rules": [
      { "id": "r1", "match": { "packages": ["com.roblox.client"] }, "daily_minutes": 60 },
      { "id": "r2", "match": { "category": "social" }, "daily_minutes": 30 }
    ],
    "downtime": [ { "days": "mon tue wed thu", "start": "21:00", "stop": "07:00" } ],
    "blocked": ["com.example.blocked"],
    "essential": ["com.google.android.dialer", "io.github.denogio.familywarden"],
    "approved": ["com.roblox.client", "com.spotify.music"],
    "new_apps": "suspend_until_approved",
    "budget": { "as_of": 1792539900, "remaining": { "r1": 1800, "r2": 0 } },
    "grants": [ "…active grants the router knows, see §4…" ]
  },
  "parents": [ { "device_id": "…", "name": "Dad's phone", "pubkey": "RWQ…" } ]
}
```

- `parents` lists only parents whose `approves` includes this child's profile. The child
  phone trusts a grant signature only from these keys or from the router key.
- `budget.remaining` is the **shared per-child** remainder (D4) at `as_of`, in seconds.
- `essential` packages are never suspended, whatever the rules say.
- Category membership comes from Android `ApplicationInfo.category` plus a router-side
  override list; unknown means "no category".

### Precedence (evaluated per package, first match wins)

1. `essential` → always allowed (dialer, FamilyWarden itself, launcher, input method).
2. `blocked` → suspended. A grant does **not** unblock it; the parent changes the rule.
3. Not in `approved` while `new_apps` is `suspend_until_approved` → suspended.
4. A valid grant with minutes left → allowed, also during downtime and beyond the budget (D3).
5. Inside `downtime` → suspended.
6. Any matching rule with no shared budget left → suspended.
7. Otherwise allowed.

Adult profiles under a pact never accept grants (§4); their daily pool is defined in P11.
Downtime windows use the router schedule semantics: `days` name the start day and an
overnight window continues into the next day.

## 3. Extra-time request (child phone → router)

Signed by the child phone key, sent on the LAN to the router (first slice); later also via
the off-home transport (A2/P9).

```json
{ "type": "request", "v": 1, "request_id": "uuid-v4", "device": "…", "profile": "alex",
  "package": "com.roblox.client", "minutes": 30, "created": 1792540012 }
```

Signed message: `familywarden-request-v1\n` followed by the **exact JSON bytes sent**; the
receiver verifies those bytes and only then parses them (no canonical JSON form, so Kotlin
and Go never have to agree on serialization). Rules (D7):
- At most **one open request per child profile**: a second is refused with `request_open`.
- A request expires **1 hour** after `created` (router clock) → `request_expired`.
- No rate limit. Denial and expiry are visible to the child.
- `minutes` is a positive integer, max 240; the parent may approve fewer.

## 4. Grant (router or parent phone → child phone)

```json
{ "type": "grant", "v": 1, "grant_id": "uuid-v4", "request_id": "uuid-v4 or null",
  "issuer": "router | <parent device_id>", "profile": "alex",
  "package": "com.roblox.client", "minutes": 20,
  "not_before": 1792540100, "not_after": 1792547300 }
```

Signed message: `familywarden-grant-v1\n` + the exact grant JSON bytes, by the issuer's key.
- **Lifetime (D3):** `not_after = issue time + 2 h`; unused minutes are lost then, not at midnight.
- **Bedtime (D3):** a valid grant permits the granted package during downtime/outside
  windows and beyond its daily rule, for exactly `minutes` of counted use.
- **Single use:** the child phone stores used `grant_id`s until `not_after`; a replayed
  grant is ignored. Grants for another profile, an unknown issuer or an expired window are
  rejected locally and reported.
- **Network (D3):** when the router learns a grant (issued by it, relayed from a parent, or
  presented by the child phone), it opens a profile **internet ticket** until `not_after`
  (bounded by the existing ticket rules). Off home, the phone needs nothing from the router.
- **Pacts:** grants never apply to an adult profile bound by a pact (no self-approval
  beyond the daily pool, A4). The router refuses to sign them; phones refuse them.

Clock: the phone validates `not_before/not_after` against the last signed `server_time`
plus elapsed monotonic time since that sync, never the wall clock alone.

## 5. Usage reports (child/adult phone → router)

```json
{ "type": "usage", "v": 1, "device": "…", "day": "2026-10-21", "seq": 17,
  "apps": { "com.roblox.client": 1260, "com.spotify.music": 0 }, "grants_used": { "uuid": 600 } }
```

- **What counts (D5):** seconds with the screen on and the app visible in the foreground,
  including picture-in-picture for the PiP app. Screen-off background audio does not count.
- **Day (D8):** midnight in the router's `timezone`, regardless of the phone's timezone.
- Reports are **cumulative per (device, day)**; `seq` increases. The router keeps the
  highest `seq` per device and day, so retries and duplicates cannot double count.
- The router sums all of a child's devices into the shared budget (D4) and returns the new
  `budget.remaining` in the next policy reply. Offline overspend is shown, not hidden.

## 6. Storage and retention (D6)

- App activity: per profile, per day, per package seconds — **30 days**, then deleted.
- Websites: per profile, per day, per **domain** request counts, derived daily from
  AdGuard Home's query log for the profile's `familywarden-*` client — **30 days**. Domains
  only; no URLs or content. FamilyWarden never changes AdGuard Home's global query-log
  settings; if AGH retains less than a day, website history is reported as unavailable.
- Stored by the Go core in the household (router storage; flash-write budget to be checked
  in P3/P4). Nothing leaves the household.

## 7. Error codes (all replies are signed JSON objects)

`not_enrolled`, `bad_signature`, `replay`, `wrong_profile`, `not_parent`, `request_open`,
`request_expired`, `grant_expired`, `invalid_minutes`, `unknown_package`, `pact_forbids`,
`unsupported_version`, `malformed`. Grant validity tolerates 5 minutes of clock skew between
the issuing phone and the router-anchored clock.

## 8. Compatibility

- v2 phones keep working: the router answers v2 unless the phone asks for v3.
- An enrolled phone that has received v3 refuses later v2/v1 replies (no downgrade), as today.
- Unknown fields are ignored. If a reply contains `"requires": ["feature", …]` that the phone
  does not implement, it keeps enforcing its last trusted policy and reports `unsupported_version`.
- All signed messages carry a type-specific prefix line, so a signature for one message type
  can never be accepted as another.

## Still open (not blocking the LAN slice)

- Per-role visibility of history (what a child sees of their own app/domain history).
- Off-home transport (A2) — needed for P9, not for the LAN slice.
- Category source details and the maximum grant size (240 min proposed).
