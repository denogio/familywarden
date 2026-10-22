#!/bin/sh
#
# Build FamilyWarden .apk packages using the official OpenWrt SDK in Docker.
#
# familywarden and luci-app-familywarden are architecture-independent (pure
# shell/lua/js/json), so any apk-era SDK image produces packages installable on
# any 25.x router. familywarden-core is compiled Go: it only runs on the SDK's
# architecture (x86_64 by default; e.g. SDK_IMAGE=openwrt/sdk:mediatek-filogic-25.12.4
# for aarch64_cortex-a53). WITH_CORE=0 skips it.
#
#   SDK_IMAGE=openwrt/sdk:x86-64-main ./scripts/build-apk-docker.sh
#
# Resulting .apk files are written to ./dist/ (override with OUT=...)

set -e

SDK_IMAGE="${SDK_IMAGE:-openwrt/sdk:x86-64-25.12.4}"
WITH_CORE="${WITH_CORE:-1}"
# The SDK compiles its own Go (feeds/packages lang/golang) from this external
# bootstrap instead of building the whole bootstrap chain from C.
GO_BOOTSTRAP_IMAGE="${GO_BOOTSTRAP_IMAGE:-golang:1.25-bookworm}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${OUT:-$REPO/dist}"
mkdir -p "$OUT"
# The SDK runs as buildbot (uid 1000); let it write here even when the host
# user differs (e.g. GitHub runners are uid 1001).
chmod a+w "$OUT"

echo "Using SDK image: $SDK_IMAGE"
echo "Repo:            $REPO"
echo "Output:          $OUT"

if [ "$WITH_CORE" = 1 ]; then
	docker volume create fw-go-bootstrap >/dev/null
	docker run --rm -v fw-go-bootstrap:/out "$GO_BOOTSTRAP_IMAGE" \
		sh -c '[ -x /out/bin/go ] || cp -a /usr/local/go/. /out/'
fi

docker run --rm \
	-v "$REPO:/src:ro" \
	-v "$OUT:/out" \
	-v fw-go-bootstrap:/opt/go-bootstrap:ro \
	-e FW_SRC=/src -e WITH_CORE="$WITH_CORE" \
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

		if [ "$WITH_CORE" = 1 ]; then
			# -L: src is a symlink to the repository core/ module
			rm -rf package/familywarden-core
			cp -rL "$FW_SRC/familywarden-core" package/familywarden-core
			./scripts/feeds install golang
			echo CONFIG_GOLANG_EXTERNAL_BOOTSTRAP_ROOT=\"/opt/go-bootstrap\" >> .config
		fi

		make defconfig >/dev/null

		# Force rebuild by removing stale build artifacts
		rm -rf build_dir/target-*/familywarden build_dir/target-*/luci-app-familywarden
		rm -f bin/target-*/packages/*/familywarden-*.apk bin/target-*/packages/*/luci-app-familywarden-*.apk

		make package/familywarden/compile V=s
		make package/luci-app-familywarden/compile V=s
		if [ "$WITH_CORE" = 1 ]; then
			rm -rf build_dir/target-*/familywarden-core-*
			rm -f bin/packages/*/base/familywarden-core-*.apk
			make package/familywarden-core/compile V=s
		fi

		find bin -name "*familywarden*.apk" -exec cp -v {} /out/ \;
	'

echo ""
echo "=== Built packages ==="
ls -la "$OUT"
