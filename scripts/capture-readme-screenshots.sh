#!/bin/bash
# README screenshots with a fictional family only: LuCI (desktop, light/dark) from a
# throwaway OpenWrt container, plus phone captures from scripts/test-requests-ui.sh and
# scripts/test-screen-time-ui.sh output directories (optional).
#
#   ./scripts/capture-readme-screenshots.sh <apk-dir> [phone-capture-dir ...]
#
# Needs docker, node and the Playwright browser (see test/e2e). Never targets a router.
set -euo pipefail
REPO=$(cd "$(dirname "$0")/.." && pwd)
APKS=$(realpath "${1:?apk directory}")
shift
OUT="$REPO/docs/screenshots"
NAME=fw-readme-$$
PORT=${README_PORT:-18097}
PASSWORD=readme-only
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$NAME" --cap-add NET_ADMIN -p "127.0.0.1:$PORT:80" \
    -v "$APKS:/apk:ro" -v "$REPO:/src:ro" openwrt/rootfs:x86-64-25.12.4 sleep infinity >/dev/null
timeout --kill-after=10 300 docker exec -e PASSWORD="$PASSWORD" "$NAME" sh /src/scripts/e2e-container-setup.sh
timeout --kill-after=10 120 docker exec "$NAME" sh /src/scripts/readme-fixture.sh
FW_BASE_URL="http://127.0.0.1:$PORT" FW_PASSWORD="$PASSWORD" FW_OUT="$OUT" \
    node "$REPO/test/e2e/readme-screenshots.cjs"

# Phone captures (1080×2400) → 540 px wide, status bar cropped, small optimized PNGs.
phone() {
    local src
    for dir in "$@"; do
        [ -f "$dir/$PHONE_SRC" ] && { src="$dir/$PHONE_SRC"; break; }
    done
    [ -n "${src:-}" ] || return 0
    magick "$src" -crop 1080x2337+0+63 +repage -resize 540x -strip -define png:compression-level=9 "$OUT/$PHONE_DST"
    echo "wrote $OUT/$PHONE_DST"
}
for pair in child-light:app-today.png child-extra-light:app-extra-time.png \
            parent-light:app-parent-requests.png times-up-dark:app-times-up-dark.png; do
    PHONE_SRC="${pair%%:*}.png" PHONE_DST="${pair##*:}" phone "$@"
done
