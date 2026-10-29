# ICON2 local visual and functional check

Implementation check, 2026-10-29. No live router, phone, QEMU pact or deployment.
Fixtures use Alex and fictional packages only. Independent review completed; the main
session verified both P1 fixes, inspected the DPR2 captures and ran final affected Go
race tests plus vet. Passing unchanged worker checks were reused, not repeated.

## Reproduce

```sh
FW_CONTROL_PROTOTYPE=/path/to/control-prototype.html node test/e2e/icon-coverage.cjs
(cd core && FW_TEST_SHARED_UI=1 FW_ICON_SCREENSHOTS=/tmp/fw-icon2-screenshots \
  go test ./internal/service -run TestPortableSharedUI -count=1 -v)
```

The first command requires the existing Playwright/Chromium installation. Set
`FW_CONTROL_PROTOTYPE` to the local control prototype for visual comparison; without
it only prototype captures are skipped, not the functional checks.
`FW_ICON_SCREENSHOTS` overrides output.
It serves only a disposable loopback HTTP fixture, not a product listener.

## Browser evidence

All captures use 1440×1000 CSS pixels and DPR 2:

- `/tmp/fw-icon2-screenshots/prototype-family-dpr2.png`
- `/tmp/fw-icon2-screenshots/prototype-history-dpr2.png`
- `/tmp/fw-icon2-screenshots/control-history-dpr2.png`
- `/tmp/fw-icon2-screenshots/icon-grid-dpr2.png`

The real-service History capture retains the prototype's rounded-square 34px tile,
left text alignment, row spacing and meter alignment. Artwork occupies the tile
instead of sitting inside a blue initials background. The gallery adds square,
circular transparent-style and wide artwork, each normalized onto the same opaque
square; clipping belongs only to the outer tile. Neither artwork nor wide motifs
are stretched. Missing and broken PNGs retain initials. The prototype uses colored
initials rather than real PNG artwork, so this is a geometry/alignment comparison,
not a pixel-identical artwork comparison or acceptance of unrelated History gaps.
The gallery's noisy images are deliberately adversarial transport fixtures, not
proposed UI artwork.

The browser checks all 210 images, including the last package, through 207 bounded
responses on initial load, as well as missing/invalid images. The five-minute
background renewal drains another 207 responses; two ordinary polls transfer none. The real-service browser uses a signed
96px sidecar upload and checks intrinsic size, image loading, tile size and CSS fit.
Android native-graphics JVM tests independently check adaptive layer corners/center,
96px dimensions, the adaptive visible 72/108 region reaching each viewport edge,
nullable layers, opaque legacy transparency and a 2:1 legacy motif's aspect ratio.

## Persistence and repair

The Go tests cover 512 persisted icons, restart, metadata-only refresh, uninstall /
reinstall, rebinding, revoked keys, replay, unobserved packages, PNG bounds, exact
60-KiB upload limits, all administrator pages, cache loss/corruption and repair.
Icons are regenerable atomic files, not globally witnessed authority documents;
missing/corrupt artwork cannot prevent startup or discard inventory metadata.

Android progress survives restart and suppresses unchanged uploads between repair
cycles. A complete identical-image reconciliation starts after six hours, on the
next successful sync. Each sync sends at most 32 bounded batches; larger sets resume
on later syncs (the existing WorkManager schedule is nominally 15 minutes, subject
to Android scheduling/connectivity). Thus six hours is the repair-start interval,
not a guaranteed completion deadline. Failures defer only icons for six hours and
never mark unsigned refusals as unsupported or change legacy inventory fallback.


## Review corrections

- Browser cache freshness is five minutes, independent of the 15-second polling
  interval. Expired cached artwork remains visible during background revalidation.
  The browser regression advances two poll cycles without icon transfers, then
  blocks the five-minute renewal and checks that all 210 images remain present.
- Adaptive layers use Android's extra 25% inset on each edge, preserving the visible
  72/108 scale without restoring the launcher mask. Null layers are skipped.
- OpenWrt backups exclude `household/core/inventory-icons`, but keep authority
  metadata; an adapter test includes over 4096 regenerable files to exercise limits.
- The requested identical-image no-write optimization was deliberately not applied:
  a newer sequence must remain durable even if its pixels are unchanged. Otherwise
  an intermediate-sequence replay could replace it after restart. This is covered
  by a restart regression. A repair with 189 cached icons writes 189 atomic files;
  four nominal six-hour cycles mean about 756 file replacements per phone per day,
  in addition to initial transfers and actual changes. This flash cost is retained
  to preserve the approved replay guarantee; no new watermark architecture is added.
