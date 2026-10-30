# Local Mast emulator regression

The default `scripts/test-mast-emulator.sh <router-apk-dir> <debug-apk>` still runs
all checks. CI continues to use that default; no workflow input changes are needed.

For the RB1 reboot/foreground-count regression only, use validated, matching cached
artifacts for `7e203b04e17d1030fa96dc716cfae65e4d0a9044` (CI run `41097273092`):

```sh
sha=7e203b04e17d1030fa96dc716cfae65e4d0a9044
# Uses the denogio GitHub account; output directory must not exist yet.
./scripts/fetch-ci-artifacts.sh "$sha" /tmp/fw-ci4r
MAST_MODE=focused ./scripts/test-mast-emulator.sh \
  "/tmp/fw-ci4r/apks-$sha" "/tmp/fw-ci4r/mast-$sha/ulysses-mast-debug.apk"
```

Requires local Docker and `/dev/kvm`; no APK rebuild, phone deployment or live
router access. The script provisions a disposable router container and emulator,
pairs and approves the emulator, releases that test pact via signed sync (otherwise
app settings remain frozen), and sets up usage access and acknowledged app policy.
It then runs the **same** reboot/debit/checkpoint and measured foreground-growth
assertions as the full mode, with the same bounded polling and first-failure
capture (`FW_MAST_DIAG_DIR`). Any failed check still produces a nonzero exit.

Focused success is **scoped RB1 evidence only**, never the full installation/release
gate. It omits address repair, UI variants, release/rebind/removal scenarios,
offline cached-pact reboot and daily-limit/bonus checks. Run the default full mode
independently for those guarantees. The main session records real-run results and
timing separately; fast harness tests alone are not emulator evidence.
