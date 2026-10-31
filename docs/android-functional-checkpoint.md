# Android functional checkpoint — 2026-10-29

Local development only, on `feature/app-control`. This is not a release, production
upgrade, release-signing authorization or physical-device pilot. Core package:
`0.13.0-r25`; candidate OpenWrt packages: `dist/android-functional/`.

## Working flows

| Flow | Available now | Boundary |
|---|---|---|
| Child Today / Ask | Real local counters, app limits, Downtime, signed requests and retryable outbox | Missing/incomplete observations are not zero; requests remain 1–240 minutes |
| Child History / Settings | Recorded local days/apps, DNS/router/security/removal details | No website history or inferred cross-phone coverage |
| Parent Screen time | Scoped signed totals/apps and source receipt observations | No summary is different from a reported empty day; cached previous-day data is not headed Today |
| Parent History | Screen time → History; choose child/day, calendar and previous/next, source reports, retry/back | One profile/day per signed read, up to 30 router-local days, 20 busiest apps, at most 16 historical source reports; needs r24+ |
| Parent Requests | Ordinary non-Device-Owner approvals/denials/new-app decisions on the LAN | Only current assigned scope; approvals 1..requested, expiry two hours after approval |
| Ulysses | Calm/bound/release-requested/offline status, separate Right now section, delivered course summary, waiting period and ongoing port call | Signed optional display metadata needs r25; absent facts stay unknown; no voyage age or complete frozen-household snapshot |
| Router hand-off | Actual browser actions in Ulysses and parent Controls | Opens the host web root (sign in, choose FamilyWarden) or port-7700 sail page; requires network access and the appropriate router authentication |
| Availability | Settings / parent Controls → App status and remaining work | Separates app support, router-only actions and missing native features; never grants extra authority |

The native history view discards its loaded state when identity, selected child/day
or effective parent scope changes. Reads run off the main thread. A slow response
cannot replace another child's current view. There is no persistent history cache,
policy replacement, revision acknowledgement or new outbox in this presentation.
Errors/unsupported older routers are not rendered as empty history. Source receipt
and read times are observations, not live connectivity or confirmed enforcement.

The Ulysses summary distinguishes being bound from today's restrictions. Known
fixed-end voyages have no amendment window. App/Downtime rules cannot be tightened
arbitrarily outside the supported ongoing amendment window; DNS has its separate
rules. Displayed app allowances can include today's bonus and are not presented as
a complete immutable saved snapshot. The router retains the full course. Waiting
seconds come from the frozen pact, not mutable household defaults; missing/malformed
source values never become an assumed zero or 24-hour period. Unknown phone policy
is not shown as calm. My phone and Family remain separate.

## Still missing — not hidden behind working-looking buttons

1. **Scoped phone writes:** native parent rule/Downtime/DNS/network editors and bonus
   controls, and captain create/extend/amend flows. Go/admin implementation already
   exists, but the signed phone authorization/CAS/confirmation/retry contracts do not.
   Do not expose `/api/*` to phone credentials or smuggle administrator credentials
   into Android. Until then, use the explicitly labeled router hand-off. The installed-
   apps browser/editor design is received and reviewed
   ([review](design/android-installed-apps-review.md), [plan](../plans/android-installed-apps.md)): friendly
   names/icons/search, reported phone/time, essential-app protection and profile-wide
   scope; never ask a parent to type package IDs. Read-only browsing (I1–I3: Family ›
   Controls › Installed apps, needs core r26) works; **changing app rules from the
   phone (I4) is still missing** and the detail view says so with the router hand-off.
2. **Richer observations:** historical expected-device roster, contact events,
   historical bonus/Unlimited attribution, detailed parent delivery state, voyage
   creation time and complete frozen course. A receipt/ack cannot fill those gaps.
3. **Product features:** native Lock now/School time/Tonight, independent adult
   voyages, adult pool/categories, website history, longer history ranges and
   off-home push delivery. Existing total counting/shared offsets are not a bounded
   offline allocation ledger.
4. **Proposals, not shipping semantics:** P1 approval above the request and P2 grant
   lifetime `max(2h, approved minutes)` remain separate coordinated contract changes.
   P3 excluding app-grant intervals from the daily total requires a separate
   accounting design. This change activates none of them.
5. **Release/pilot:** stable-key release-signed upgrade and explicitly authorized
   physical phone/router tests remain open. Emulator proof is not physical hotspot,
   positive public DoT, network-transition or before-first-unlock proof.

Lost mutation replies remain **outcome unknown**, not proof that nothing changed.
Existing native request/decision retries reuse the saved exact signed operation.
Future pact/bonus/editor flows must reconcile committed state before retrying and
bind pact creation to the exact reviewed revision.

## Verification

- 216 JVM tests, zero failures/errors/skips; `lintDebug` and `assembleDebug`.
- Full Go race/vet, including the portable production-service/shared-UI proof.
- Fresh r25 package build, syntax checks and APK installation/removal smoke tests.
- All three focused Android 15 UI scripts pass: voyage/history/capability states,
  requests and Screen time. EN light/dark and representative Danish dark 200% flows;
  icon-only accessible selected navigation (labelled since UI6), scope revocation and back navigation.
  These are fictional debug previews, not an exhaustive state/theme/device matrix.
- 36 native request checks against fresh r25 packages, including the actual Android
  history client/UI reading the core's signed response and receipts without Device
  Owner, plus revoked-scope refusal through CGI.
- 98 Mast Device Owner checks pass, including a second focused milestone whose initial
  ongoing pact ends its amendment window at **24:00**. That valid metadata is accepted
  without changing policy evaluation; the midnight display boundary has JVM/service
  and native-preview coverage too.
- Browser hand-off construction has numeric/DNS/IPv6/injection regression coverage.
  It uses the generic host web root, not a LuCI path embedded in Android.

An earlier reused preview VM stopped dispatching input and reported no focused
window. A fresh isolated VM passed all three UI scripts. The UI harness also needed
scrolling to the availability entry at 200% text. No production ANR or general
network/focus root-cause fix is claimed. Runtime logs/screenshots are local `/tmp`
artifacts, never private household state checked into the repository.

Design corrections: [export-six review](design/android-design6-review.md).
Boundaries and API inventory: [API readiness](android-api-readiness.md).
Wire: [app-control protocol](../plans/app-control-protocol.md).
