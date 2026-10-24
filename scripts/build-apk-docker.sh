#!/bin/sh
#
# Build FamilyWarden .apk packages using the official OpenWrt SDK in Docker.
#
# familywarden and luci-app-familywarden are architecture-independent (pure
# shell/lua/js/json), so any apk-era SDK image produces packages installable on
# any 25.x router. familywarden-core is compiled Go: it only runs on the SDK's
# architecture (x86_64 by default; e.g. SDK_IMAGE=openwrt/sdk:mediatek-filogic-25.12.4
# for aarch64_cortex-a53). familywarden depends on it. WITH_CORE=0 skips it.
#
# CORE_GO=cross (default) cross-compiles the core with plain Go (seconds) and lets the
# SDK only package it; CORE_GO=sdk compiles it with the feeds' Go toolchain (~10 min).
# CORE_GOARCH/CORE_GOARM select the target for cross builds (default from SDK_IMAGE).
#
#   SDK_IMAGE=openwrt/sdk:x86-64-main ./scripts/build-apk-docker.sh
#
# Resulting .apk files are written to ./dist/ (override with OUT=...)

set -e

SDK_IMAGE="${SDK_IMAGE:-openwrt/sdk:x86-64-25.12.4}"
WITH_CORE="${WITH_CORE:-1}"
CORE_GO="${CORE_GO:-cross}"
case "$SDK_IMAGE" in
	*x86-64*) DEFAULT_GOARCH=amd64 ;;
	*mediatek-filogic*|*mediatek-mt7622*|*rockchip-armv8*|*bcm27xx-bcm2710*|*armsr-armv8*) DEFAULT_GOARCH=arm64 ;;
	*) DEFAULT_GOARCH="" ;;
esac
CORE_GOARCH="${CORE_GOARCH:-$DEFAULT_GOARCH}"
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

PREBUILT=""
if [ "$WITH_CORE" = 1 ] && [ "$CORE_GO" = cross ]; then
	[ -n "$CORE_GOARCH" ] || { echo "set CORE_GOARCH for $SDK_IMAGE (or CORE_GO=sdk)"; exit 1; }
	PREBUILT=$(mktemp -d)
	trap 'rm -rf "$PREBUILT"' EXIT
	VERSION="$(sed -n 's/^PKG_VERSION:=//p' "$REPO/familywarden-core/Makefile")-r$(sed -n 's/^PKG_RELEASE:=//p' "$REPO/familywarden-core/Makefile")"
	echo "Cross-compiling familywarden-core $VERSION for linux/$CORE_GOARCH"
	docker run --rm -u "$(id -u):$(id -g)" -v "$REPO/core:/core:ro" -v "$PREBUILT:/out" -w /core -e GOPATH=/tmp/gopath \
		-e CGO_ENABLED=0 -e GOOS=linux -e GOARCH="$CORE_GOARCH" -e GOARM="${CORE_GOARM:-}" \
		-e GOFLAGS=-modcacherw -e GOCACHE=/tmp/gocache "$GO_BOOTSTRAP_IMAGE" \
		go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/familywardend ./cmd/familywardend
	chmod 755 "$PREBUILT" "$PREBUILT/familywardend"
fi
if [ "$WITH_CORE" = 1 ] && [ "$CORE_GO" = sdk ]; then
	docker volume create fw-go-bootstrap >/dev/null
	docker run --rm -v fw-go-bootstrap:/out "$GO_BOOTSTRAP_IMAGE" \
		sh -c '[ -x /out/bin/go ] || cp -a /usr/local/go/. /out/'
fi

docker run --rm \
	-v "$REPO:/src:ro" \
	-v "$OUT:/out" \
	${PREBUILT:+-v "$PREBUILT:/prebuilt:ro"} \
	$( [ "$CORE_GO" = sdk ] && echo "-v fw-go-bootstrap:/opt/go-bootstrap:ro" ) \
	-e FW_SRC=/src -e WITH_CORE="$WITH_CORE" -e CORE_GO="$CORE_GO" \
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
			if [ "$CORE_GO" = cross ]; then
				# Package the cross-compiled binary; no Go toolchain in the SDK.
				cp -r "$FW_SRC/familywarden-core" package/familywarden-core
				rm -f package/familywarden-core/src
				mkdir -p package/familywarden-core/prebuilt
				cp /prebuilt/familywardend package/familywarden-core/prebuilt/
			else
				cp -rL "$FW_SRC/familywarden-core" package/familywarden-core
				./scripts/feeds install golang
				echo CONFIG_GOLANG_EXTERNAL_BOOTSTRAP_ROOT=\"/opt/go-bootstrap\" >> .config
			fi
		fi

		make defconfig >/dev/null

		# Force rebuild by removing stale build artifacts
		rm -rf build_dir/target-*/familywarden build_dir/target-*/luci-app-familywarden
		rm -f bin/target-*/packages/*/familywarden-*.apk bin/target-*/packages/*/luci-app-familywarden-*.apk

		# The core first: familywarden depends on it.
		if [ "$WITH_CORE" = 1 ]; then
			rm -rf build_dir/target-*/familywarden-core-*
			rm -f bin/packages/*/base/familywarden-core-*.apk
			make package/familywarden-core/compile V=s
		fi
		make package/familywarden/compile V=s
		make package/luci-app-familywarden/compile V=s

		find bin -name "*familywarden*.apk" -exec cp -v {} /out/ \;
	'

echo ""
echo "=== Built packages ==="
ls -la "$OUT"
