# Android export 6 — review

Reviewed 2026-10-29 against local source at `6f2e526`.
Source: `FamilyWarden (6).html`, SHA-256
`289ccde974a70d6dd1140164a3b30e78ac81e3dd2546357f3b983d9d7f584636`.

Scope: rendered the local export's 18 frames offline, extracted their text, and
visually reviewed the added/replaced specification, capability, Ulysses, adult pool
and parent boards in their light/dark/large-text examples. This is **not** access to
the original hosted design canvas or a native Android accessibility/runtime test.
At this review-only checkpoint the source export, web prototype, contracts and
application code were not changed.

## Assessment

Keep the visual direction: voyage versus Right now, calm copy, My phone/Family
separation, icon-only navigation, retained drafts, and missing/zero/incomplete history
are good foundations. Do not implement the export literally yet: several availability
labels and behavioral promises contradict the current contracts.

## Corrections before implementation

### 1. A lost response is not proof that nothing changed (high)

Set sail says: "The server didn't answer. Nothing changed: you're not bound."
Rule saving, Bonus and Lock repeat the same assumption. The server can commit and
lose its response. In particular, blindly retrying an additive bonus can duplicate it.

Add an **outcome unknown / checking saved state** state, distinct from an explicit
server refusal. Preserve inputs and the operation identity; reconcile status before
retrying. After submitting a pact, do not promise that Stay in harbour undoes it.
Bind the pact to the exact reviewed configuration revision; if it changed since the
review, show the new course rather than freezing unreviewed rules.

### 2. Rebuild the capability matrix around Android integration (high)

The matrix inconsistently uses Available now for backend behavior, finished native
screens, and data that is not exposed to parent phones. Correct at least these rows:

| Export claim | Fact at the review checkpoint |
|---|---|
| Requests inbox / approve less needs integration | Native LAN requests, approvals and denials already work |
| Daily limits, Unlimited and Bonus need core/phone support | Those rules and bonus already work in core/phone; native parent writers need a scoped contract |
| Parent Controls, DNS/network editors and delivery details are available | Core/admin capabilities exist; signed parent editing and detailed device-state APIs are not integrated |
| CAS needs a new revision check | Root configuration API already has CAS/confirmation; parent integration and author attribution are separate |
| Release state needs to be added to policy | `pact.release_at` and the existing phone countdown already exist; requested-at and frozen waiting-period metadata are missing |
| Recorded-days History screens are available | r24 signed single-day history reads and Android client exist; the complete native list/day screens are not implemented |
| Voyage age, port call and full frozen-course metadata are available | Current phone pact fields are active/type/until/release_at, not creation time, window or frozen delay |

Categories and the adult pool correctly remain future concepts. Distinguish backend
support from native integration, and label mixed screens per capability rather than
calling the whole screen shipped.

### 3. Network override copy changes product rules (high)

Controls says a manual block ends when the next schedule opens. It does not: a manual
block persists until another action/reset (or loss of volatile native state).
A manual timed ticket can replace that block; after expiry it returns to the normal
effective policy, not necessarily to the previous manual block. Grant-created tickets
have different eligibility: they never override an explicit manual block.

Use "blocked until changed/reset" and "ticket expires back to the normal rules".
Do not guarantee that 18:12 is blocked when the example's schedule allows until 20:00.
Reference: `configuration/network.go`, `decideNetwork`.

### 4. Frozen-course permissions are overstated (high)

"You can still tighten" is not a blanket permission for app-limit/downtime edits
outside a pact amendment window. Those configuration fields remain frozen; specific
DNS tightening has its own supported path. Current amendment windows apply to
**ongoing** pacts, not fixed-end pacts, yet the fixed-end examples offer port calls.

Keep unsupported actions integration-dependent. The current core has one captain
profile, including its bound phone identities, not independent per-adult/per-phone
voyages. Do not imply that ordinary phone self-service already changes that scope.
Reference: `configuration/model.go`, `InWindow` and `ValidateEdit`.

### 5. Delivery data is not continuous enforcement evidence (medium)

The tablet example receives revision 43 at 17:21 even though it was saved at 17:36.
Fix the example chronology. More importantly, an older acknowledged revision is not
proof that the phone is still enforcing exactly that revision after an apply/counting
failure. Say "last acknowledged revision" and "new revision not confirmed" where
that is all the API knows.

