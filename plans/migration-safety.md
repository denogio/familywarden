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
| `core/*.json.present`, `core/usage/*/*.json.present` | Presence witnesses v1, private and written once per data file | Distinguish never-created state from loss; not content hashes or newest-state counters |
| `core/authority.json` | Immutable ownership record v1 (`owner=core`) | Prevent a missing household journal from selecting legacy; not an anti-rollback generation |
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

## R7b — Durable ownership and refusal of legacy fallback

Implemented in core **0.13.0-r19** with backend **0.11.0_rc10-r15**. Update the pair
together; no LuCI or signed phone protocol revision is needed. Owned startup/import
requires the native `authority_format=authority-v1` capability before adoption.

- [x] First activation persists and reads back the household recovery journal, then
  `authority.json`, before any native apply. The floor is immutable and not rewritten
  for ordinary mutations. Failed floor write/readback leaves recovery required.
- [x] Adopt an existing unmarked v1 household before applying it, without changing its
  revision, active pact or registry. Incompatible native adapters cannot activate/adopt.
- [x] A valid floor with a missing journal refuses reads, signed phone policy, import
  and restart before native recovery. An owned household's missing registry also
  refuses startup before any apply/import; an empty first-use registry is not invented.
- [x] Native routing checks presence of both files, including dangling links and
  non-regular entries. Only the core validates their contents. Core JSON readers reject
  non-regular files rather than confusing dangling links with first-use absence.
- [x] Under the native state lock, owned households cannot call the old configuration,
  context, phone-policy or registry import/removal seams. Original network, DNS and
  protected-release entry points retain core routing when the journal/core is missing.
- [x] Real OpenWrt tests cover authenticated sail release/cancel refusal, unchanged
  native rules and exact journal repair retaining the pact. Real AGH tests preserve
  both core acknowledgements and legacy files, and retain remote parental protection.

The lost-journal regression originally exported permissive legacy configuration and
returned a signed pending-enrollment response. It now refuses without native calls or
document changes. Startup preflight covers six known versioned documents plus missing
owned journal/registry cases (14 refusal cases). Existing-family adoption is tested
with identical v1/r18-format documents, not claimed as a historical APK upgrade run.

| Layout / pair | Current behavior |
|---|---|
| No journal/floor, no registry | Legacy first-use import remains supported |
| Unmarked v1 journal + registry, matching native capability | Persist/read back floor before recovery/apply; preserve revision and identity |
| Journal + floor + registry | Ordinary owned operation |
| Floor but missing journal | Explicit recovery required; no legacy fallback or new signed policy |
| Owned journal but missing registry | Refuse before native recovery or stale identity import |
| Future/malformed ownership record or non-regular core JSON file | Refuse; native routing never interprets damage as legacy |
| New core with an adapter lacking `authority-v1` | Refuse owned activation/adoption without changing the household/native policy |

Local R7b evidence: full Go race/vet with the portable UI, 519 real OpenWrt checks,
101 real AGH checks, eight Node checks and syntax checks passed. Fresh r19/r15/r8 APKs
passed install/reinstall/removal smoke and all 39 browser cases (33 foundation,
four owned-core and two shared/native). Package lifecycle tests now wait for write
ownership, not merely socket disappearance, before simulating a replacement daemon.
No Android milestone or production deployment was performed.

Limits: this floor records **that** the core was selected, not **which state is newest**.
It cannot detect deletion of every ownership artifact, restoring an older complete
household, or an older backend that does not honor it. R7c below adds per-file absence detection,
not a newest-state generation manifest. Full rollback-resistant restore and
historical package compatibility remain open; no automatic unsafe downgrade is supported.

## R7c — Detect loss of previously stored ledgers and accounting

Implemented in core **0.13.0-r20** with backend **0.11.0_rc10-r16**. The native
`authority-v2` capability extends ownership routing to the configuration/floor's
`.present` witnesses. Older native capabilities are refused for owned activation/adoption.

- [x] Each top-level document and usage-day write persists its data and a private v1
  presence witness before returning success. Existing witnesses are validated, not
  rewritten for every report. This adds no background evaluator or platform dependency.
- [x] A witnessed missing file is damaged state, not a first-use empty ledger or a
  measured zero. Cached writers cannot silently recreate it. An uncertain data/witness
  write blocks further reads/writes in that store instance until reopen/reconciliation.
- [x] Startup checks all existing witnesses before product recovery. After known-schema
  preflight, adoption validates all existing JSON candidates before witnessing them.
  Existing bytes, identity, decisions and counters are not rewritten by adoption.
