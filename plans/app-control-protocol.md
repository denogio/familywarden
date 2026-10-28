# App control — protocol and data contract

Current LAN contract, reconciled 2026-10-28. Sections 2a–2c and 3–5c describe
implemented enrollment, policy, requests/decisions, device-bound grant consumption,
usage, cached shared offsets, new-app decisions and read-only app history.
Section 2 retains the original **future policy sketch**, not the accepted wire format.
Bounded offline multi-phone allocation, categories/pool, direct parent-to-child grants
and off-home delivery remain unimplemented. Android API/permission boundaries:
[API readiness](../docs/android-api-readiness.md). Product decisions: [app-control.md](app-control.md).

## 1. Principals and keys

| Principal | Key | Already exists? | Trusted by |
|---|---|---|---|
| Router | usign Ed25519 (`mast.key`) | yes | every paired phone (pinned at pairing) |
| Phone | Ed25519 per device (`PhoneKey`) | yes | router, after fingerprint approval |
| Parent phone | the same phone key | yes | children's phones, **via the router-signed policy** |

Roles are attributes of an approved device binding, set by an administrator in LuCI:

- There is no implemented `role` discriminator. Managed child/adult views derive from
  signed app policy/captain attributes; parent authority derives from `binding.approves`.
  A device may have approval scope and also be subject to its own managed rules.
- `approves: [profile_id…]` — the child profiles a parent phone may grant for.
- `administrator` (existing) keeps its meaning: self-removal rights. It does **not** grant
  approval power, and `parent` does not imply administrator or captain.

A parent phone needs no Device Owner; it only signs. Revocation = removing the parent role
in LuCI; a child phone learns it at its next successful sync (D2 consequence).

## 2a. Wire v3 as implemented (P5, 2026-10-22)

The first implemented subset of §2. Since core migration step 1, signing and enrollment
live in the Go core; the adapter reads the unchanged router key. This was not a wire change.

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
- Parent scopes/keys are implemented (§2c). Go now emits device-bound `apps.grants`
  (§4), gated by `grants-v1`; Android development builds since 2026-10-25 advertise it.
  Cached shared offsets are implemented (§5a); bounded offline allocation and categories
  remain unimplemented. Unknown optional fields are
  ignorable; unknown required features are not.

**Fixture.** `android/app/src/test/resources/v3-payload.{json,sig}` + `v3-router.pub`
are real Go-core-signed replies; `WireV3FixtureTest` verifies the signature and parses them. Regenerate them after any wire change.

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

*Implementation (since core migration step 1): the Go core, `core/internal/identity`.*

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
| `grants-v1` | Active grants exist for this particular phone (§4) |
| `shared-v1` | Another phone of the same profile reported usage today (§5a) |

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

Since core migration step 1 the replies are signed by the Go core, so build it first:

```sh
mkdir -p /tmp/fw-core && (cd core && CGO_ENABLED=0 go build -trimpath -o /tmp/fw-core/familywardend ./cmd/familywardend)
docker run -d --name fw-fixtures --cap-add NET_ADMIN -v /tmp/fw-core:/core:ro \
  -v "$PWD:/src:ro" openwrt/rootfs:x86-64-25.12.4 sleep infinity
docker exec fw-fixtures sh /src/scripts/test-integration.sh --inside
docker exec -e FW_TEST_FIXTURES=1 fw-fixtures sh /src/test/android/generate-v3-fixtures.sh
for file in v3-payload.json v3-payload.sig v3-router.pub v3-refused.json v3-refused.sig; do
  docker cp "fw-fixtures:/tmp/$file" "android/app/src/test/resources/$file"
done
docker rm -f fw-fixtures
```

JVM fixtures verify both the exact signed accepted policy and the signed refusal.

## 2c. Current signed roles and information fields

