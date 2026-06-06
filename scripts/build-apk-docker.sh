#!/bin/sh
#
# Build FamilyWarden .apk packages using the official OpenWrt SDK in Docker.
#
# Both packages are architecture-independent (pure shell/lua/js/json), so any
# apk-era SDK image produces packages installable on any 25.x router. Override
# the SDK image/tag for your exact target with SDK_IMAGE=...
#
#   SDK_IMAGE=openwrt/sdk:x86-64-main ./scripts/build-apk-docker.sh
#
# Resulting .apk files are written to ./dist/

set -e

SDK_IMAGE="${SDK_IMAGE:-openwrt/sdk:x86-64-25.12.4}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$REPO/dist"
mkdir -p "$OUT"

echo "Using SDK image: $SDK_IMAGE"
echo "Repo:            $REPO"
echo "Output:          $OUT"

docker run --rm \
	-v "$REPO:/src:ro" \
	-v "$OUT:/out" \
	-e FW_SRC=/src \
	"$SDK_IMAGE" sh -ce '
		# cd to SDK root (may not be the default working directory)
		SDK="/builder"
		cd "$SDK"
		echo "SDK root: $SDK"

		# Feeds needed: luci provides luci.mk + luci-base for the app.
		./scripts/feeds update -a >/dev/null 2>&1 || ./scripts/feeds update base packages luci

		# Daemon package -> package/familywarden
		rm -rf package/familywarden
		cp -r "$FW_SRC/familywarden" package/familywarden

		# LuCI app must live inside the luci feed so ../../luci.mk resolves.
		LUCI_APPS="feeds/luci/applications"
		rm -rf "$LUCI_APPS/luci-app-familywarden"
		mkdir -p "$LUCI_APPS/luci-app-familywarden"
		cp -r "$FW_SRC/luci-app-familywarden/." "$LUCI_APPS/luci-app-familywarden/"

		# Re-index feeds so the freshly-copied app is known, then install.
		./scripts/feeds update -i
		./scripts/feeds install luci-base luci-app-familywarden

		make defconfig >/dev/null

		# Force rebuild by removing stale build artifacts
		rm -rf build_dir/target-*/familywarden build_dir/target-*/luci-app-familywarden
		rm -f bin/target-*/packages/*/familywarden-*.apk bin/target-*/packages/*/luci-app-familywarden-*.apk

		make package/familywarden/compile V=s
		make package/luci-app-familywarden/compile V=s

		find bin -name "*familywarden*.apk" -exec cp -v {} /out/ \;
	'

echo ""
echo "=== Built packages ==="
ls -la "$OUT"