- [x] Intentional usage retention durably removes the expired witness before deleting
  its data. Interrupted retention may leave old data, not a required-but-deleted day.
- [x] Core/native ownership routing recognizes surviving witnesses even when both
  authority JSON files are absent. Authenticated sail and original RPCs stay refused.
- [x] Fault injection covers interruption after data persistence/before witness commit,
  store-instance refusal, subsequent adoption, metadata version/corruption refusal,
  optional-ledger loss before recovery, lost usage and attempted counter restart.

The regression originally treated a deleted `requests.json` as a never-created ledger.
Real OpenWrt tests now remove the actual persisted request/grant ledger, refuse startup
without changing native rules, and recover its exact bytes without resetting decisions.
Synthetic legacy-import fixtures move witnesses with their data only while stopped;
that fixture reset is not a recovery recommendation.

Local evidence: full Go race/vet and portable browser proof; 524 real fw4/nft checks,
101 AGH checks, eight Node checks, syntax checks, fresh r20/r16/r8 APK smoke/reinstall,
and all 39 foundation/owned/shared browser cases passed. No Android milestone or live
deployment was performed.

Limits: witnesses detect **absence**, not replacement by older valid contents. Removing
both a data file and its witness removes this evidence; an old complete backup can still
contain both. They are not a signed generation manifest, do not make live backups
consistent, and do not authorize automatic restore. Historical data lost before first
successful adoption cannot be reconstructed or retrospectively detected.

## R7d — Tagged legacy source state into current APKs

`scripts/test-upgrade.sh [apk-dir]` checks local `v0.9.1` and `v0.9.2` release tags
in separate disposable OpenWrt containers. Each actual tagged backend creates a
fictional household with an active timed pact, overnight rules, router-access
restriction, default deny, strict DNS identity and an approved administrator phone.
Fresh keys/secrets are generated only in the container; no router backups are used.

The current core/backend/LuCI APKs are then installed. Explicit import and restart
preserve identity, administrator role, signing keys, native AGH ownership/credentials/
acknowledgements, pact deadline, cooling-off, default policy and ClientID. An authenticated
v2 phone request verifies the original router signature and retains approved binding,
active pact and strict DNS hostname. Reimport of divergent UCI and captain allow cannot
bypass the pact. Both paths passed **46 checks** against r20/r16/r8 packages.

CI's x86 package-build job is configured to fetch release-tag history and run this
source-state migration proof before publishing its APK artifact. This tests current
APK installation and tagged historical backend behavior; it does **not** install an old
APK or exercise its historical maintainer scripts. The handset is a signed protocol
consumer, not a real old Android APK. Recorded pilot versions, arbitrary mixed packages,
physical behavior and the grouped Android acceptance matrix remain separate gates.

## R7e — Offline, signed whole-household backup (no restore)

Implemented in core **0.13.0-r21**; backend/LuCI unchanged (rc10-r16/r8).

- `familywardend backup OUT.tar.gz` runs only offline: it takes the daemon writer
  lease first, so a running core refuses it before any native read. It then holds the
  native ticket/state/DNS locks in their established order. It preflights presence
  witnesses and known schemas read-only; it never constructs services, adopts,
  imports, expires, confirms or applies policy.
- The router-agnostic `internal/backup` format is one gzip'd USTAR archive. A
  domain-separated (`familywarden-backup-v1`) usign signature by the existing router
  key covers a manifest of platform, installed versions, signer fingerprint and every
  file's size/SHA-256. The OpenWrt adapter supplies `/etc/familywarden` (keys, AGH
  credentials/ownership/acks, pact files, core journal/ledgers/usage and witnesses)
  plus `/etc/config/familywarden`, excluding the lifetime lock. No key is generated.
- Output is written through a private temporary file and published with a no-clobber
  link as 0600. Sources must be regular files within size/count limits; symlinked
  roots/files, special files and destinations inside a source are refused. Each source
  is re-checked for size/mtime changes while copying.
- `familywardend -trusted-pub PIN.pub backup-inspect FILE` requires an externally
  trusted pin, verifies every byte without extracting, and refuses foreign signers,
  changed content/manifest/signature, traversal, duplicates, links, oversize entries,
  truncation, concatenated gzip members and trailing bytes. Summaries never print
  file names, paths or contents and always report `restore_authorized:false`.