Approved v3 bindings contain `device_id, profile_id, profile_name, captain,
administrator, can_self_remove, approves[]`. Each approval entry has `profile_id,
profile_name`. Scope is current: excludes nonexistent profiles, the phone's own
profile and a pact-bound captain. Administrator/captain flags do not imply parent
permission. Parent-only Android pairing is a local operating mode, not authority.

Optional informational fields include scoped `requests[]`, `new_apps[]`, today's
`usage[]`, the child's latest `request` and parent device keys/names in `parents[]`.
Names identify devices, not necessarily people. There is no parent rule-editing or
captain self-service mutation permission in this wire version.

Optional `generation` carries the household epoch/counter and restore lineage.
Once observed, it must not disappear or regress in later policy replies; see
[migration-safety.md](migration-safety.md). Reads in §5c are information only and
never replace the last accepted policy or advance its generation witness.

## 2. Future policy sketch — NOT the implemented v3 wire format

This retained P1 proposal contains unimplemented fields such as numeric revisions,
categories, budgets and essential-package declarations. Use §2a/2b/2c and the real
signed fixtures for implementation, not this example or its proposed precedence.

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

## 3. LAN extra-time requests and decisions (implemented: Go and Android)

Transport: `POST /cgi-bin/app?op=request` or `?op=decision` on the existing phone
port 7700. Body: one base64 usign signature line, newline, then exact UTF-8 JSON bytes
(maximum whole body 16 KiB). No canonical JSON is needed between Go and Kotlin.
Sign `familywarden-request-v1\n` or `familywarden-decision-v1\n` plus those exact bytes.

Child upload:

```json
{"type":"request","v":1,"request_id":"a67cc491-91c8-4d42-998c-a8b5d189d561","device":"fw-alex","profile":"alex","target":"com.roblox.client","minutes":30,"label":"Roblox","created":1792540012}
```

- `target` is an Android package name (maximum 255 bytes) or `"total"` for screen time.
  This replaces the draft request's `package` field; there is no released client for it.
- IDs are 8–64 ASCII letters/digits/hyphens; hash and identity, not UUID formatting,
  protect retries. `minutes` is an integer 1–240.
- Optional `label` (64 code points) and `note` (140) are untrusted display text; never
  render as HTML. All fields use exact case; duplicates, nulls, missing required fields,
  unknown fields, invalid UTF-8 and trailing data are refused.
- One open request per **profile**. Expiry is `min(created, received) + 1 hour`; transport
  delays cannot reset it. Timestamps over five minutes in the future are refused.
- Exact-byte retries return recorded status; reusing an ID with different bytes is
  `replay`. No product rate limit; bounded storage can return `unavailable` (see below).
- Requests require app control and an approved device in the named profile; never for
  a pact-bound captain.

Parent upload:

```json
{"type":"decision","v":1,"request_id":"a67cc491-91c8-4d42-998c-a8b5d189d561","request_sha256":"<64 lowercase hex>","device":"fw-parent","approve":true,"minutes":20,"created":1792540100}
```

The hash covers the child's **exact JSON bytes**, not its signature or a reserialization.
It is mandatory, even for denial. Parent signatures do not directly create standalone
grants: the router rechecks current enrollment/key/profile, effective approval scope,
pact status, requester identity and app-control state before persisting a decision.
The same parser bounds/strictness apply. Decision timestamps use the request TTL and
five-minute future skew; an expired request cannot be answered. Denial uses explicit
`approve:false, minutes:0`; approval may grant 1 through the requested minutes.

**First answer wins.** Concurrent parents cannot both decide. An identical retry by the
same issuer gets the recorded decision and the original deadline, never more minutes.
Changed answers return `already_answered`. The local administrator may answer through
`ubus call familywarden request_answer '{"request_id":"…","approve":true,"minutes":20}'`;
the root-only API uses issuer `router`, not a parent scope. Pacts still forbid it.

