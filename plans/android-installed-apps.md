# Native parent installed apps and rule editing

Status: **I1–I4 implemented and emulator-verified locally (core r27/backend r18)**.
Authorized continuation: 2026-10-29, `feature/app-control`. Finish the existing
Ulysses/history slice first (`fc3fa89`, core r25); then implement this flow in bounded
slices. No push, release signing, production router/phone access or physical pilot.

Design: local **FamilyWarden Android · installed apps**, 27 phone mockups.
Source/reconciliation: [design review](../docs/design/android-installed-apps-review.md).
Current implementation inventory: [API readiness](../docs/android-api-readiness.md).

## Product and UX boundaries

- Family → existing child selector → Controls → Installed apps → app details.
- Browse/search friendly reported names, with stable icon/letter tiles and current
  rule/filter labels when the core actually supplies them. No package-name entry.
- Each row identifies reporting phones; source receipt times and partial/missing/
  stale inventory remain visible. No report is not an empty installed-app list.
- Rules belong to the child's profile and apply across its phones. Protected apps
  are identified per phone; never make messaging unconditionally essential.
- Four standing choices: Allow, Daily limit, Always allowed, Block. Allow remains
  subject to total limits and Downtime. Daily limit accepts the existing 0..1440 range;
  do not convert zero quota to Block. Preserve new-app approval and bonus semantics.
- Package identity remains internal; handle duplicate labels and unknown names honestly.
  Initials are acceptable icons; no new remote asset service or privacy dependency.
- EN/DA resources, fixed-height scrolling at 200% text, accessible 48 dp targets,
  retained drafts and icon-only navigation are acceptance requirements, not mockup proof.

## Source-backed gap

`AppControl.kt` already scans packages for enforcement and publishes the installed
set locally. Essential roles come from the same enforcer, augmented with non-launchable
system components. `UsageUpload.kt` sends counters/labels for used apps and a bounded
waiting-for-approval list. `identity/usage.go` and `newapps.go` therefore cannot answer
which apps are installed on every current phone. Do not reuse a usage list as inventory.

I4 adds an explicitly scoped standing-rule mutation. Root/admin configuration and native
new-app decisions do not silently grant a generic phone editor.
Detailed per-phone received/applied timestamps are also not available to this flow.

## Ordered implementation

### I1 — Inventory facts and safe persistence

- [x] Versioned snapshot per enrolled phone key/profile: package, sanitized label,
      system and essential observations; 512 items / 60 KiB; `installed` > sent = partial.
- [x] Reuses `AppEnforcer.essentialPackages()` and PackageManager after successful sync;
      no new polling collector. Sends on change, else every six hours for freshness.
- [x] Dedicated signed `inventory` op (protocol §5d): own profile only, strict fields,
      persisted monotonic seq, separate from usage uploads.
- [x] Latest snapshot only, presence-witnessed, no generation advance; re-key or
      reassignment removes the device's older snapshot.
      ponytail: files of deleted devices remain until that device id reports again;
      I2 readers filter by the current registry. Add pruning if enrollment churn matters.
      Schema preflight is not extended: an invalid file makes reads `unavailable`,
      never authority. Backup includes the directory by walking the core data dir.

**Accept:** unused installed apps appear in real reports; an old/replayed snapshot
cannot replace newer facts; empty/partial/missing/error differ; no policy/ack/outbox,
usage counter, grant or generation change is caused by observational reporting.

### I2 — Bounded scoped parent reads

- [x] Signed `installed` read (protocol §5e): current key + effective scope per request,
      current registry phones only, pact-bound captain protected.
- [x] Package-cursor paging (~48 KiB pages) with a snapshot id; per-source indices for
      reported/system/essential; newest label wins. Missing phones stay `reported:false`.
- [x] Rule per app from the same compiler plus core new-app approvals, with `apps_rev`;
      configured-but-unreported apps carry an empty source list.
- [x] `FamilyAppsClient.apps` (off main thread): fresh policy preview for scope/generation,
      strict parsing, page joining, one restart on snapshot change, typed refusals.
