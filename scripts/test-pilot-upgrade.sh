#!/bin/bash
# Rehearse a pilot upgrade on disposable containers before touching a real phone/router:
# router <old> + enrolled phone <old app>  ->  router <new> + phone <new app> (adb install -r,
# same signing key), then app limits from the signed v3 policy and the router kill switch.
#
#   scripts/test-pilot-upgrade.sh <old-router-apk> <new-router-apk> <old-app.apk> <new-app.apk> [user-app.apk]
#
# user-app.apk: any ordinary app (e.g. test/android/install-probe fixture). With
# apps_suspend_new=0 it must stay usable: that is what keeps a daily phone usable.
# Release builds have no debug receivers, so everything goes through the real UI.
set -u
OLD_ROUTER=$(realpath "$1"); NEW_ROUTER=$(realpath "$2"); OLD_APP=$(realpath "$3"); NEW_APP=$(realpath "$4")
USER_APP=${5:+$(realpath "$5")}
SDK_IMAGE=ghcr.io/cirruslabs/android-sdk:35
ID=$$; NET="pilot-net-$ID"; ROUTER="pilot-router-$ID"; EMU="pilot-emu-$ID"
FAILED=0
pass() { echo "✓ $1"; }
fail() { echo "❌ $1"; FAILED=1; }
cleanup() { docker rm -f "$ROUTER" "$EMU" >/dev/null 2>&1; docker network rm "$NET" >/dev/null 2>&1; }
trap cleanup EXIT

docker network create "$NET" >/dev/null
echo "=== Router on the old version ==="
docker run -d --name "$ROUTER" --network "$NET" --cap-add NET_ADMIN \
    -v "$OLD_ROUTER:/old.apk:ro" -v "$NEW_ROUTER:/new.apk:ro" openwrt/rootfs:x86-64-25.12.4 sleep infinity >/dev/null
docker exec "$ROUTER" sh -c '
    mkdir -p /var/lock /var/run /var/etc
    apk update >/dev/null 2>&1
    apk add --allow-untrusted /old.apk >/dev/null 2>&1 || { echo "apk add failed"; exit 1; }
    cat > /etc/config/familywarden <<EOF
config global settings
	option enabled 1
config profile pilot
	option name Pilot
	option enabled 1
	option policy allow
EOF
    uci set firewall.@defaults[0].input=ACCEPT; uci commit firewall
    fw4 start >/dev/null 2>&1; familywarden reload >/dev/null
    (uhttpd -f -p 0.0.0.0:7700 -h /www-familywarden -x /cgi-bin >/dev/null 2>&1 &)'