**Replies.** Usign signature line + exact router-signed JSON, containing `v:1`,
`type:"request_reply"|"decision_reply"`, `device`, `server_time` and
`message_sha256` (hash of the exact upload JSON). Verify the pinned key **and** that
hash/type/device before accepting a reply. Success adds `request_id, state`;
approval adds `granted, not_after`, and `home_internet` when the ticket is attempted.
Authenticated refusals have `error`; malformed/unverifiable uploads have bare HTTP
400/403. Core/platform failure is 500; core down is 503. Neither grants authority.

Signed v3 sync adds optional `request` (latest profile request, including status/answer)
for the child and `requests[]` (pending, in-scope requests with exact hash, profile,
target, minutes, label/note and expiry) for the parent. Old clients may ignore these
display-only fields. `ubus call familywarden requests` lists retained decisions for the
administrator. Android has the scoped Requests inbox and approval/denial flow;
retained administrator history is not automatically exposed to parent phones.

## 4. Router-policy grants (implemented: Go and Android)

An approved request becomes a durable grant **for the requesting device and enrolled
key only**. This deliberately prevents copying one allocation to every phone before
shared consumption/leases exist. It does not implement the shared child budget.

```json
"apps": {"rev":"<16 hex>","grants":[
  {"grant_id":"<32 hex>","device_id":"fw-alex","target":"com.roblox.client",
   "minutes":20,"not_before":1792540100,"not_after":1792547300}
]}
```

- Authority is the enclosing router-signed policy, not a separately forwarded parent
  grant. Effective grants change the **per-device** app revision and require
  `grants-v1`; an older client receives signed `unsupported_capabilities`, never a
  weaker policy. Released Android builds before 0.11 do not advertise this feature.
- Lifetime: two hours **after approval**, not delivery or midnight. Expired grants are
  finalized durably when observed. The ledger's durable time floor rejects clock
  rollback rather than reviving expired/pruned authority; it does not age out an
  offline phone's last trusted policy.
- Parent revocation prevents new decisions; an already router-issued grant retains
  its fixed expiry. Moving/deleting/rekeying the **requesting** device makes its old
  grants ineligible. Pacts and disabled app control omit grants entirely.
- Delivery is a signed `applied` acknowledgement of the grant-bearing revision by the
  **requesting phone**. `requests[].delivered` is RAM-only and clears on daemon restart;
  it is not proof of consumption, connectivity or continuous enforcement.
- Home internet: the OpenWrt adapter ensures a profile ticket until `not_after`, never
  shortening a longer ticket, overriding a manual block, or opening for a bound captain.
  The app grant remains recorded if firewall application fails; identical decision
  retries and child sync retry the **same deadline**. After reboot, sync restores lost
  RAM tickets. The internet ticket covers the profile, not just one app.
- One versioned `requests.json` stores requests/answers/grants. There is no second
  experimental request store/parser. Retain entries until 24 hours after their end,
  at most 256 entries; at most 32 simultaneously active grants per requesting device.
  These are technical storage/payload bounds, not a per-child product rate limit.
  Full/corrupt/unwritable state refuses new authority; pruning recovers capacity.
  File reads are bounded to 1 MiB; signed replies to 64 KiB.

**Android client (implemented 2026-10-25).**

- Parsing: `apps.grants` is accepted only in v3 with `grants-v1` in `requires`, for this
  device id, with exact 7200 s lifetime, active at the signed `server_time`; never for a
  pact-bound captain. Strict request/parent-list/child-status parsers bound every field.
- Consumption lives in the **same atomic checkpoint** as the usage cursor
  (`UsageJournal`/`GrantLedger`): restart, process death or repeated delivery of the same
  grant cannot refill spent minutes. Grant terms that change under the same id are refused.
- Spending starts only when ordinary rules would deny use: an app grant covers that app's
  exhausted limit, daily total and downtime; a `total` grant extends the daily total and
  downtime but never an exhausted per-app limit. Blocked, unapproved and safety-excluded
  apps never spend grants. PiP/split-screen counts once against `total`. Overlapping grants
  spend earliest expiry first.
