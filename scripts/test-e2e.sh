#!/bin/bash
# LuCI end-to-end tests (Playwright) against a throwaway OpenWrt container
# running the built apks with LuCI (ubusd + rpcd + uhttpd, no procd).
#
#   ./scripts/test-e2e.sh [apk-dir]          build apks if no dir is given
#   FW_BASE_URL=http://localhost:8080 FW_PASSWORD=ulysses \
#       npx --prefix test/e2e playwright test   # against the QEMU router instead
#
# Needs docker, node and the Playwright browser (npx playwright install chromium).

set -e

REPO="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE="${OPENWRT_IMAGE:-openwrt/rootfs:x86-64-25.12.4}"
NAME="familywarden-e2e-$$"
PORT="${E2E_PORT:-18090}"
PASSWORD="e2e-sirens"

APK_DIR="$1"
if [ -z "$APK_DIR" ]; then
    APK_DIR=$(mktemp -d)
    echo "Building apks..."
    OUT="$APK_DIR" "$REPO/scripts/build-apk-docker.sh" > "$APK_DIR/build.log" 2>&1 \
        || { tail -20 "$APK_DIR/build.log"; exit 1; }
fi
APK_DIR="$(cd "$APK_DIR" && pwd)"

cleanup() { docker rm -f "$NAME" "${NAME}-owned" "${NAME}-shared" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$NAME" --memory=768m --cpus=2 --cap-add NET_ADMIN -p "127.0.0.1:$PORT:80" \
    -v "$APK_DIR:/apk:ro" -v "$REPO:/src:ro" "$IMAGE" sleep infinity >/dev/null
docker exec -e PASSWORD="$PASSWORD" "$NAME" sh /src/scripts/e2e-container-setup.sh \
    || { echo "container setup failed"; exit 1; }

echo "LuCI up on http://127.0.0.1:$PORT - running Playwright"
cd "$REPO/test/e2e"
FW_BASE_URL="http://127.0.0.1:$PORT" FW_PASSWORD="$PASSWORD" FW_E2E_ALLOW_PACT=1 \
    FW_E2E_CONTAINER="$NAME" npx playwright test "${@:2}"

# Phase 2: the core owns the configuration (import, CAS, pact DNS). Only the
# runner may set FW_E2E_OWNED=1: it starts a second, fresh container so the
# cutover never runs against a pact-active router or a user-configured QEMU.
if [ "${FW_E2E_OWNED:-}" = 1 ]; then
    OWNED_NAME="${NAME}-owned"
    OWNED_PORT="${E2E_OWNED_PORT:-18091}"
    docker run -d --name "$OWNED_NAME" --memory=768m --cpus=2 --cap-add NET_ADMIN -p "127.0.0.1:$OWNED_PORT:80" \
        -v "$APK_DIR:/apk:ro" -v "$REPO:/src:ro" "$IMAGE" sleep infinity >/dev/null
    docker exec -e PASSWORD="$PASSWORD" "$OWNED_NAME" sh /src/scripts/e2e-container-setup.sh
    echo "Owned-core phase on http://127.0.0.1:$OWNED_PORT"
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    FW_BASE_URL="http://127.0.0.1:$OWNED_PORT" FW_PASSWORD="$PASSWORD" FW_E2E_ALLOW_PACT=1 \
        FW_E2E_CONTAINER="$OWNED_NAME" FW_E2E_OWNED_PHASE=1 npx playwright test owned-core.spec.js

    # Shared administration performs its own explicit cutover in a fresh host;
    # it must never import or mutate the legacy suite's real pact fixture.
    docker rm -f "$OWNED_NAME" >/dev/null 2>&1 || true
    SHARED_NAME="${NAME}-shared"
    SHARED_PORT="${E2E_SHARED_PORT:-18092}"
    docker run -d --name "$SHARED_NAME" --memory=768m --cpus=2 --cap-add NET_ADMIN -p "127.0.0.1:$SHARED_PORT:80" \
        -v "$APK_DIR:/apk:ro" -v "$REPO:/src:ro" "$IMAGE" sleep infinity >/dev/null
    docker exec -e PASSWORD="$PASSWORD" "$SHARED_NAME" sh /src/scripts/e2e-container-setup.sh
    echo "Shared-control phase on http://127.0.0.1:$SHARED_PORT"
    FW_BASE_URL="http://127.0.0.1:$SHARED_PORT" FW_PASSWORD="$PASSWORD" FW_E2E_ALLOW_PACT=1 \
        FW_E2E_CONTAINER="$SHARED_NAME" FW_E2E_SHARED_PHASE=1 npx playwright test control.spec.js --grep 'native LuCI bridge'
fi
