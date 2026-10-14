#!/bin/sh
# Smoke-test built apks: install into an OpenWrt rootfs container, check files
# land where init/rpcd expect them and that postinst adds exactly one firewall
# include, also after a reinstall/upgrade.
#
#   ./scripts/test-apk.sh [apk-dir]     (default ./dist, newest versions)

if [ "$1" != "--inside" ]; then
    IMAGE="${OPENWRT_IMAGE:-openwrt/rootfs:x86-64-25.12.4}"
    REPO="$(cd "$(dirname "$0")/.." && pwd)"
    DIR="$(cd "${1:-$REPO/dist}" && pwd)"
    exec docker run --rm -v "$REPO:/src:ro" -v "$DIR:/apk:ro" "$IMAGE" \
        sh /src/scripts/test-apk.sh --inside
fi

FAILED=0
pass() { echo "✓ $1"; }
fail() { echo "❌ $1"; FAILED=1; }

# Newest version of each package in /apk
pick() { ls /apk/"$1"-[0-9]*.apk | sort -V | tail -1; }
FW=$(pick familywarden)
LUCI=$(pick luci-app-familywarden)
echo "Testing $(basename "$FW") + $(basename "$LUCI")"

# luci-base is a dependency of the luci app; avoid the network by not
# resolving deps — we only verify our own packaging here.
apk add --allow-untrusted --no-network --force-broken-world "$FW" "$LUCI" >/tmp/apk.log 2>&1 \
    && pass "apk install" || { fail "apk install"; cat /tmp/apk.log; }

for f in /usr/libexec/rpcd/familywarden /etc/init.d/familywarden /usr/sbin/familywarden; do
    [ -x "$f" ] && pass "executable: $f" || fail "missing/not executable: $f"
done
for f in /etc/config/familywarden /usr/share/rpcd/acl.d/familywarden.json \
         /usr/share/rpcd/acl.d/luci-app-familywarden.json \
         /usr/share/luci/menu.d/luci-app-familywarden.json \
         /www/luci-static/resources/view/familywarden/overview.js \
         /www/luci-static/resources/familywarden/ulysses.js; do
    [ -f "$f" ] && pass "installed: $f" || fail "missing: $f"
done
grep -q '@@VERSION@@' /www/luci-static/resources/view/familywarden/info.js \
    && fail "info.js version placeholder not substituted" \
    || pass "info.js version substituted"

count_includes() { uci show firewall | grep -c "path='/var/etc/familywarden.include'"; }
[ "$(count_includes)" = "1" ] && pass "postinst added firewall include" || fail "firewall includes: $(count_includes) (want 1)"

uci set familywarden.marker=profile && uci commit familywarden
apk fix --allow-untrusted --no-network --reinstall familywarden >/dev/null 2>&1 \
    || apk add --allow-untrusted --no-network --force-broken-world "$FW" >/dev/null 2>&1
[ "$(count_includes)" = "1" ] && pass "reinstall keeps one include" || fail "firewall includes after reinstall: $(count_includes) (want 1)"
uci -q get familywarden.marker >/dev/null && pass "reinstall keeps user config" || fail "reinstall overwrote /etc/config/familywarden"

apk del --no-network luci-app-familywarden familywarden >/dev/null 2>&1
[ "$(count_includes)" = "0" ] && pass "prerm removed firewall include" || fail "firewall includes after removal: $(count_includes) (want 0)"

echo ""
[ "$FAILED" -eq 0 ] && echo "All apk checks passed!" || echo "Some apk checks FAILED"
exit "$FAILED"