- Time authority: request-bound server time plus `elapsedRealtime` within one boot
  (`PolicyClock`), upper-bounded by the full request round trip, never moved backwards by
  a faster later reply. Reboots, clock jumps and usage gaps make extra time unavailable
  (not zero use); a gap is charged as possible use up to each grant's overlap. A monotonic
  high-water mark per router/key/device/profile scope prevents rollback revival.
- An exact alarm closes the grant window while the screen is off. Formerly granted apps
  are re-paused when usage becomes unknown rather than left running.
- Uploads: one durable outbox entry of exact bytes; retry after an uncertain response or
  process death resends the same signed action; only a reply verified against the pinned
  key, device, type and `message_sha256` clears it. Dismissing it locally cancels nothing.
- Parent-only phones need no Device Owner: they sign `caps=-`, accept only policies with no
  apps, DNS, captain role or pact, receive scoped `requests[]` via LAN sync (30 s while open,
  WorkManager otherwise) and post decisions. Disconnecting deletes the signing key.
- Verified on Android 15 (`scripts/test-requests-emulator.sh`): exhausted 1-minute limit,
  child UI request, dropped reply + process death + exact retry (one ledger entry), approval,
  unpause only after policy sync, persisted spending across process death, re-pause at
  exactly 60 s spent, repeated sync without refill, authorized removal, non-owner parent
  pairing, scoped approval and decline.

Authenticated grant-consumption upload is implemented through §5. Still open:
bounded multi-phone allocation, off-home delivery/notifications (P9), and remaining
physical-device validation. Cached offsets alone do not bound offline overspend.

**Future (P9, not an accepted wire format):** directly delivered parent-signed grants,
offline issuance and encrypted notification/relay transport. They must reuse the same
scope, request binding, device-bound consumption and replay rules; do not expose a second
grant verifier or ledger as a shortcut. Shared multi-device spending needs its own
bounded allocation/consumption design before one grant can be applied on several phones.

## 5. Usage reports (child/adult phone → router) — implemented 2026-10-25

Transport: `POST /cgi-bin/app?op=usage`, signed `familywarden-usage-v1\n` + exact JSON,
same envelope and verified reply (`type:"usage_reply"`, `message_sha256`) as §3.

```json
{"type":"usage","v":1,"device":"fw-alex","profile":"alex","day":"2026-10-21","seq":17,
 "total":4920,"apps":{"com.roblox.client":1260},"labels":{"com.roblox.client":"Roblox"},
 "grants_used":{"<grant_id>":600},"incomplete":false}
```

- **Authenticated source, not guaranteed accuracy.** Reports cannot authorize grants
  or loosen standing policy. The core derives signed other-phone offsets from them
  (§5a); those offsets can only tighten local decisions. Shared allocation/leases
  bounding offline overspend are not implemented.
- **What counts (D5):** whole seconds with the screen on and unlocked, app visible
  (PiP included). `total` is the union (PiP counts once), not the sum of `apps`.
  Always-allowed/essential apps are excluded from `total`.
- **Day (D8):** router-local midnight; accepted for today and the previous 29 days only.
- **Cumulative per (phone key, day):** stored under `device@<16 hex of key fingerprint>`,
  so a re-enrolled phone starts a separate record. Only a higher `seq` can change the
  record; `total`, every app and every grant counter may never decrease
  (`usage_regressed`). Identical values are not rewritten (flash wear). Bounds: 25 h per
  counter, 256 apps, 32 grants (each ≤ its granted minutes, and only grants issued to
  this phone and key: `unknown_grant`), 16 phones per profile and day, 64-character labels.
- Strict decoding as §3 (exact keys, no duplicates/nulls/trailing data). Other refusals:
  `wrong_profile`, `app_control_off`, `day_out_of_range`, `unavailable`.