The core's signed acknowledgement is observed at a fetch; it is not a separately
reported exact application timestamp. Do not synthesize per-revision received/applied
times from last contact. Keep required missing timestamps nullable in the state model.

### 6. History includes facts not carried by its API (medium)

The new read supplies day totals, reports (name/seconds/incomplete/received), and
up to 20 app counters. It does not supply a historical expected-device roster,
previous/next contact events, bonus attribution, or historical Unlimited classification.
Remove or mark those annotations as requiring extra data. No report is not evidence
that a phone was unused; a partial reported zero still has unknown sibling coverage.
Current retention is 30 days; the future long-range boards must remain separate.
Reference: `identity/history.go`, `UsageUpload.kt`, `FamilyHistoryClient.kt`.

### 7. State specification and large-text variants need reconciliation (medium)

- Requests allow **1–240** minutes; custom approval cannot be `5..asked` when asked is
  1–4. Keep the current `1..asked` contract.
- Revisions are opaque strings, not sequential `Int` revisions 43/44.
- `alwaysShowLabel = false` is not the icon-only solution: it still shows a
  `NavigationBarItem` label when selected. Omit the optional `label` and retain
  localized icon semantics, as the app already does. The parameter belongs to the
  item, not `NavigationBar`. Checked against the Material 3 API via Context7
  `/websites/composables_jetpack-compose_androidx_compose_material3_material3`
  ([reference](https://composables.com/jetpack-compose/androidx.compose.material3/material3/components/NavigationBarItem)).
- `receivedAt` can be unknown on older replies; history currently does not provide
  stable source phone IDs, only reported device names.
- Equal downtime endpoints mean a full day in the current evaluator, not an invalid
  empty window. Offer an explicit full-day representation or flag a proposed change.
- The 200% Ulysses confirmation still has Back and Bind side by side (same vertical
  position, measured widths 146/204), despite its caption/spec promising two full-width
  actions. The 200% conflict screen does stack them correctly.
- There are representative 200% examples, not every state in every theme: e.g. the
  set-sail board has three large-text states for seven normal light/dark states.
  Do not describe this as exhaustive font/theme coverage or native accessibility proof.

## P1–P3 and the web prototype

Recommendation, **not a new product decision**: preserve P1/P2 in the web prototype
as explicitly marked target/proposed behavior rather than deleting prior design
intent merely because implementation is behind. The shipping web and Android UI
must continue to obey the implemented contract until coordinated changes land.

- **P1 — approval above the requested amount:** a real contract change. Today both
  Go and Android enforce `1..requested`; the absolute maximum remains 240 minutes.
- **P2 — lifetime `max(2h, approved minutes)`:** also a real contract change. Today
  Go fixes 7200 seconds and Android validates that lifetime. P2 is meaningful even
  without P1: current requests may already ask for 121–240 minutes. Do not label
  it useful only together with P1.
- **P3 — app-grant intervals excluded from the daily total:** a separate accounting
  change, not a missing shared-budget prerequisite. Total counting and cached
  cross-phone offsets already exist. `UsageCounter` currently counts ordinary app
  visibility toward the daily union even while a grant permits use. Do not silently
  subtract it or retroactively lower durable counters. PiP/other visible apps,
  consumption, uploads, history and old-client compatibility need defined semantics.

The 2026-10-25 review records the web's expanded approval controls, extended-window
proposal and extra-time tooltip as design intent, while explicitly reserving contract
reconciliation. It is not evidence that those semantics shipped or were silently
approved for activation. See `control-prototype-review.md`, current
`plans/app-control-protocol.md` and `docs/android-api-readiness.md`.

## Implementation follow-up (2026-10-29)

The [functional Android checkpoint](../android-functional-checkpoint.md) implements
Ulysses status/Right now, signed scoped day history, truthful source/unknown/error
states, real router hand-offs and explicit capability disclosure. Optional core r25
pact metadata supplies the frozen waiting period and ongoing amendment window.
Native parent/captain writers, detailed delivery observations and full frozen-course
metadata remain gaps, not working-looking buttons. P1/P2/P3 behavior is unchanged.
This follow-up corrects the native implementation and its documentation; it does not
claim edits to the unavailable hosted canvas or to the archived HTML export.

No runtime tests were needed for the original review-only document. Rendering and DOM checks
were local/offline; screenshot/text artifacts are in `/tmp/fw-android-design6/`.
