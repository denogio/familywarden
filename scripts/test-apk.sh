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
# familywarden depends on the core (phone identity, plans/core-migration.md)
CORE=$(pick familywarden-core)
echo "Testing $(basename "$FW") + $(basename "$LUCI") + $(basename "$CORE")"

# Dependencies (qrencode, ...) come from the OpenWrt feeds
apk update >/dev/null 2>&1
apk add --allow-untrusted "$CORE" "$FW" "$LUCI" >/tmp/apk.log 2>&1 \
    && pass "apk install" || { fail "apk install"; cat /tmp/apk.log; }
[ -x /usr/libexec/rpcd/familywarden ] || { fail "package not actually installed"; cat /tmp/apk.log; }

for f in /usr/libexec/rpcd/familywarden /etc/init.d/familywarden /usr/sbin/familywarden \
         /www-familywarden/cgi-bin/sail /www-familywarden/cgi-bin/mast /www-familywarden/cgi-bin/app; do
    [ -x "$f" ] && pass "executable: $f" || fail "missing/not executable: $f"
done
for f in /etc/config/familywarden /usr/share/rpcd/acl.d/familywarden.json \
         /usr/share/rpcd/acl.d/luci-app-familywarden.json \
         /usr/share/luci/menu.d/luci-app-familywarden.json \
         /www/luci-static/resources/view/familywarden/overview.js \
         /www/luci-static/resources/familywarden/ulysses.js \
         /www-familywarden/index.html; do
    [ -f "$f" ] && pass "installed: $f" || fail "missing: $f"
done
grep -q '@@VERSION@@' /www/luci-static/resources/view/familywarden/info.js \
    && fail "info.js version placeholder not substituted" \
    || pass "info.js version substituted"

count_includes() { uci show firewall | grep -c "path='/var/etc/familywarden.include'"; }
[ "$(count_includes)" = "1" ] && pass "postinst added firewall include" || fail "firewall includes: $(count_includes) (want 1)"
[ "$(uci -q get uhttpd.familywarden.home)" = "/www-familywarden" ] && uci -q get uhttpd.familywarden.listen_http | grep -q ':7700' \
    && pass "postinst added the sail page on :7700" || fail "uhttpd sail instance missing"

uci set familywarden.marker=profile && uci commit familywarden
apk add --allow-untrusted --force-reinstall "$FW" >/tmp/fw-reinstall.log 2>&1 \
    && pass "backend package reinstalled" || { fail "backend reinstall"; cat /tmp/fw-reinstall.log; }
[ "$(count_includes)" = "1" ] && pass "reinstall keeps one include" || fail "firewall includes after reinstall: $(count_includes) (want 1)"
uci -q get familywarden.marker >/dev/null && pass "reinstall keeps user config" || fail "reinstall overwrote /etc/config/familywarden"

if [ -n "$CORE" ]; then
    echo "--- $(basename "$CORE") (no procd here: the daemon is started by hand) ---"
    SOCK=/var/run/familywarden-core/core.sock
    st() { /usr/libexec/rpcd/familywarden-core call status; }
    for f in /usr/sbin/familywardend /etc/init.d/familywarden-core /usr/libexec/rpcd/familywarden-core; do
        [ -x "$f" ] && pass "executable: $f" || fail "missing/not executable: $f"
    done
    [ "$(/usr/libexec/rpcd/familywarden-core list)" = '{"status":{}}' ] && pass "rpcd lists status" || fail "rpcd list"
    [ "$(st)" = '{"error":"core not reachable","running":false}' ] && pass "status while down" || fail "status while down: $(st)"

    start_core() {
        mkdir -p /var/run/familywarden-core /var/lock   # procd boot creates /var/lock on a router
        familywardend -data /etc/familywarden/core -socket "$SOCK" 2>>/tmp/core.log &
        CORE_PID=$!
        # Not just [ -S ]: after a crash the stale socket file already exists.
        for _ in 1 2 3 4 5; do st | grep -q '"running":true' && break; sleep 1; done
    }
    start_core
    v=$(st)
    echo "$v" | grep -q '"running":true' && echo "$v" | grep -q '"core":"0\.' && pass "status via ubus bridge: $v" || fail "status while up: $v"
    mode() { ls -ld "$1" | cut -c1-10; }   # busybox has no stat
    [ "$(mode "$SOCK")" = srw------- ] && pass "socket is root-only" || fail "socket mode $(mode "$SOCK")"
    [ "$(mode /etc/familywarden/core)" = drwx------ ] && pass "data dir is root-only" || fail "data dir mode $(mode /etc/familywarden/core)"
    familywardend -data /etc/familywarden/core -socket "$SOCK" 2>/tmp/second.log && fail "second instance started" \
        || { grep -q 'another familywardend' /tmp/second.log && pass "second instance refused" || fail "second instance: $(cat /tmp/second.log)"; }
    echo "RSS of an idle core: $(grep VmRSS /proc/$CORE_PID/status | tr -s ' ')"

    kill -9 "$CORE_PID"; wait "$CORE_PID" 2>/dev/null
    [ -S "$SOCK" ] && pass "crash leaves a stale socket" || fail "no stale socket to test"
    start_core
    st | grep -q '"running":true' && pass "restart takes over the stale socket" || fail "restart after crash: $(cat /tmp/core.log)"

    touch /etc/familywarden/core/usage/marker
    # No procd in this rootfs: adapt only rc.common's service lifecycle. The real
    # packaged init/postinst and the real daemon/socket still participate. Without
    # postinst's restart the old PID remains and this assertion fails.
    cp /etc/rc.common /tmp/rc.common.original
    printf '%s\n' "$CORE_PID" >/tmp/core-service.pid
    cat >/etc/rc.common <<'EOF'