Evidence: focused Go fault/tamper tests (round trip, no-clobber, unsafe sources, ten
rejected artifact mutations, live-writer refusal, lock holding/release, missing key
not regenerated, non-disclosing summaries). `scripts/test-upgrade.sh` now also backs
up both migrated v0.9.1/v0.9.2 households (72 checks): live refusal, private signed
archive containing keys/journals/pact/UCI but not the lock, unchanged sources, pinned
inspection, no-clobber and appended-byte refusal. Full Go race/vet, 524 fw4/nft,
101 AGH, APK smoke and syntax checks pass.

Limits: the signature proves **who** produced the archive and that it is intact, not
that it is the newest household state. The archive contains secrets: anyone with it
can impersonate the router. Restore is deliberately not implemented; R7f must define
admission (newest-state evidence, pact/grant/usage floors) before any restore exists.

## R7f — Restore onto a cleared router only

Implemented in core **0.13.0-r22**; backend/LuCI unchanged.

`familywardend -trusted-pub PIN.pub restore FILE` is offline (writer lease first) and
holds the native locks. It admits a backup only when **all** hold:

- The target household is cleared: at most an unpaired fresh router key, the lifetime
  lock, an empty imported registry/witness and empty usage directories; the legacy
  projection has no profiles, phones or pact. Any other file, link or approved phone is
  refused. Restore never overlays or merges, so it cannot roll back newer router state.
- The archive verifies against an externally supplied pin (not the replacement router's
  local key) in a single extract-and-hash pass into a private staging directory.
- It is a complete core-owned household (authority floor, journal, registry). Legacy
  backups are refused: their UCI and pact files cannot be placed atomically together.
- The staged router key pair is consistent and is the archive signer, so phones pinned
  to it recognize the restored router.
- The staged core passes presence-witness, configuration and owned-registry preflight.

Activation moves the cleared household aside (never deleted) and renames the staged
household into place in one directory rename. UCI/pact files are not copied: core
startup materializes them from the restored journal and reapplies native intent. DNS
reconciles through the normal sync; restored acknowledgements are kept so phones keep
strict DNS instead of falling back. The command never starts the daemon.

Evidence: focused tests for placement and eight refusal reasons (each asserted by
cause, with target identity and state unchanged). `scripts/test-upgrade.sh` now backs
up each migrated v0.9.1/v0.9.2 household, proves overlay refusal by cause, simulates a
replacement router with a fresh identity, refuses live-daemon/local-key/changed-archive
restores, then restores and restarts: core ownership, pact deadline/default deny, UCI
projection, kernel rules and v2 handset verification/binding/pact/DNS all survive, and a
second restore is refused (116 checks). Full Go race/vet, 524 fw4/nft, 101 AGH, APK
smoke and syntax checks pass.

**Limit addressed by R7g below:** the router alone cannot tell whether a valid backup
is the newest; an older one is still admitted onto a cleared router. Only phones
observed the newer state, so R7g makes them the witnesses.

## R7g — Phones refuse older household state (generation with restore lineage)

Core **0.13.0-r23** and the Android app (same release line) implement it.

- The store keeps a reserved, witnessed `generation.json`: a random 128-bit epoch, a
  counter advanced **before** every other top-level document write (usage days are
  excluded; phones re-raise their own counters), and up to 16 restore fork points.
  Product code cannot write it. Startup, backup and restore preflight validate it.
- Signed approved v2/v3 replies carry `generation` read **after** the policy content,
  so it covers at least what was read. Offline backups therefore never pair older
  content with a generation a phone saw for newer content.
- Restore forks: the staged household gets a fresh epoch whose lineage records where
  each earlier epoch stopped. Pre-generation backups fork without ancestry.
- The phone (`HouseholdGeneration.follows`) accepts a reply only if it continues its
  last accepted epoch at or beyond its counter, or if lineage contains that epoch at
  or beyond it. An older backup is refused **permanently**: later router writes advance
  only the new epoch, never the frozen fork point. A router that stops sending a
  generation after one was seen is a downgrade. Refusal keeps the last trusted policy
  (reason: "router state is older than this phone"), like other signed-reply failures.

Evidence: store tests (advance-before-write, usage exclusion, reserved name, bounded
distinct lineage, loss/damage refusal); service test that signed policy carries an
advancing generation across configuration, bonus and restart; preflight refusal now
covers seven documents. Android: 204 JVM tests (four new generation tests plus a real
HTTP transport test: same-epoch regression, older restore even after 900 writes,
missing generation, newest restore and continuation), lint and debug build pass.
`scripts/test-upgrade.sh` (156 checks) now uses the same rule in its signed handset:
an older backup restored onto a cleared router is refused across three restarts with
new writes, while the newest backup is accepted with pact, binding and DNS intact.
Full Go race/vet, 524 fw4/nft, 101 AGH, APK smoke and all 39 browser cases pass.

