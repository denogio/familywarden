# CI artifacts and pilot deployment

This is the authoritative CI gate and artifact deployment reference.

CI is the consolidated gate for pushes to `master` and `feature/**`, pull requests,
tags and manual dispatches. Superseded non-tag pushes on a ref are cancelled.
Concurrency groups are isolated by event: a push cannot cancel a manual emulator dispatch.
Independent shell/integration/AdGuard, portable Go/browser, x86_64 SDK,
aarch64_cortex-a53 SDK and Android checks/builds run in parallel. LuCI e2e reuses
x86 packages. Each router target and Android variant is built once; consumers never
rebuild. The **Validated artifacts gate** requires all mandatory jobs and the emulator
when explicitly requested. Releases also require this gate.

## Coverage and provenance audit

- x86_64: SDK packages, installation/reinstallation/removal smoke, migration fixtures
  (full tag history), downloaded v0.9.2 release migration, and LuCI browser tests.
  The legacy download remains an external GitHub release dependency.
- aarch64_cortex-a53: static linux/arm64 core cross-build and SDK packaging. This is
  **build-only validation**, not an installation or hardware smoke test. Architecture-
  independent backend/LuCI packages come from the x86 artifact.
- Go: formatting, vet, race tests, portable-service dependency boundary and shell-free
  shared-UI browser proof. The aarch64 SDK job builds the deployable core binary.
- Android: unit tests, lint, release and debug builds; versionCode is the workflow run
  number. Release signing still uses only `MAST_KEYSTORE_B64` and
  `MAST_KEYSTORE_PASSWORD` through the established temporary-keystore flow.
- Emulator checks are manual-dispatch only, not implied by an ordinary green push.
  CI2 tracks the unresolved approved-minute exhaustion failure from `41f309b`.
  Its assertion is unchanged; no flake/bug classification has been established.

The three deployment artifacts are `apks-<sha>`, `core-aarch64-<sha>` and `mast-<sha>`,
retained for 30 days. Each includes `provenance.json`: full source SHA, repository,
run ID/attempt, source package versions including release counters, Android version,
validation scope and SHA-256 hashes of the exact package files. These are workflow
provenance/checksums, not independent cryptographic attestations or package signatures.
Treat GitHub workflow/repository access as the trust boundary. Logs and artifacts must
contain only synthetic fixtures, never private router/household data or signing keys.

## Fetch and verify (no deployment)

Authenticate `gh` as `denogio` for this private repository; do not switch accounts to
bypass a permission error. Use a trusted checkout of these scripts. Pin a full 40-digit
commit SHA, not a mutable branch name. From this repository:

```sh
sha=<full-40-digit-commit-sha>
./scripts/fetch-ci-artifacts.sh "$sha" "./ci-$sha"
```

Requires `gh`, Python 3 and Bash. The output directory must not already exist.
The helper selects the latest non-PR CI run among the latest 100 runs for exactly
that SHA. PR runs are excluded because their artifacts use a synthetic merge SHA.
It refuses if no eligible run exists, or unless the selected run is complete and
successful and its current-attempt **Validated artifacts gate** succeeded. It downloads
all three artifacts, checks their SHA/repository/run, exact file sets, consistent
package versions and SHA-256 hashes, and rejects unsigned Android release output.
Expired artifacts, missing permissions or attempts newer than the run fail closed: do not
replace them with unvalidated local builds. A failed fetch may leave a partial directory;
remove it before retrying. "Re-run failed jobs" keeps artifacts from jobs that already
succeeded in an earlier attempt of the same run; those are accepted (attempt 1 through the
current attempt) because the current attempt's gate still requires every job. A rerun keeps its original Android versionCode,
so do not use it to supersede an already installed higher-code build. Before the first
CI-based Android update, compare the recorded versionCode with the installed versionCode;
a previous local build may have a higher code. Do not bypass downgrade protection.

Review the printed versions and each manifest against the pinned source Makefiles;
keep the printed run URL and manifests as deployment evidence. Checksums detect corrupt
or mixed files, not a compromised workflow. Do not install the debug APK as a release
update. Verify the release APK signing-certificate fingerprint against the previously
trusted installed/release certificate (for example with `apksigner verify --print-certs`)
before updating: the signing key must remain the same.

## Separately authorized deployment

Fetching does **not** authorize deployment. After required gates and any pilot-specific
emulator/hardware checks are green, obtain deployment authorization, preserve the
existing configuration/keys/bindings, and select the core matching the router architecture.
For example, copy only the verified packages and manifests to the authorized target;
verify SHA-256 again after transfer, then use the normal package update procedure:

```sh
# On the authorized target, with exact verified filenames (no cross-architecture glob):
apk add --allow-untrusted ./familywarden-core-<version>-r<release>.apk \
  ./familywarden-<version>-r<release>.apk ./luci-app-familywarden-<version>-r<release>.apk
# Separately authorized Android update, retaining the existing signing key:
adb install -r ./ulysses-mast-<version>.apk
```

Do not rebuild locally for deployment. `scripts/deploy.sh` remains a source-copy iteration
tool, not the validated artifact deployment path. Release-tag version matching and
`--verify-tag` are unchanged. This document authorizes no router, pact, tag or release changes.