#!/bin/sh
if [ "$1" = /etc/init.d/familywarden-core ]; then
    case "$2" in
    enabled) exit 0 ;;
    restart)
        kill "$(cat /tmp/core-service.pid)" || exit 1
        for _ in 1 2 3 4 5; do [ ! -S /var/run/familywarden-core/core.sock ] && break; sleep 1; done
        familywardend >>/tmp/core.log 2>&1 &
        echo $! >/tmp/core-service.pid
        exit 0 ;;
    esac
fi
exec sh /tmp/rc.common.original "$@"
EOF
    apk add --allow-untrusted --force-reinstall "$CORE" >/tmp/core-reinstall.log 2>&1 \
        && pass "core package reinstalled while serving" || { fail "core reinstall"; cat /tmp/core-reinstall.log; }
    mv /tmp/rc.common.original /etc/rc.common
    old_pid=$CORE_PID
    CORE_PID=$(cat /tmp/core-service.pid)
    expected=$(basename "$CORE" .apk); expected=${expected#familywarden-core-}
    for _ in 1 2 3 4 5; do st | grep -q "\"core\":\"$expected\"" && break; sleep 1; done
    [ "$old_pid" != "$CORE_PID" ] && st | grep -q "\"core\":\"$expected\"" \
        && pass "upgrade restarted the daemon and serves installed version $expected" \
        || fail "upgrade left old daemon/version running: old=$old_pid new=$CORE_PID status=$(st)"
    [ -f /etc/familywarden/core/usage/marker ] && pass "reinstall keeps core data" || fail "reinstall removed core data"
    # Phone sync goes through the core: CGI on :7700 forwards, core down = 503.
    out=$(QUERY_STRING="device=x&nonce=y" sh /www-familywarden/cgi-bin/mast | head -1)
    [ "$out" = "Status: 400 Bad Request" ] && pass "sync endpoint answered by the core" || fail "sync endpoint: $out"
    echo '{}' | /usr/libexec/rpcd/familywarden call profiles | grep -q '"profiles"' && pass "LuCI profile directory via the core" || fail "profile directory via the core"
    echo '{}' | /usr/libexec/rpcd/familywarden call devices | grep -q '"devices"' && pass "LuCI devices via the core" || fail "devices via the core"
    echo '{}' | /usr/libexec/rpcd/familywarden call requests | grep -q '"requests"' && pass "LuCI requests via the core" || fail "requests via the core"
    out=$(REQUEST_METHOD=POST QUERY_STRING="op=request" sh /www-familywarden/cgi-bin/app </dev/null | head -1)
    [ "$out" = "Status: 400 Bad Request" ] && pass "upload endpoint answered by the core" || fail "upload endpoint: $out"
    kill "$CORE_PID"
    for _ in 1 2 3 4 5; do [ ! -e "$SOCK" ] && break; sleep 1; done
    [ -e "$SOCK" ] && fail "socket left after clean stop" || pass "clean stop removes the socket"
    out=$(QUERY_STRING="device=x&nonce=y" sh /www-familywarden/cgi-bin/mast | head -1)
    [ "$out" = "Status: 503 Service Unavailable" ] && pass "core down: 503, no reply" || fail "core down: $out"
fi

apk del luci-app-familywarden familywarden >/dev/null 2>&1
[ "$(count_includes)" = "0" ] && pass "prerm removed firewall include" || fail "firewall includes after removal: $(count_includes) (want 0)"
uci -q get uhttpd.familywarden >/dev/null && fail "uhttpd sail instance left after removal" || pass "prerm removed the sail page"
apk del familywarden-core >/dev/null 2>&1
[ -e /usr/sbin/familywardend ] && fail "core binary left after removal" || pass "core removed"
[ -f /etc/familywarden/core/usage/marker ] && pass "removal keeps core data (user state)" || fail "removal deleted core data"

echo ""
[ "$FAILED" -eq 0 ] && echo "All apk checks passed!" || echo "Some apk checks FAILED"
exit "$FAILED"
