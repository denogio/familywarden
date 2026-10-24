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

## 2a. Wire v3 as implemented (P5, 2026-10-22)

The first implemented subset of §2. Signing, enrollment and the router key stay in the
shell plugin for now (usign, unchanged pins); moving identity into the Go core (A6) is a
later migration, not a wire change.

**Request.** A v3 phone adds two parameters to the v2 request and signs two messages
with its enrolled key:

- `sig` over `familywarden-sync-v2\n<id>\n<nonce>\n` (unchanged: an old router answers v2);
- `sig3` over `familywarden-sync-v3\n<id>\n<nonce>\n<applied>\n`, with `applied` = the
  16-hex app policy revision the phone **enforces**, or `-`. Both must verify, else 403;
  a malformed `applied`/`sig3` is 400.

**Reply.** `v: 3` and everything v2 has, plus, when the phone's profile has app control
on (`option apps '1'`):

```json
"apps": { "rev": "adf7f46a6b7de5f1", "suspend_new": true,
  "limits": [ { "package": "com.roblox.client", "minutes": 45 } ],
  "blocked": [ "com.zhiliaoapp.musically" ],
  "approved": [ "com.roblox.client", "com.spotify.music" ],
  "downtime": [ { "days": "mon fri", "start": "21:30", "stop": "07:00" } ] }
```

- UCI source: profile `apps`, `apps_suspend_new` (default 1, A12); `config app`
  (`profile package minutes blocked`) and `config app_downtime` (`profile days start stop`).
- `approved` = configured and not blocked; `limits` only for approved packages. Duplicate
  package sections merge to the strictest rule (blocked wins, smallest limit wins). The
  router drops invalid package names/downtime with a log line; the phone **rejects** a
  contradictory or oversized policy (keeps its last trusted one) rather than repairing it.
- `rev` = first 16 hex of SHA-256 over the canonical rule lines; any change, also of
  `suspend_new`, changes it. Pending/unapproved phones get no `apps`; v2 replies never do.
- **Pact:** the captain's app rules are part of the pact snapshot (frozen like schedules,
  refreshed only in the amendment window). Without this, live app sections of the
  captain would be dropped from the effective config and the rules loosened.
- **Acknowledgement:** the router stores the signed `applied` revision per phone (RAM,
  with last-seen); `ubus call familywarden devices` shows `apps_applied` next to the
  expected `apps_rev`. Fetching is not applying: the app sends `-` until enforcement
  has actually applied a revision.
- Not yet in v3: shared `budget`, `grants`, `parents`, categories and parent roles.
  Required-feature negotiation is now implemented separately in §2b; unknown optional
  fields remain ignorable, but unknown required features are not.

**Fixture.** `android/app/src/test/resources/v3-payload.{json,sig}` + `v3-router.pub`
are a real reply from the shell plugin (jshn + usign); `WireV3FixtureTest` verifies the
signature and parses it. Regenerate them after any wire change.

### Compatibility and rollback matrix

| Phone app \ Router | 0.9.2 (v2 only) | with v3 |
|---|---|---|
| 0.9.0 pilot (v2, no `sig3`) | v2 | v2 only if app control is off; otherwise signed refusal (§2b) |
| v3 app, never accepted a v3 reply | v2 accepted; no app control | v3 |
| v3 app, has accepted v3 | **refused as "downgraded reply"**; keeps the last trusted policy offline, shows a sync error | v3 |

- Router rollback after phones saw v3: phones keep enforcing their last signed policy
  and report the sync error until the router is upgraded again, or until the phone is
  re-paired (pairing is a fresh trust decision and resets the highest seen version).
- Upgrading the router during a pact: existing snapshots have no app sections, so the
  captain gets app control only after the amendment window or the next pact. Nothing is
  loosened, because there were no app rules before.

## 2b. Authenticated feature support (implemented 2026-10-24)

This closes a compatibility hole: an old v3 client could ignore P7b fields and still
acknowledge their revision. It is a prerequisite for adding parent roles, not an
implementation of parent authorization.

**Request.** Current clients additionally send `caps=apps-v1,screen-time-v1` and
`sigcaps`, signed by the same enrolled phone key over these exact bytes:

```text
familywarden-sync-caps-v1\n<id>\n<nonce>\n<applied>\n<caps>\n
```

Here `\n` denotes a single newline. `caps` is a sorted, unique comma-separated list
of at most 16 names matching `[a-z][a-z0-9-]{0,31}`, or `-` for no features.
Both parameters must be present together and require the existing valid v3
signature. The signature binds the list to this device, nonce and applied revision;
it is not interchangeable with `sig` or `sig3`. Bad syntax is HTTP 400, signature
substitution is HTTP 403. Old routers ignore the extra parameters.

