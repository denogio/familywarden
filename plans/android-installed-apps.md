# Native parent installed apps and rule editing

Status: **I1 implemented locally (core r26); I2–I4 open**.
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

No parent-signed standing-rule mutation exists. Root/admin configuration and native
new-app decisions already work, but neither silently grants a generic phone editor.
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

- [ ] Expose current scoped phone roster and latest per-source snapshots through a
      dedicated signed read. Recheck enrolled key and effective parent scope for every
      request; remove re-keyed/reassigned/deleted sources and protect a bound captain.
- [ ] Define bounded retrieval/pagination before claiming complete large inventories.
      Aggregate by package, retaining each reporting source/receipt and conflicting
      labels or essential observations. Do not invent completeness across missing phones.
- [ ] Supply core-derived standing rule state separately from reported installation,
      reusing the existing compiler and approval ledger; include an opaque revision.
      Configured-but-unreported apps are not described as currently installed.
- [ ] Add a typed off-main-thread Android client using existing exact signing/transport
      and fresh scope/generation checks. Informational reads never replace trusted policy.

**Accept:** wrong key/profile, revoked parent, own-profile spoofing, protected captain,
malformed/oversized/replayed replies and store failures fail closed. A missing source
never becomes a fabricated zero-app report. No root API or credential forwarding.

### I3 — Native browse/search/details

- [ ] Wire the real signed data into existing Family child selection and Controls;
      no redundant choose-child step and no extra bottom tab.
- [ ] Implement names/tiles, search, available rule filters, sources, missing/stale/
      partial/error/retry states and protected-app details. Cancel/discard stale reads
      immediately on child, household identity or effective scope changes.
- [ ] Keep standing-rule mutations explicitly unavailable until I4 is integrated;
      working native new-app approvals remain in Requests meanwhile.

**Accept:** no package typing, fake inventory or working-looking unsupported Save.
Verify EN/DA, 200%, light/dark, duplicate names, empty/partial snapshots, app removal,
mode switching, source changes and revocation in fixed-height native UI tests.

### I4 — Explicit scoped standing-rule writes

- [ ] Specify the signed phone operation and current child-edit authority explicitly.
      Whitelist one profile/package/mode/quota delta; never accept a generic administrator
      method, arbitrary configuration document or client-supplied privilege.
- [ ] Recheck current key/scope and opaque CAS revision at the commit boundary, using
      the existing Go configuration validator/compiler/pact checks and lock order.
      No self/adult/captain escalation via parent mode or administrator flags.
- [ ] Define atomic transitions between blocked, limited, approved and always-allowed
      representations. Clear conflicting fields for this target only and preserve all
      unrelated rules. Local essential protections remain unconditional.
- [ ] Define retained signed operation identity, expiry and lost-response reconciliation
      before UI Save. A desired-state setter with CAS may reconcile to matching current
      state without falsely attributing another parent's change to this operation; do not
      create an additive retry or invent a delivery receipt.
- [ ] Integrate draft/save/refusal/conflict/outcome-unknown states. Keep pending operations
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

## Earlier evidence

The design review extracted all text and rendered representative light/dark/large-text
mockups offline; it did not change the export. The preceding Ulysses/history checkpoint
passes 216 JVM checks, Go race/vet/portable UI, package smoke, three focused UI suites,
36 native requests and 98 Mast checks. Those results do **not** validate I1–I4: each
new slice must add its own runnable regression before its checkbox can be completed.