Limits: old phone APKs do not enforce this; phones without a stored generation (first
contact, pre-r23 routers) accept the first value. A phone that refused an older restore
stays on its last trusted policy until re-enrolled (re-pairing is guarded while bound);
there is no administrator override, by design. Root/physical access to the router can
still forge state and stays outside the protection boundary. Usage history is not
protected by the generation (it self-heals from phones' cumulative reports).

## R7h — Released v0.9.2 package upgrade and grouped Android milestone

`LEGACY_RELEASE_DIR=<dir> scripts/test-upgrade.sh` additionally installs the actual
published `familywarden-0.9.2-r1` and `luci-app-familywarden-0.7.0-r1` APKs (their own
postinst creates the firewall include and sail-page instance), builds the same fictional
active-pact household with the packaged backend, then upgrades with `apk` to the current
packages. The full migration/backup/restore/generation path passes for it too; the
package is really replaced, exactly one firewall include remains and the protected-
release uhttpd instance survives. Locally: 240 checks over v0.9.1/v0.9.2 sources and the
released package against r23/r16/r8. CI's package job downloads the project's own
v0.9.2 release assets with its repository token and runs this.

Grouped Android milestone on r23 + the R7g debug app (Android 15 emulator, disposable
OpenWrt): `test-mast-emulator.sh` 98 checks (pairing, enrollment, Device Owner, pact,
address repair, DNS failure/restoration, offline reboot, removal) and
`test-requests-emulator.sh` 31 P8 checkpoints (signed requests, grants, new apps,
non-owner parent) passed. The earlier CI grant-consumption-after-restart failure was
later traced to a policy-read race at process start and fixed (see core-migration.md,
"Android request failure evidence").

App-upgrade rehearsal (`scripts/test-pilot-upgrade.sh`, Android 15 emulator, debug-
signed with the same local debug key): the pre-generation app (`b18fa3a`) as Device
Owner paired with the released v0.9.2 router; router upgraded in place to r16 + core
r23 (legacy import of the enrolled phone); app upgraded in place to the R7g build.
Device Owner, pairing and keys survive; the upgraded app accepts generation-bearing
signed policy, acknowledges the signed app revision, pauses a blocked app and lifts it
on the router kill switch. All checks passed.

Pilot-generation router path (what the production router runs): rc4 packages
(`familywarden 0.11.0_rc4-r1`, `familywarden-core 0.6.0-r1`, LuCI `0.9.0-r1`) are not
published, so they were rebuilt from their exact commit `73b921b` with that commit's own
build script. `LEGACY_RELEASE_DIR` accepts several package directories; when the old
package includes a core it is started first, imports the enrolled phone, and the pact is
created with it running, then the current packages replace it. The full migration,
backup, restore and generation path passes: 323 checks across v0.9.1/v0.9.2 sources,
released v0.9.2 and rebuilt rc4. CI runs the released v0.9.2 variant only.

Historical finding (rc4, not the current path): rc4's core imported **no** captain
phones when it was first started *during* an active pact (its effective configuration
hid the captain's phone). The production router imported its phone with core 0.4.0
before any pact, so it is not affected; the current core imports a captain phone from
a v0.9.x household with an active pact correctly (proved above).

Not covered: release-signed APK upgrades (needs the release keystore, deliberately not
used here), rc3/rc5 packages, the real production household's data, and physical devices.

## R7i — Interrupted-mutation matrix

`core/internal/configuration/interruption_test.go` interrupts, in separate fixtures,
**each** durable write and the native apply of configuration apply, confirm, pact
creation, release request, release cancel and release completion (16 cut points:
11 writes, 5 native applies; confirm has none). For every case the mutation must not
report success, and after a restart the native projection must equal a reconciled
durable document that is either the prior state, the uninterrupted control result or,
for an unconfirmed edit, its confirmed predecessor; the recovered authority must serve.
A separate case proves an interrupted bonus is neither kept as success nor doubled by
the retry. Import interruption (four cut points), identity ledger/registry/new-app write
failures and network apply restoration were already covered. The test fake now reports
the materialized pact captain like the OpenWrt adapter, instead of never doing so.

Not covered here: power loss inside a single file replacement (relies on the store's
temp-file, fsync and rename discipline) and simultaneous loss of several cut points.

## Remaining implementation and acceptance matrix

| Gate | Required evidence | Status |
|---|---|---|
| Supported upgrade paths | Fictional v0.9.1/v0.9.2 and recorded pilot-generation fixtures upgrade without changing effective restrictions, keys, identity/scopes or DNS ownership | v0.9.1/v0.9.2 sources, released v0.9.2 and rebuilt rc4 (production generation) -> current proved; debug-signed app upgrade proved on emulator; release-signed app upgrade and physical pilot open |
| Idempotent import | Retry before/after journal, ownership-floor write/readback, native apply and completion; once owned, a changed legacy projection cannot become authority again | Current-pair coverage passes; extend to historical/package fixtures |
| Interrupted mutations | Configuration confirm/rollback, pact creation/release, grants/usage, bonus and DNS ownership/ack writes interrupted at durable/native cut points | Configuration/pact/release/bonus matrix (R7i) plus existing import/identity/network/DNS coverage; single-file power loss relies on atomic replace |
| Concurrent/versioned writers | Current daemon exclusion plus supported package combinations and explicit refusal of non-cooperating/legacy writers | Current daemon lease and current-pair legacy refusal proved; historical mixed-version gate open |
| Router/app compatibility | Old router/new app and new router/old app; unsupported capabilities/formats refuse without erasing last trusted protection | Existing protocol tests plus v2 signed-consumer continuity from both legacy source states; actual app-version matrix open |
| Whole-household backup | One private artifact, manifest/checksums/versions, native secrets and all durable ledgers from a quiesced consistent state | Offline signed export/inspection proved (R7e); restore absent by design |
| Restore admission | Validate before activation; never merge arbitrary old ledgers or weaken pact minima; define what can be proved when the latest state is unavailable | Cleared-target-only, pinned, staged, complete-owned restore (R7f); phones refuse older restored state via generation lineage (R7g) |
| Post-restore application | Reapply native intent and reconcile DNS; do not treat backed-up applied/verified observations as fresh proof | Startup reapplies/materializes (proved); DNS acks kept until normal reconciliation |
| Integrated release milestone | Native packages/UI/AGH plus one grouped Android milestone against the selected candidate; resolve or explicitly block on unexplained failures | r23 candidate: native, 39 browser, 98+31 emulator checks pass; the earlier CI grant failure is explained (policy-read race at process start) and fixed in the app |

### Compatibility rules to establish before enabling restore

1. Package version, document schema, private adapter format and signed phone
   capabilities are different compatibility dimensions; record the supported tuples.
2. Unsupported old software must not fall back to a materialized UCI projection of
   an owned household. A package downgrade is not an approved schema downgrade.
3. R19's ownership floor and R20's presence witnesses block missing-file fallback after
   adoption. Removing all evidence remains indistinguishable from historical first use;
   this is not enough to admit partial backups or arbitrary old household snapshots.
4. A complete but old backup may still roll back spent grants, usage or tighter pact
   rules. Never promise automatic safe merging when no trustworthy newer floor exists.
5. A backup requires the daemon and native writers to be quiesced. The startup lease
   alone does not make a live multi-document/native-file copy consistent.
6. Backups contain signing keys and credentials: keep them private and out of fixtures,
   screenshots, logs and public artifacts. All regression households/keys are fictional.

## Status (2026-10-28)

The automated R7 matrix is closed: startup preflight and writer exclusion (R7a),
ownership floor (R7b), absence witnesses (R7c), tagged-source, released v0.9.2 and
rebuilt rc4 package upgrades (R7d/R7h), offline signed backup (R7e), cleared-target
restore (R7f), phone-witnessed generation lineage (R7g), interrupted-mutation matrix
(R7i), app-upgrade rehearsal, and the explained/fixed CI grant failure. Candidate:
core 0.13.0-r23, backend 0.11.0_rc10-r16, LuCI 0.11.0-r8 and the app at `7e600bb`.

Still required before production cutover, and only with explicit authorization: a
release-signed app upgrade rehearsal and a physical router+phone pilot with backups.
Accepted, documented residual limits: non-cooperating old binaries, deliberate removal
of all ownership evidence, root/physical forgery, and old APKs without generation checks.

## Validation and rollout

Use focused Go tests for each recovery boundary, then real OpenWrt/package/browser
checks where those layers change. Run the grouped Android suites once at the integrated
milestone, not after each edit. The unresolved CI grant-consumption-after-restart failure
must retain its evidence and cannot be dismissed because an unchanged local run passed.

No production router, physical phone or active QEMU pact may be changed without new
explicit authorization. Legacy paths remain until the compatibility/recovery matrix
passes. R7 stays open until its remaining gates are either proved or explicitly excluded
from the supported release contract.