**Requirements.** The router derives root-level `requires` from the **effective**
configuration (including the pact snapshot), never from client claims:

| Feature | Required when |
|---|---|
| `apps-v1` | This profile has app control enabled |
| `screen-time-v1` | The app policy has daily screen limits, always-allowed apps, whole-phone downtime, or today's bonus |

`screen-time-v1` names the complete P7b semantics: union counting, ordinary-app
downtime (including messaging unless always allowed), daily limits, unlimited
exceptions and dated bonus. Capability claims are **not remote attestation** and
confer no parent/admin authority.

Legacy v3 requests without `caps` have implicit `apps-v1` support only. Thus basic
per-app limits still work, but P7b policy is refused. A v2 request cannot accept an
app-controlled binding at all: stripping `sig3/caps/sigcaps` cannot obtain an approved,
weaker v2 policy. Profiles without app control retain v2 compatibility.

**Signed refusal.** Missing support returns a normally router-signed response with
`v`, `server_time`, `nonce`, `device`, `enrollment: "approved"`, `requires` and
`error: "unsupported_capabilities"`. It contains **no binding, DNS, pact/rules or app
policy**. It is not an accepted enrollment/policy transition. Even old parsers reject
it because mandatory binding/rules are absent. The current app verifies the signature
and request binding, reports the unsupported policy, and retains its last trusted
policy. It never grants removal from this error response.

Phone-side applied acknowledgements are stored with the implementation capability
set. An older/missing set makes `appsApplied` read as absent after an APK upgrade,
even if the revision hash is unchanged; enforcement must successfully apply the
policy before the new app acknowledges it. Future incompatible semantic changes
must use a new feature name, not silently reuse an existing capability.

Android strictly validates `requires` before parsing either approved or pending
policy: bounded array, string names, no duplicates, no unknown required features.
Unknown root-level errors are also refused. No arbitrary cache expiry or automatic
loosening is introduced.

**Reporting.** The router clears incompatible signed acknowledgements, records the
authenticated list and last refusal, and returns `capabilities` / `apps_error` in
`devices`. Compatibility is also checked when reading old two-line seen files, so
an old matching revision is not displayed as current between a router upgrade and
the next phone fetch. With app control off, obsolete capability errors are suppressed.
LuCI distinguishes upgrade-needed, waiting, not configured, missing reporting and
"Current revision acknowledged" (not a perpetual enforcement/online attestation).

**Rollout.** Update phones to a build with this capability handshake **before**
upgrading the router or enabling P7b requirements. Older P7b builds without the
handshake are also refused where `screen-time-v1` is required. While refused, an old
phone keeps its cached policy and cannot use that refusal as fresh removal authority.
Pact-bound app rules cannot be disabled as a workaround outside their normal amendment
or release rules. This change has not been deployed to the production router/phone.

### Regenerate interoperability fixtures

Only in a disposable OpenWrt integration-test container; never on a router with real
configuration. The helper uses the integration-generated fictional phone key; no
private key is copied out.

```sh
docker run -d --name fw-fixtures --cap-add NET_ADMIN \
  -v "$PWD:/src:ro" openwrt/rootfs:x86-64-25.12.4 sleep infinity
docker exec fw-fixtures sh /src/scripts/test-integration.sh --inside
docker exec -e FW_TEST_FIXTURES=1 fw-fixtures sh /src/test/android/generate-v3-fixtures.sh
for file in v3-payload.json v3-payload.sig v3-router.pub v3-refused.json v3-refused.sig; do
  docker cp "fw-fixtures:/tmp/$file" "android/app/src/test/resources/$file"
done
docker rm -f fw-fixtures
```

JVM fixtures verify both the exact signed accepted policy and the signed refusal.

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

- v2 phones keep working on profiles without app control. An app-controlled profile
  refuses unsupported clients rather than returning a weaker approved policy (§2b).
- An enrolled phone that has received v3 refuses later v2/v1 replies (no downgrade), as today.
- Unknown fields are ignored. If a reply contains `"requires": ["feature", …]` that the phone
  does not implement, it keeps enforcing its last trusted policy and reports `unsupported_version`.
- All signed messages carry a type-specific prefix line, so a signature for one message type
  can never be accepted as another.

## Still open (not blocking the LAN slice)

- ~~Per-role visibility of history~~ decided A14: the child sees own app and domain history, read-only.
- Off-home transport (A2): native notifications on the parent phone decided; FCM vs UnifiedPush open — needed for P9, not for the LAN slice.
- ~~Category source and maximum grant size~~ decided A15/A16: 240 min; Android category + router override.