- Found while testing: the identity upload limit was 16 KiB for every operation, so a
  real large inventory would have been refused. `inventory` now has its own 64 KiB limit
  with a >16 KiB regression test.

**Accept:** wrong key/profile, revoked parent, own-profile spoofing, protected captain,
malformed/oversized/replayed replies and store failures fail closed. A missing source
never becomes a fabricated zero-app report. No root API or credential forwarding.

### I3 — Native browse/search/details

- [x] Wire the real signed data into existing Family child selection and Controls;
      no redundant choose-child step and no extra bottom tab.
- [x] Implement names/tiles, search, available rule filters, sources, missing/stale/
      partial/error/retry states and protected-app details. Cancel/discard stale reads
      immediately on child, household identity or effective scope changes.
- [x] Keep standing-rule mutations explicitly unavailable until I4 is integrated;
      working native new-app approvals remain in Requests meanwhile.

**Accept:** no package typing, fake inventory or working-looking unsupported Save.
Verify EN/DA, 200%, light/dark, duplicate names, empty/partial snapshots, app removal,
mode switching, source changes and revocation in fixed-height native UI tests.

### I4 — Explicit scoped standing-rule writes

- [x] Specify the signed phone operation and current child-edit authority explicitly.
      Whitelist one profile/package/mode/quota delta; never accept a generic administrator
      method, arbitrary configuration document or client-supplied privilege.
- [x] Recheck current key/scope and opaque CAS revision at the commit boundary, using
      the existing Go configuration validator/compiler/pact checks and lock order.
      No self/adult/captain escalation via parent mode or administrator flags.
- [x] Define atomic transitions between blocked, limited, approved and always-allowed
      representations. Clear conflicting fields for this target only and preserve all
      unrelated rules. Local essential protections remain unconditional.
- [x] Define retained signed operation identity, expiry and lost-response reconciliation
      before UI Save. A desired-state setter with CAS may reconcile to matching current
      state without falsely attributing another parent's change to this operation; do not
      create an additive retry or invent a delivery receipt.
- [x] Integrate draft/save/refusal/conflict/outcome-unknown states. Keep pending operations
      separate from editable drafts; leaving or changing child cannot silently discard an
      unresolved write. Show saved policy versus last acknowledgement honestly.

**Accept:** ordinary non-Device-Owner parents can change only assigned children's rules;
child/new-key/revoked/stale-revision and pact targets are refused. Test simultaneous
parents, transport loss after commit, exact retry, restart and delayed phone sync.
Read-only collection and the portable recording adapter retain their existing guarantees.

## Explicitly later

**Allowed hours per app stays future**, including editors, next-opening-time claims,
precedence/capability negotiation and child presentation. Also out of this slice:
P1/P2 grant changes, P3 accounting, adult pools/categories, website history, independent
adult voyages and off-home push. No new delivery/apply timestamps are inferred.

## I1 evidence (2026-10-29)

Go store tests (monotonic seq, re-key/reassignment, presence loss, no generation
advance, bounds/sorting), identity tests (own-profile only, foreign signature 403,
unchanged signed policy, label sanitizing), CGI forwarding; Android report contract
and partial/size tests; 38/38 native P8 checks with r26 packages, including an
emulator snapshot with unused apps and essential roles stored by the router.

## I3 evidence (2026-10-29)

Family › Controls › Installed apps reuses the child pill (no extra tab or picker) and reads
via `FamilyAppsClient` off the main thread; state is keyed by child, household/phone
identity and effective scope, so a change discards the old list. Fixed-height emulator
checks (`scripts/test-functional-android-ui.sh`): EN light at 200%, DA dark at 200%,
duplicate labels show the package, label-less apps show it once, stale and missing
sources are said, Blocked/All filters, search, detail with the per-phone report time
and essential marking, no Save, and a second child with no list (no Alex data leaks).
Native (`scripts/test-requests-emulator.sh`, r26/r17, 42 PASS): a non-owner parent
reads a real signed partial snapshot ("sent 2 of 3"), a rule for an unreported app
shows "Not reported", and the detail opens without Device Owner.

