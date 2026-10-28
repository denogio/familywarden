# R7 — Migration and recovery safety

R1–R6 establish the portable product boundary. R7 proves that supported upgrades,
interruptions and recovery preserve its authority. This is not a rewrite, another
router implementation, permission to deploy, or a claim that arbitrary downgrades work.

## Safety contract

These are release acceptance requirements, not blanket claims about the current build.

- A failed/incompatible startup must not change native enforcement, roll back an
  edit, import identities or overwrite documents before incompatibility is detected.
- Existing keys, identity/scopes, ClientIDs, DNS ownership and pact minima survive a
  supported migration. Strict DNS never falls back to unfiltered resolution.
- Retry/restart cannot duplicate bonus, extend a fixed grant, replay consumed usage
  or resurrect a permission removed by a pact.
- One supported daemon owns a data directory, even during startup recovery/retry or
  when different socket paths are supplied. Native adapters still serialize their
  own transactions; a daemon lease is not a replacement for native locks.
- An unsafe restore/downgrade is refused explicitly, not implemented by deleting
  the authority document or selecting a legacy writer.

## Durable-state inventory

| State | Current format / boundary | Safety concern |
|---|---|---|
| `core/configuration.json` | Household journal v1, embedded configuration v1; sealed revisions including rollback target | Current/candidate/before state, ownership, pact snapshots and durable bonus must stay consistent |
| `core/devices.json` | Registry v1 | Preserve public-key bindings, administrator role and parent scopes; missing is not equivalent to an approved empty replacement |
| `core/requests.json` | Ledger v1 | Preserve request decisions, fixed grant identity/deadlines, finalized expiry and trusted clock floor |
| `core/newapps.json` | Approval state v1 | Do not resurrect revoked approvals or erase pending restrictions |
| `core/dns.json` | DNS runtime v1 | Preserve owned ClientIDs and verified acknowledgements; reconcile remote reality after recovery |
| `core/usage/<profile>/<day>.json` | Unversioned per-device cumulative reports, 30-day retention | Spent grants and accounting offsets cannot regress or become an invented zero |
| Router signing keys, AGH credentials and legacy/native projection | Native root-private files outside those core documents | A configuration export is not a complete household backup |
| Native manual overrides/tickets; pending enrollment/removal and delivery observations | Volatile | Do not accidentally make RAM permissions durable through backup/restore |

## R7a — Startup preflight and writer exclusion

Implemented in core **0.13.0-r18**; no backend or UI revision change is required.

- [x] Read-only preflight of the five known versioned core documents before either
  production constructor can recover/apply/import. Reuse existing configuration,
  ledger, new-app and DNS validators; the registry keeps its existing version gate.
- [x] Future versions and truncated JSON in each document leave durable documents and
  native apply attempts unchanged, including when a connection confirmation is pending.
- [x] Acquire a native process-lifetime lease before opening the store or starting the
  product service. Hold it while retrying startup; do not unlink its inode.
- [x] Check private permissions, path aliases, lock-file symlink refusal, real second
  daemon startup rejection before store creation, normal exit and SIGKILL release.
- [x] Wait for HTTP shutdown draining before normal writer-lease release; a drain
  timeout exits the process without releasing ownership ahead of surviving handlers.

The regression first reproduced startup applying/rolling back configuration before
rejecting a future registry, and accepting future request/new-app/DNS formats until
later reads. `core/internal/service/migration_test.go` now exercises ten refusal cases
through the production constructor with real temporary files. Native lifecycle cases
live in `core/cmd/familywardend/writer_test.go`, including a real idle server/SIGTERM.
Full interruption coverage of in-flight mutations is still below, not implied by
that idle shutdown check.

Local R7a verification passed: full Go race/vet/gofmt with the portable browser proof
enabled, 495 real fw4/nft integration checks, 96 real AGH checks, eight Node checks,
syntax and production service dependency checks. FreeBSD cross-compilation still
selects OpenWrt and is build evidence only. No APK release, grouped Android rerun,
physical-device test, production deployment or safe-restore completion is claimed.

This is not a cross-file snapshot transaction or a full restore validator. It does
not inspect all historical usage at startup, detect every schema-valid modification,
introduce an irreversible ownership floor, or make old non-cooperating binaries honor
the new lease. These limits must not be mistaken for a completed R7 gate.

## Remaining implementation and acceptance matrix

| Gate | Required evidence | Status |
|---|---|---|
| Supported upgrade paths | Fictional v0.9.1/v0.9.2 and recorded pilot-generation fixtures upgrade without changing effective restrictions, keys, identity/scopes or DNS ownership | Open |
| Idempotent import | Retry before/after journal, native apply and completion; once owned, a changed legacy projection cannot become authority again | Existing focused coverage; extend to historical/package fixtures |
| Interrupted mutations | Configuration confirm/rollback, pact creation/release, grants/usage, bonus and DNS ownership/ack writes interrupted at durable/native cut points | Partial existing coverage; complete matrix open |
| Concurrent/versioned writers | Current daemon exclusion plus supported package combinations and explicit refusal of non-cooperating/legacy writers | Current daemon lease proved; mixed-version gate open |
| Router/app compatibility | Old router/new app and new router/old app; unsupported capabilities/formats refuse without erasing last trusted protection | Existing protocol tests; consolidated historical matrix open |
| Whole-household backup | One private artifact, manifest/checksums/versions, native secrets and all durable ledgers from a quiesced consistent state | Open |
| Restore admission | Validate before activation; never merge arbitrary old ledgers or weaken pact minima; define what can be proved when the latest state is unavailable | Open |
| Post-restore application | Reapply native intent and reconcile DNS; do not treat backed-up applied/verified observations as fresh proof | Open |
| Integrated release milestone | Native packages/UI/AGH plus one grouped Android milestone against the selected candidate; resolve or explicitly block on unexplained failures | Open |

### Compatibility rules to establish before enabling restore

1. Package version, document schema, private adapter format and signed phone
   capabilities are different compatibility dimensions; record the supported tuples.
2. Unsupported old software must not fall back to a materialized UCI projection of
   an owned household. A package downgrade is not an approved schema downgrade.
3. An absent authority file is currently a legacy selector, not proof that a household
   was never owned. Add/test the appropriate authority floor before claiming safety
   for partial backups or missing-file recovery.
4. A complete but old backup may still roll back spent grants, usage or tighter pact
   rules. Never promise automatic safe merging when no trustworthy newer floor exists.
5. A backup requires the daemon and native writers to be quiesced. The startup lease
   alone does not make a live multi-document/native-file copy consistent.
6. Backups contain signing keys and credentials: keep them private and out of fixtures,
   screenshots, logs and public artifacts. All regression households/keys are fictional.

## Validation and rollout

Use focused Go tests for each recovery boundary, then real OpenWrt/package/browser
checks where those layers change. Run the grouped Android suites once at the integrated
milestone, not after each edit. The unresolved CI grant-consumption-after-restart failure
must retain its evidence and cannot be dismissed because an unchanged local run passed.

No production router, physical phone or active QEMU pact may be changed without new
explicit authorization. Legacy paths remain until the compatibility/recovery matrix
passes. R7 stays open until its remaining gates are either proved or explicitly excluded
from the supported release contract.