ROUTER_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$ROUTER")
KEY=$(docker exec "$ROUTER" familywarden mast-pubkey | head -1 | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g')
PAIR_URI="familywarden://pair?host=router.invalid&address=$ROUTER_IP:7700&key=$KEY"
call() { docker exec "$ROUTER" sh -c "echo '${2:-{\}}' | /usr/libexec/rpcd/familywarden call $1"; }
dev() { call devices | python3 -c "import json,sys; d=json.load(sys.stdin)['devices']; print(d[0].get('$1','') if d else '')"; }
echo "router $(docker exec "$ROUTER" apk list -I familywarden 2>/dev/null | sed 's/ .*//') at $ROUTER_IP"

echo "=== Phone with the old app (Device Owner, enrolled) ==="
docker run -d --name "$EMU" --network "$NET" --memory 4g --memory-swap 4g --device /dev/kvm \
    --sysctl net.ipv6.conf.lo.disable_ipv6=0 -v mast-sdk:/opt/extra \
    -v "$OLD_APP:/old.apk:ro" -v "$NEW_APP:/new.apk:ro" ${USER_APP:+-v "$USER_APP:/user.apk:ro"} \
    "$SDK_IMAGE" sleep infinity >/dev/null
docker exec "$EMU" sh -c "
    export ANDROID_HOME=/opt/extra ANDROID_SDK_ROOT=/opt/extra
    mkdir -p ~/.android/avd/t.avd
    printf 'avd.ini.encoding=UTF-8\npath=/root/.android/avd/t.avd\ntarget=android-35\n' > ~/.android/avd/t.ini
    printf '%s\n' 'image.sysdir.1=system-images/android-35/google_apis/x86_64/' 'abi.type=x86_64' \
        'hw.cpu.arch=x86_64' 'tag.id=google_apis' 'hw.ramSize=2048' 'disk.dataPartition.size=4G' \
        'hw.lcd.width=1080' 'hw.lcd.height=2400' 'hw.lcd.density=420' > ~/.android/avd/t.avd/config.ini
    (/opt/extra/emulator/emulator -avd t -no-window -no-audio -no-snapshot -no-boot-anim \
        -gpu swangle_indirect -accel on -memory 2048 -feature -ModemSimulator >/tmp/emu.log 2>&1 &)"
ADB="docker exec $EMU /opt/extra/platform-tools/adb"
for _ in $(seq 1 120); do [ "$($ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = 1 ] && break; sleep 3; done
sleep 15
ui() {
    $ADB shell input keyevent KEYCODE_WAKEUP; $ADB shell wm dismiss-keyguard
    # Home first: a start that only reaches the running activity skips onResume, i.e. no sync.
    $ADB shell input keyevent KEYCODE_HOME; sleep 1
    $ADB shell am start -n io.github.denogio.familywarden/.MainActivity >/dev/null   # onResume syncs
    sleep 8
    $ADB shell uiautomator dump /sdcard/ui.xml >/dev/null
    UI=$($ADB shell cat /sdcard/ui.xml | tr -d '\r')
}
tap_text() {
    local c
    c=$(printf '%s' "$UI" | python3 -c '
import sys, re, xml.etree.ElementTree as ET
n = next(n for n in ET.fromstring(sys.stdin.read()).iter("node") if n.get("text") == sys.argv[1])
x1,y1,x2,y2 = map(int, re.findall(r"\d+", n.get("bounds"))); print((x1+x2)//2, (y1+y2)//2)' "$1") || { fail "cannot tap $1"; return 1; }
    $ADB shell input tap $c; sleep 4
}
suspended() { $ADB shell dumpsys package "$1" | tr -d '\r' | grep -q 'suspended=true'; }
version() { $ADB shell dumpsys package io.github.denogio.familywarden | tr -d '\r' | grep -m1 -oE 'versionName=[^ ]+'; }

$ADB install /old.apk >/dev/null && pass "old app installed ($(version))" || fail "old app install"
for _ in $(seq 1 12); do $ADB shell dpm set-device-owner io.github.denogio.familywarden/.AdminReceiver 2>&1 | grep -q Success && break; sleep 5; done
$ADB shell dpm list-owners | grep -q familywarden && pass "Device Owner" || fail "not Device Owner"
$ADB shell am start -a android.intent.action.VIEW -d "'$PAIR_URI'" -n io.github.denogio.familywarden/.MainActivity >/dev/null
sleep 6; $ADB shell uiautomator dump /sdcard/ui.xml >/dev/null; UI=$($ADB shell cat /sdcard/ui.xml | tr -d '\r')
tap_text 'It matches, pair'
P=$(call devices); DEV=$(echo "$P" | python3 -c 'import json,sys; print(json.load(sys.stdin)["pending"][0]["id"])')
FP=$(echo "$P" | python3 -c 'import json,sys; print(json.load(sys.stdin)["pending"][0]["fingerprint"])')
call device_approve "{\"device\":\"$DEV\",\"profile\":\"pilot\",\"name\":\"Pilot\",\"administrator\":\"1\",\"fingerprint\":\"$FP\"}" | grep -q '"ok"' \
    && pass "router approved the phone" || fail "approval"
ui; [ "$(dev seen)" != 0 ] && pass "old app syncs with old router" || fail "no sync after approval"
[ -n "$USER_APP" ] && { $ADB install /user.apk >/dev/null && pass "ordinary user app installed"; USER_PKG=io.github.denogio.familywarden.fixture; }

echo "=== Upgrade router, then phone (in place, same key) ==="
docker exec "$ROUTER" sh -c 'apk add --allow-untrusted /new.apk >/dev/null 2>&1 && familywarden reload >/dev/null' \
    && pass "router upgraded: $(docker exec "$ROUTER" apk list -I familywarden | sed 's/ .*//')" || fail "router upgrade"
ui; echo "$UI" | grep -q 'Pair with your router' && fail "old app lost pairing on router upgrade" || pass "old app still paired after router upgrade"
$ADB install -r /new.apk >/dev/null && pass "app upgraded in place ($(version))" || fail "app upgrade (signature/version?)"
$ADB shell dpm list-owners | grep -q familywarden && pass "still Device Owner" || fail "Device Owner lost"
ui; echo "$UI" | grep -q 'Pair with your router' && fail "pairing lost on app upgrade" || pass "pairing and keys kept"
[ "$(dev apps_applied)" = "" ] && pass "router: nothing applied yet (no app policy)" || fail "unexpected ack $(dev apps_applied)"

echo "=== App limits on (apps_suspend_new=0, as on a daily phone) ==="
TARGET=""
for p in com.android.camera2 com.android.deskclock com.google.android.deskclock; do
    $ADB shell pm path "$p" 2>/dev/null | grep -q package: && { TARGET=$p; break; }
done
$ADB shell appops set --uid io.github.denogio.familywarden GET_USAGE_STATS allow
docker exec -e T="$TARGET" "$ROUTER" sh -c '
    uci set familywarden.pilot.apps=1; uci set familywarden.pilot.apps_suspend_new=0
    uci set familywarden.pilot_block=app; uci set familywarden.pilot_block.profile=pilot
    uci set familywarden.pilot_block.package="$T"; uci set familywarden.pilot_block.blocked=1
    uci set familywarden.pilot_yt=app; uci set familywarden.pilot_yt.profile=pilot
    uci set familywarden.pilot_yt.package=com.google.android.youtube; uci set familywarden.pilot_yt.minutes=10
    uci commit familywarden; familywarden reload >/dev/null'
ui; sleep 6
suspended "$TARGET" && pass "blocked app paused ($TARGET)" || fail "blocked app not paused"
[ -n "${USER_PKG:-}" ] && { suspended "$USER_PKG" && fail "ordinary app paused despite suspend_new=0" || pass "ordinary app stays usable (suspend_new=0)"; }
$ADB shell dumpsys activity services io.github.denogio.familywarden | grep -q AppControlService && pass "app-control service running" || fail "service not running"
ui; [ -n "$(dev apps_rev)" ] && [ "$(dev apps_applied)" = "$(dev apps_rev)" ] && pass "signed ack = $(dev apps_rev)" || fail "ack '$(dev apps_applied)' vs '$(dev apps_rev)'"

echo "=== Kill switch on the router ==="
docker exec "$ROUTER" sh -c 'uci set familywarden.pilot.apps=0; uci commit familywarden; familywarden reload >/dev/null'
ui; sleep 4
suspended "$TARGET" && fail "pause not lifted by the kill switch" || pass "kill switch lifts the pause"
$ADB shell dumpsys activity services io.github.denogio.familywarden | grep -q AppControlService && fail "service still running" || pass "service stopped"
# The ack travels with a sync request: the sync that turned app control off still
# carried the old revision; the next one reports none.
ui
[ "$(dev apps_applied)" = "" ] && pass "router: no applied policy any more" || fail "stale ack"

[ "$FAILED" -eq 0 ] || $ADB logcat -d -s UlyssesMast:* AndroidRuntime:E | tr -d '\r' | grep -E "FATAL|Exception|app control|decision=" | tail -15 | sed 's/^/  log: /'
echo ""
[ "$FAILED" -eq 0 ] && echo "All pilot upgrade checks passed!" || echo "Some pilot upgrade checks FAILED"
exit "$FAILED"
