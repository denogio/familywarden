# FamilyWarden design/state review

Date: 2026-10-17. Review scope: supplied HTML, UI specification, and Kotlin contract.
No new app implementation or production changes. Original references are preserved;
this review takes precedence over unsafe or unsupported claims in the draft.

## Overall assessment

The visual direction is coherent with the lighthouse identity and the earlier Mast
work. Three ordinary destinations and a conditional Ulysses destination work well.
The four independent policy/application/resolver/router facts are the right foundation.
Keeping future administration absent from the actual navigation is appropriate.

The contract is a good **design draft**, not ready to adopt unchanged as an enforcement
model. Several pure derivations accept contradictory inputs and produce reassuring copy.
Authorization and freshness also need backend design; UI flags are not permission checks.

## Reproducible checks

From `android/` with JDK 17/21:

```sh
./gradlew --no-daemon -I ../test/design-contract/init.gradle testDebugUnitTest
```

Result: **55 tests passed**: 36 existing Android tests and 19 design-review cases.
The draft and review helper are added to the unit-test source set only. Nothing in
`docs/design/` or `test/design-contract/` is packaged into the app.

These are characterization/proposal tests: they deliberately reproduce flaws in the
original draft, then demonstrate minimum proposed headline guards. Passing is evidence
that the findings are reproducible, **not** evidence that the draft is production-ready.

## Findings

### R1 — Positive headline ignores policy, lock, and hostname (must fix)

`overviewHeadline()` returns `ProtectionOn` for `DnsApplication.Applied` even if:
- no verified policy has ever been received;
- a rejected policy has no last verified predecessor;
- `locked` is false;
- Android's applied hostname differs from the verified hostname;
- resolver health is Untested or Testing.

The specification says policy participates in headline derivation, but the function
never reads it. Require verified effective policy, matching applied hostname, and
confirmed edit restriction before a positive application claim. Distinguish configured,
locked, checking, and last-test-passed. A probe never proves all filtering categories.

The opt-in `ReviewedHeadline.kt` proposal demonstrates guards, not final product copy.

### R2 — Failure is not a diagnosis (must fix)

`NetworkBlocksResolver` implies the network is responsible for any Unreachable result.
Use a neutral cause-agnostic name such as `ResolverUnavailable`. A server outage,
allowlist rejection, DNS bootstrap failure, carrier policy, and captive portal are
separate possibilities. A certificate error need not be a hostname mismatch: expiry,
untrusted chain, and clock problems must have honest copy too.

"Websites/apps won't load" is too broad: cached IPs and app-specific resolvers may
still work. Prefer "System DNS resolution is unavailable" and explain likely impact.

### R3 — Health needs endpoint/network context (must fix before network switching)

`Reachable(testedAt)` does not identify the tested hostname, policy generation,
network, or transport. A Wi-Fi result can otherwise appear healthy on mobile data,
or an old hostname's probe can overwrite a new policy's status.

Bind results to endpoint and current network/generation; discard superseded results
and show old results as historical/untested for the new context. This is **not**
age-based expiry of cached policy. A raw DoT probe, Android's configured strict mode,
and successful filtering remain distinct facts.

### R4 — Pact visibility must be scoped to this enrolled device (security gate)

The existing Mast payload describes the router's captain, not a signed individual
phone/profile authorization. The new model must carry stable device/profile IDs and
an authenticated enrollment association, not only `profileName` and a local boolean.

An active **applicable** cached pact must win over a local feature-Off preference.
Do not propagate the router's captain pact to every ordinary managed phone.
`ulyssesVisible()` also trusts feature On despite an Association.Mismatch; an
authoritative mapper and action guards must prevent that combination from granting
capabilities. Navigation can remain presentation-only.

### R5 — Removal is authorization-sensitive (unresolved product gate)

A fresh no-pact answer is **necessary but not sufficient** for managed-device removal.
The later user requirement supersedes the supplied removal boards and `Allowed` model.
No-pact does not establish the caller's administrator/self-removal authority, and
non-admin does not mean child.

Resolve D11's actor/device role matrix before implementing removal. Represent refusal
for missing authority separately from an active pact, unavailable router, and unsupported
API. Read-only/diagnostic navigation may remain available without granting an action.
Recheck authorization AND pact state at final confirmation, serialized with relevant
state changes. A dialog's `confirmedAt` is not a reusable permission token.

### R6 — Router identity and response freshness are incomplete (security gate)

The QR pins the router's public signing key; this does not authenticate a requesting
phone. Agree device enrollment and per-device request proof (D12).

A live HTTP request plus a valid signature/nondecreasing server timestamp does not
prove freshness against replay. Agree request binding/challenge and bounded, one-use
removal authorization semantics before sensitive actions (D14). Do not describe the
current Mast GET path as a finished proof of live authorization.

### R7 — Schedule is not a complete day/time model (must fix)

`List<IntRange>` has inclusive endpoints, whereas access windows are start-inclusive
and stop-exclusive. It also omits weekday, overnight windows, and timezone/day context.
A generic 24-hour bar cannot faithfully represent arbitrary weekly schedules.

For UI, supply resolved intervals for an explicitly selected local date and a text
summary, reusing tested `Policy` calculations. Do not implement a second enforcement
engine in the composable. Keep next-change calculations based on actual block changes,
not every evaluation boundary. Pact-end and release countdowns must choose the earlier
applicable end and must not claim hotspot stays blocked through an intervening allow window.