- **Android** sends today's and yesterday's reports after a successful sync, at most once
  a minute; no outbox (the next cumulative report supersedes a lost one). `incomplete`
  marks days with usage gaps (restart, clock change, revoked access).
- **Readers:** v3 sync for phones with `approves` scope adds `usage[]` (today per scoped
  profile with app control: total, phones, top 5 apps; never for pact-bound profiles).
  `ubus call familywarden usage` returns today per phone and app plus 7 daily totals.
  `requests[].used` shows reported grant use. Totals are summed across phones.
- **Administrator weekly coverage (core 0.10.0):** each `usage.profiles[].week[]`
  item adds `reported`, true when at least one stored device report exists for that
  profile/date. A real zero report is `{day,total:0,reported:true}`; absence has
  `reported:false`. This describes presence, not complete coverage or accurate
  counters. Older replies without the field have unknown coverage, not measured zero.
  Retention, counters, grants, revisions and signed phone payloads are unchanged.

## 5a. Shared per-child budget (D4) — implemented 2026-10-25

A v3 policy for an app-controlled profile adds top-level
`"shared": {"day":"2026-10-21","total":<s>,"apps":{"<package>":<s>}}`: today's
(router-local) totals of the profile's **other** phone keys, summed from their usage
reports (§5), at most 64 apps by time. It is present only when non-zero and then
`requires` contains `shared-v1`; an app without it gets the signed refusal (§2b), never
a policy that lets each phone spend the whole budget. `shared` is not part of the app
revision, so it does not churn acknowledgements. A usage-store error yields no policy.

Android adds `shared` to its own counters for that day only, before enforcement,
warnings and grant accounting (`GrantLedger` keeps the offsets in its checkpoint), so
another phone's usage can only tighten: the tablet's minutes pause the phone, and a grant
starts to spend when the combined use exceeds the standing limit. Today shows "Includes
N min on other devices". Offline phones keep the last signed `shared` value; usage on
other phones during that time is only seen at the next sync (bounded overspend, by design
of the trusted offline policy). Verified: Go tests (own phone excluded, required feature,
fail-closed store error, LuCI upgrade status), 5 router checks, 2 JVM tests, and the
emulator step "another phone's usage pauses the app before any local use".

## 5b. New-app approval — implemented 2026-10-25

With `apps_suspend_new`, a newly installed user app is paused (A12). The child's phone
adds `"new_apps": {"<package>": "<label>"}` (≤32, labels ≤64 code points, untrusted
display text) to **today's** usage report (§5), also when today's usage is zero. The
core keeps per profile in `newapps.json`: `pending` (last reporter, refreshed at most
hourly; an app the same phone stops reporting leaves the list; shown for 7 days),
`approved` (≤256, who/when) and `dismissed`. Reports never resurrect dismissed apps.

Decisions: administrator `ubus call familywarden app_decision
'{"profile":"alex","package":"…","action":"approve|deny|revoke"}'` (`new_apps` lists
them), or a parent phone in scope via signed `POST /cgi-bin/app?op=app`,
`familywarden-app-v1\n` + `{"type":"app_decision","v":1,"device":…,"profile":…,
"package":…,"action":…,"created":…}` (strict decoding, 1 h validity, reply type
`app_reply` echoing profile/package/state). Only a reported app can be approved;
`approve` is refused for a pact-bound captain (`pact_forbids`), `deny`/`revoke` only
tighten. Approved packages are appended to the signed `apps.approved` list (never if
blocked; no new capability needed) and change the app revision, so phones re-apply and
acknowledge. Parent phones receive `new_apps[]` (scoped) in v3 sync.

## 5c. Bounded signed app-history reads — core 0.13.0-r24

`POST /cgi-bin/app?op=history`, same bounded signed transport as §3; signing domain
`familywarden-history-v1\n` plus exact JSON:

```json
{"type":"history","v":1,"device":"fw-parent","profile":"alex","day":"2027-01-15","created":1800000000,"nonce":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
```