Not covered by a UI test yet: live revocation while the screen is open (the client maps
`not_parent` to the scope-changed state; the core refusal is tested natively for history
and in Go for `installed`), and app removal/source change between two reads.

## Earlier evidence

The design review extracted all text and rendered representative light/dark/large-text
mockups offline; it did not change the export. The preceding Ulysses/history checkpoint
passes 216 JVM checks, Go race/vet/portable UI, package smoke, three focused UI suites,
36 native requests and 98 Mast checks. Those results do **not** validate I1–I4: each
new slice must add its own runnable regression before its checkbox can be completed.

## I4 implementation evidence (2026-10-29)

Protocol §5f specifies bounded `rule`/read-only `rule_status`, a ten-minute retained
signed intent and desired-state (not receipt) reconciliation. Writes are available
only after an administrator explicitly activates core configuration; legacy refusal
never imports or advances generation. Existing effective `approves`, compiler,
`ValidateEdit`, pact and configuration transaction remain the authority. Package
releases: core r27, backend r18. No LuCI rule editor or allowed-hours work is included.

Android keeps drafts separately from a single durable exact signed operation, with
Draft/Saving/Saved/Refused/Conflict/Outcome unknown and current-state match/expiry copy
in EN/DA. It checks status before retry, preserves unresolved writes across app/child
navigation and restart, and does not invent phone delivery/apply acknowledgements.

Focused production-composition Go tests cover four modes (including zero quota),
exact retries, restart/lost-response reconciliation, two simultaneous parents,
read-only status/generation, scope revocation, self, old key, pact targets, malformed
inputs, legacy refusal/ownership and post-persistence projection failure/recovery.
JVM tests cover signed-byte retention, bounds, strict response binding, refusal vs
unknown, and status-before-exact-retry. CGI forwarding and native test scripts include
the new operations; the native fixture explicitly imports through the administrator
path before the ordinary parent's real Save.

Final validation: full Go race suite, vet/gofmt, syntax/integration and fresh-package
smoke pass. Android: 228 JVM tests, lint and debug assembly pass. Fixed-height native
UI checks pass in EN light and DA dark at 200% text. The full native requests suite
passes 46 checks with rebuilt core r27/backend r18/LuCI r8 packages, including an
ordinary non-Device-Owner parent's real signed Save, zero quota, CAS/status/exact retry
and scope revocation. Saved policy is explicitly not a phone acknowledgement.
Remaining UI gaps from I3 (live scope revocation while open, source changes/removal)
are not claimed as covered. No release signing, push or physical deployment.

### I4 review corrections

All five verified findings addressed: explicit unpair/logout clears the old pairing's
local standing-rule operation (not an already committed router change); preflight
failures say Save was not sent rather than inventing a retained unknown operation;
draft help follows its selected mode; obsolete read-only strings were removed in EN/DA.

Content-hash CAS alone did not prevent ABA. §5f now specifies a restart-safe 64-entry
committed-intent journal in the same atomic configuration document, bound to current
key/device identity, nonce and all exact signed request bytes. A committed intent can
never become retryable or commit again after another parent's revert. Different bytes
with its nonce are refused; capacity fails closed. Root edits/recovery preserve the
journal; expired entries are pruned only on writes, with a persisted time floor against
clock rollback. Status performs no journal pruning or configuration refresh writes.
Regression coverage includes ABA after root revert/restart, current scope on replay,
nonce substitution, capacity/expiry and generation stability, real Store unpair/logout,
preflight classification and per-mode draft help.

The integrated native run exposed sparse OpenWrt configuration-transaction facts.
Writes now compile scope/pact from the authoritative core document, while phone reads
retain runtime presence/drift checks. A sparse-facts regression covers this boundary
and pact refusal. Final independent review found no blockers. An intermediate emulator
ADB timeout was resolved by removing the retained diagnostic emulator and rerunning
the full suite successfully.