### R8 — Account readiness may be unknown (must fix copy/model)

Before enrollment, `AccountManager.getAccounts()` lists accounts **visible to the
caller**, not a guaranteed full device inventory. `accountCount: Int` has no unknown
or incomplete state and can falsely reassure users that zero visible accounts means
enrollment-ready. Use an explicit unknown/partial readiness result; final enrollment
eligibility comes from Android provisioning, not this diagnostic estimate.

"Android allows it only over USB" is inaccurate as a general statement. Describe
**this adb setup method**; Android has other managed-provisioning flows. Retain backup,
profile-data-loss, and possible-reset warnings. `.AdminReceiver` remains a placeholder
until the renamed app's manifest is implemented and verified.

### R9 — DNS setup success and cleanup are not automatic (must fix)

The setter validates DoT and can fail. A later diagnostic failure can coexist with an
already applied policy, but setter failure must never become Applied/Complete protection.
Read back global mode/host and this app's restriction after success; show setup failure
or a truthful limited completion state when Android rejects the operation.

Do not promise "Private DNS goes back to Android's default" until restoration behavior
is tested. Preserve previous owned settings and distinguish restoring a former host,
setting opportunistic mode, and exact restoration of a formerly-Off setting. Relinquishing
Device Owner does not itself prove that global Private DNS settings were restored.

### R10 — Localization and display data need ownership (implementation requirement)

Mockups remain English only. Runtime English+Danish strings, plurals, accessibility,
and locale-aware dates/times are still required. Raw `reason: String`, category names,
and `Schedule.summary` must not make localization dependent on server-supplied English.
Prefer typed error/category IDs plus local resources; retain optional redacted technical
information for diagnostics.

Treat collections passed under `@Immutable` as immutable snapshots. `List` alone does
not prevent mutation by an upstream owner. Document ownership/copy boundaries rather
than adding a new collection framework just for the design draft.

## Navigation invariants already demonstrated

- Ordinary complete setup: Overview, DNS, Settings.
- Active applicable pact overrides local feature Off for visibility.
- Finite pact and synced release expire locally at the boundary without router access.
- Future release cannot extend an already expired finite pact.
- Captain and administrator stay independent; captain eligibility does not opt in.
- Onboarding has no bottom navigation.
- Off-home router sync does not invalidate an already applied DNS policy.

These do not resolve device scoping, administrator authentication, or action authorization.

## Implementation handoff

1. Keep the original draft as reference, not as runtime source.
2. Resolve R4–R6 permission/protocol questions before enrollment/removal work.
3. Adopt a corrected state contract with tests for R1–R3 and R7–R10 before composing views.
4. Implement only approved capabilities; no fake statistics, Family tab, or unsigned roles.
5. Verify Android behavior on a disposable device before migrating the existing Device Owner.

## Local implementation follow-up

The subsequent `feature/familywarden-android` work is tracked in
[the implementation checkpoint](../../../plans/android-implementation.md). Original
supplied references and the historical 55-test result above remain unchanged.

| Finding | Local disposition | Still required before rollout |
|---------|-------------------|-------------------------------|
| R1–R2 | Guarded DNS headlines; missing policy is distinct from a confirmed v1 incompatibility; neutral failure copy | Real setter/probe integration and physical verification |
| R3 | Probe snapshots carry DNS policy, network handle and a generation; superseded snapshots are untested | Wire asynchronous probes and network lifecycle invalidation |
| R4 | Device/profile-scoped v2 consumer; global v1 is diagnostic only; applicable pact overrides feature Off | Authenticated enrollment and server-side binding protections |
| R5 | Actor roles reflect the approved pilot; final removal re-fetches; fresh authorization uses server time; typed refusal copy separates causes | Real administrator approval/one-use authorization protocol |
| R6 | Exact signed bytes, pinned key, nonce/device matching, no enrolled downgrade; redirect/size rejection | Authenticate requesting devices; production replay/freshness semantics |
| R7 | Reused evaluator, local date/zone, half-open intervals, overnight/weekdays/overlap and DST-gap/repeated-hour tests | Compare real router schedules/timezones, including DST |
| R8 | No account-count readiness inference; native owner check, provisioning/data-loss and migration cautions | Controlled real-device enrollment/migration |
| R9 | Read-back only; this build does not claim to write/restore Private DNS | Implement/test strict DNS and authorized cleanup; update confirmation copy then |
| R10 | Complete EN/DA resources, typed category/refusal labels, private diagnostics, native locale and 200% UI checks | Keep localization complete as lifecycle/backend work lands |

Later (backend 0.9.0): R5/R6 enrollment and R9 DNS writes/restoration are implemented;
see the implementation checkpoint. Startup clock integrity, before-first-
unlock behavior, same-key address repair, and exact DNS cleanup remain explicit gates.
No production AdGuard/router or existing Device Owner was changed for this work.

## Sources

- [Android AccountManager](https://developer.android.com/reference/android/accounts/AccountManager)
- [Android DevicePolicyManager](https://developer.android.com/reference/android/app/admin/DevicePolicyManager)
- [Product/DNS decisions](../../../plans/dns-filtering.md)
- [Read-only DNS findings](../../../plans/dns-preflight-report.md)
- [Review tests](../../../test/design-contract/README.md)