- Exact required fields, strict decoding as §3. Nonce is 64 lowercase hex digits;
  clients generate a fresh cryptographic nonce for every query. Creation time must
  be less than five minutes old, at most five minutes ahead of router time.
- Current enrolled key and current scope are rechecked under the identity lock.
  A phone may read its own profile or an effective `approves` profile, never other
  profiles through administrator/captain flags. App control must be configured.
- One router-local calendar day only, today through the previous 29 days, including
  DST boundaries. No policy-sync payload expansion or new retention ledger.
- Signed `history_reply` echoes `device, server_time, message_sha256`, plus
  `profile_id, day, reported, total, devices[], apps[]`. Each device report has
  `name, total, incomplete, received`; apps are the 20 busiest packages with seconds
  and optional labels. At most 16 device records, including historical phone keys.
- `reported:false,total:0,devices:[],apps:[]` means missing data. A real zero report
  has `reported:true` and at least one device report. Coverage is not completeness.
  Store failure is signed `unavailable`, not an absent day.
- Verify the pinned router key, exact query digest (including nonce), device, type,
  minimum server time and requested profile/day. Fresh policy sync in the Android
  client checks current scope and generation first. It is not accepted or persisted
  as part of the read; neither historical data nor errors alter enforcement.
- Refusals: `malformed, request_expired, not_parent, app_control_off,
  day_out_of_range, unavailable`. Unknown/older endpoints and transport failures
  remain unavailable, never synthesized empty history.
- Read-only: no request ledger/outbox, ticket, native apply, app revision,
  acknowledgement, generation advance or retained-policy replacement.
- `FamilyHistoryClient.history` is ready for off-main-thread callers; the longer-range
  native history screens remain separate design/integration work.

Today's scoped `usage[].devices[]` additionally includes the same optional `received`
timestamp. Android accepts old replies without it as **unknown**, not current.
Receipt of a usage upload is neither policy application nor a live online signal.

## 6. Storage and retention (D6)

- App activity: per profile, per day, per package seconds — **30 days**, then deleted.
- Websites (**planned, not implemented**): per profile, per day, per **domain** request counts, derived daily from
  AdGuard Home's query log for the profile's `familywarden-*` client — **30 days**. Domains
  only; no URLs or content. FamilyWarden never changes AdGuard Home's global query-log
  settings; if AGH retains less than a day, website history is reported as unavailable.
- Stored by the Go core in the household (router storage; flash-write budget to be checked
  in P3/P4). Nothing leaves the household.

## 7. Authenticated errors and transport refusal

`not_enrolled`, `bad_signature`, `replay`, `wrong_profile`, `not_parent`, `request_open`,
`request_expired`, `grant_expired`, `invalid_minutes`, `unknown_package`, `pact_forbids`,
`unsupported_version`, `malformed` (see implemented sections for operation-specific
codes). Authenticated refusals are signed; unauthenticated/malformed transport may
return bare HTTP 400/403 and failures 500/503. Upload creation times allow at most
five minutes of future clock skew; grant deadlines have no extra grace period.

## 8. Compatibility

- v2 phones keep working on profiles without app control. An app-controlled profile
  refuses unsupported clients rather than returning a weaker approved policy (§2b).
- An enrolled phone that has received v3 refuses later v2/v1 replies (no downgrade), as today.
- Unknown fields are ignored. If a reply contains `"requires": ["feature", …]` that the phone
  does not implement, it keeps enforcing its last trusted policy and reports unsupported capabilities.
- All signed messages carry a type-specific prefix line, so a signature for one message type
  can never be accepted as another.

## Still open (not blocking the LAN slice)

- ~~Per-role visibility of history~~ decided A14: the child sees own app and domain history, read-only.
- Off-home transport (A2): native notifications on the parent phone decided; FCM vs UnifiedPush open — needed for P9, not for the LAN slice.
- ~~Category source and maximum grant size~~ decided A15/A16: 240 min; Android category + router override.
