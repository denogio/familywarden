#!/bin/bash
# Ulysses Mast on real Android: an emulator (KVM) with the debug APK as device
# owner, paired with a throwaway FamilyWarden router container. Checks
# DNS-independent pairing, enrollment approval in the router, scoped pact
# enforcement, DNS failure reporting and restoration, and Android protection.
#
#   ./scripts/test-mast-emulator.sh <router-apk-dir> [familywarden-debug.apk]
#
# LEGACY_APK_DIR=<dir with an older familywarden .apk>: start with that router
# (global v1 replies must not authorize the phone), then upgrade it in place.
#
# Needs docker and /dev/kvm. The Android SDK (emulator + system image) lives in
# the docker volume "mast-sdk" (see the sdkmanager line below).

set -e

REPO="$(cd "$(dirname "$0")/.." && pwd)"
APK_DIR="$(cd "${1:?router apk dir}" && pwd)"
MAST_APK="$(realpath "${2:-$REPO/android/app/build/outputs/apk/debug/app-debug.apk}")"
SDK_IMAGE="ghcr.io/cirruslabs/android-sdk:35"
SYSIMG="system-images;android-35;google_apis;x86_64"
ID=$$
NET="mast-net-$ID"
ROUTER="mast-router-$ID"
EMU="mast-emu-$ID"
FAILED=0

pass() { echo "✓ $1"; }
fail() { echo "❌ $1"; FAILED=1; }

cleanup() {
    docker rm -f "$ROUTER" "$EMU" >/dev/null 2>&1 || true
    docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker volume inspect mast-sdk >/dev/null 2>&1 || docker volume create mast-sdk >/dev/null
docker run --rm -v mast-sdk:/opt/extra "$SDK_IMAGE" sh -c \
    "test -d /opt/extra/system-images || { yes | sdkmanager --sdk_root=/opt/extra --licenses >/dev/null; sdkmanager --sdk_root=/opt/extra emulator platform-tools '$SYSIMG' >/dev/null; }"

docker network create "$NET" >/dev/null

echo "=== Router with an ongoing pact (captain always blocked) ==="
docker run -d --name "$ROUTER" --network "$NET" --cap-add NET_ADMIN \
    -v "$APK_DIR:/apk:ro" -v "${LEGACY_APK_DIR:-$APK_DIR}:/legacy:ro" \
    openwrt/rootfs:x86-64-25.12.4 sleep infinity >/dev/null
docker exec -e START=$([ -n "${LEGACY_APK_DIR:-}" ] && echo legacy || echo apk) "$ROUTER" sh -c '
    mkdir -p /var/lock /var/run /var/etc
    apk update >/dev/null 2>&1
    apk add --allow-untrusted /$START/familywarden-[0-9]*.apk >/dev/null 2>&1 || { echo "router: apk add failed"; exit 1; }
    cat > /etc/config/familywarden <<EOF
config global settings
	option enabled 1
	option ulysses_profile alex
	option ulysses_delay 1
config profile alex
	option name Alex
	option enabled 1
	option policy block
	list device aa:bb:cc:00:00:01
EOF
    # The emulator reaches the router over docker'"'"'s bridge, which is in no fw4 zone
    uci set firewall.@defaults[0].input=ACCEPT
    uci commit firewall
    fw4 start >/dev/null 2>&1
    familywarden reload >/dev/null
    (uhttpd -f -p 0.0.0.0:7700 -h /www-familywarden -x /cgi-bin >/dev/null 2>&1 &)
    echo "{\"until\":0}" | /usr/libexec/rpcd/familywarden call pact >/dev/null
'
ROUTER_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$ROUTER")
KEY=$(docker exec "$ROUTER" familywarden mast-pubkey | head -1 | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g')
# router.invalid cannot resolve: successful pairing must use the numeric address.
PAIR_URI="familywarden://pair?host=router.invalid&address=$ROUTER_IP:7700&key=$KEY"
echo "router at $ROUTER_IP"

echo "=== Emulator (Android 15) ==="
docker run -d --name "$EMU" --network "$NET" --device /dev/kvm --sysctl net.ipv6.conf.lo.disable_ipv6=0 \
    -v mast-sdk:/opt/extra -v "$MAST_APK:/mast.apk:ro" "$SDK_IMAGE" sleep infinity >/dev/null
docker exec "$EMU" sh -c "
    export ANDROID_HOME=/opt/extra ANDROID_SDK_ROOT=/opt/extra
    # AVD by hand: avdmanager insists on its own SDK root
    mkdir -p ~/.android/avd/t.avd
    printf 'avd.ini.encoding=UTF-8\npath=/root/.android/avd/t.avd\ntarget=android-35\n' > ~/.android/avd/t.ini
    printf '%s\n' 'image.sysdir.1=system-images/android-35/google_apis/x86_64/' 'abi.type=x86_64' \
        'hw.cpu.arch=x86_64' 'tag.id=google_apis' 'hw.ramSize=2048' 'disk.dataPartition.size=4G' \
        'hw.lcd.width=1080' 'hw.lcd.height=2400' 'hw.lcd.density=420' > ~/.android/avd/t.avd/config.ini
    (/opt/extra/emulator/emulator -avd t -no-window -no-audio -no-snapshot -no-boot-anim \
        -gpu swangle_indirect -accel on -memory 2048 -feature -ModemSimulator >/tmp/emu.log 2>&1 &)
"
ADB="docker exec $EMU /opt/extra/platform-tools/adb"
for _ in $(seq 1 120); do
    [ "$($ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ] && break
    sleep 3
done
[ "$($ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ] || { docker exec "$EMU" tail -20 /tmp/emu.log; echo "emulator did not boot"; exit 1; }
pass "emulator booted ($($ADB shell getprop ro.build.version.release | tr -d '\r'))"

restrictions() { $ADB shell dumpsys user | tr -d '\r'; }
has() { restrictions | grep -A30 "UserInfo{0:" | grep -q "$1"; }
broadcast() { $ADB shell am broadcast -n io.github.denogio.familywarden/.DebugReceiver -a "$@" >/dev/null; sleep 4; }

# Smoke-test the actual Compose activity too, not just the background restriction.
# Optional screenshots: MAST_SCREENSHOT_DIR=/tmp/mast-ui ./scripts/test-mast-emulator.sh ...
ui() {
    $ADB shell input keyevent KEYCODE_WAKEUP
    $ADB shell wm dismiss-keyguard
    $ADB shell am start -n io.github.denogio.familywarden/.MainActivity >/dev/null
    sleep 8
    $ADB shell uiautomator dump /sdcard/mast-ui.xml >/dev/null
    UI=$($ADB shell cat /sdcard/mast-ui.xml | tr -d '\r')
    echo "$UI" | grep -q 'package="io.github.denogio.familywarden"' && pass "Compose activity renders ($1)" || fail "activity failed ($1)"
    if [ -n "${MAST_SCREENSHOT_DIR:-}" ]; then
        mkdir -p "$MAST_SCREENSHOT_DIR"
        $ADB exec-out screencap -p > "$MAST_SCREENSHOT_DIR/$1.png"
        printf '%s\n' "$UI" > "$MAST_SCREENSHOT_DIR/$1.xml"
    fi
}
ui_has() { scroll_to_text "$1" && pass "UI: $1" || fail "UI missing: $1"; }
tap_text() {
    local coords
    coords=$(printf '%s' "$UI" | python3 -c '
import sys, re, xml.etree.ElementTree as ET
root = ET.fromstring(sys.stdin.read())
node = next(n for n in root.iter("node") if n.get("text") == sys.argv[1])
x1,y1,x2,y2 = map(int, re.findall(r"\d+", node.get("bounds")))
print((x1+x2)//2, (y1+y2)//2)
' "$1") || { fail "cannot tap $1"; return 1; }
    $ADB shell input tap $coords
    sleep 4
}

tap_switch() {
    local coords
    coords=$(printf '%s' "$UI" | python3 -c '
import sys,re,xml.etree.ElementTree as ET
node=next(n for n in ET.fromstring(sys.stdin.read()).iter("node") if n.get("checkable")=="true")
x1,y1,x2,y2=map(int,re.findall(r"\d+",node.get("bounds")))
print((x1+x2)//2,(y1+y2)//2)
')
    $ADB shell input tap $coords
    sleep 3
}
scroll_to_text() {
    local text="$1"
    echo "$UI" | grep -q "text=\"$text\"" && return 0
    # Begin at the top even if a previous check scrolled the same destination.
    for _ in $(seq 1 3); do $ADB shell input swipe 540 700 540 1900 200; done
    $ADB shell uiautomator dump /sdcard/mast-ui.xml >/dev/null
    UI=$($ADB shell cat /sdcard/mast-ui.xml | tr -d '\r')
    for _ in $(seq 1 6); do
        echo "$UI" | grep -q "text=\"$text\"" && return 0
        $ADB shell input swipe 540 1800 540 700 500
        sleep 1
        $ADB shell uiautomator dump /sdcard/mast-ui.xml >/dev/null
        UI=$($ADB shell cat /sdcard/mast-ui.xml | tr -d '\r')
    done
    fail "cannot scroll to $text"
    return 1
}
open_removal() {
    tap_text 'Settings'
    ui removal-settings
    scroll_to_text 'Remove FamilyWarden'
    tap_text 'Remove FamilyWarden'
}

$ADB install -r /mast.apk >/dev/null && pass "APK installed (sideloaded, not from Play)" || fail "install"
# Android checks for accounts asynchronously after boot (hasIncompatibleAccountsTask);
# until that has finished, set-device-owner is refused. Retry for a bit.
for _ in $(seq 1 12); do
    out=$($ADB shell dpm set-device-owner io.github.denogio.familywarden/.AdminReceiver 2>&1 | tr -d '\r')
    echo "$out" | grep -q "Success" && break
    sleep 5
done
echo "$out" | grep -q "Success" && pass "became device owner" || fail "dpm set-device-owner: $(echo "$out" | head -2)"

$ADB shell settings put system screen_off_timeout 1800000
ui unpaired
ui_has 'Pair with your router'

echo ""
echo "=== Pairing; the router must approve this phone ==="
# Exercise the real pairing UI; the key is pinned only after fingerprint confirmation.
# Quoted for the device shell too: the URI contains "&".
$ADB shell am start -a android.intent.action.VIEW -d "'$PAIR_URI'" -n io.github.denogio.familywarden/.MainActivity >/dev/null
ui pairing-confirmation
ui_has 'Confirm the router'
tap_text 'It matches, pair'
$ADB logcat -d -s UlyssesMast:* AndroidRuntime:E | tr -d '\r' | grep -A4 -E "debug:|FATAL" | head -8 | sed 's/^/  log: /'
echo "  router :7700 from emulator: $($ADB shell "echo | nc -w 3 $ROUTER_IP 7700 >/dev/null 2>&1 && echo open || echo CLOSED" | tr -d '\r')"
if [ -n "${LEGACY_APK_DIR:-}" ]; then
    ui legacy-unsupported
    ui_has 'Router setup needs an update'
    has no_config_tethering && fail "global captain policy applied to an unassociated phone" || pass "global v1 policy cannot restrict this phone"
    docker exec "$ROUTER" sh -c 'apk add --allow-untrusted /apk/familywarden-[0-9]*.apk >/dev/null 2>&1 && familywarden reload >/dev/null' \
        && pass "router upgraded in place" || fail "router upgrade failed"
    broadcast io.github.denogio.familywarden.DEBUG_SYNC
fi
ui awaiting-approval
ui_has 'Waiting for approval'
has no_config_tethering && fail "pending phone restricted before approval" || pass "pending phone gets no policy"
# The administrator compares the fingerprint the app shows with LuCI's
APP_FP=$(printf '%s' "$UI" | grep -oE '([0-9a-f]{4} ){15}[0-9a-f]{4}' | head -1 | tr -d ' ')
PENDING=$(docker exec "$ROUTER" sh -c 'echo "{}" | /usr/libexec/rpcd/familywarden call devices')
DEV_ID=$(echo "$PENDING" | python3 -c 'import json,sys; print(json.load(sys.stdin)["pending"][0]["id"])')
ROUTER_FP=$(echo "$PENDING" | python3 -c 'import json,sys; print(json.load(sys.stdin)["pending"][0]["fingerprint"])')
[ -n "$APP_FP" ] && [ "$APP_FP" = "$ROUTER_FP" ] && pass "app and LuCI show the same fingerprint" || fail "fingerprints differ: app=$APP_FP router=$ROUTER_FP"
out=$(docker exec "$ROUTER" sh -c "echo '{\"device\":\"$DEV_ID\",\"profile\":\"alex\",\"name\":\"Alex phone\",\"administrator\":\"1\",\"fingerprint\":\"$ROUTER_FP\"}' | /usr/libexec/rpcd/familywarden call device_approve")
echo "$out" | grep -q '"status": "ok"' && pass "administrator approved the phone" || fail "approval: $out"
broadcast io.github.denogio.familywarden.DEBUG_SYNC
has no_config_tethering && pass "scoped pact restricts tethering" || fail "no_config_tethering missing"
ui blocked-light
ui_has 'Hotspot is locked'
echo "$UI" | grep -q 'text="Remove FamilyWarden"' && fail "removal offered on overview while bound" || pass "overview has no removal shortcut"
$ADB shell cmd uimode night yes
ui blocked-dark
ui_has 'Hotspot is locked'
$ADB shell settings put system font_scale 2.0
ui blocked-dark-large
ui_has 'Hotspot is locked'
$ADB shell settings put system font_scale 1.0
$ADB shell cmd uimode night no
has no_config_date_time && pass "clock locked while paired" || fail "no_config_date_time missing"
docker exec "$ROUTER" sh -c "test -s /tmp/familywarden.seen/$DEV_ID" && pass "router saw the approved phone sync" || fail "router has no sync record"

# The profile has no DNS yet: explicitly unconfigured, not green.
ui navigation-overview
tap_text 'DNS'
ui dns-details
ui_has 'Finish setting up DNS protection'
tap_text 'Ulysses'
ui ulysses-details
ui_has 'Bound by an ongoing pact'
ui_has 'No allowed windows today'
tap_text 'Overview'
ui navigation-return

# DNS policy the resolver can't serve: Android refuses strict DNS, and the app
# says so instead of claiming protection. (A reachable DoT resolver with a
# publicly trusted certificate is a real-device test.)
docker exec "$ROUTER" sh -c 'echo "alex alex dns.invalid filter_lists -" > /etc/familywarden/dns.ack'
broadcast io.github.denogio.familywarden.DEBUG_SYNC
sleep 10
ui dns-not-served
ui_has 'Android did not apply DNS protection'
has disallow_config_private_dns && pass "Private DNS settings locked by policy" || fail "disallow_config_private_dns missing"

# Native per-app locales: real Danish resources and 200% layout, no mock screen.
$ADB shell cmd locale set-app-locales io.github.denogio.familywarden --user 0 --locales da
sleep 5
ui danish
ui_has 'Android anvendte ikke DNS-beskyttelsen'
ui_has 'Hotspot er låst'
$ADB shell settings put system font_scale 2.0
ui danish-large
ui_has 'Hotspot er låst'
$ADB shell settings put system font_scale 1.0
$ADB shell cmd locale set-app-locales io.github.denogio.familywarden --user 0 --locales en
sleep 5
ui english-restored
ui_has 'Hotspot is locked'

out=$($ADB shell pm clear io.github.denogio.familywarden 2>&1 | tr -d '\r')
echo "$out" | grep -qi "protected\|SecurityException\|Failed" && pass "pm clear refused ($out)" || fail "pm clear allowed: $out"
out=$($ADB uninstall io.github.denogio.familywarden 2>&1 | tr -d '\r')
echo "$out" | grep -qi "DELETE_FAILED\|Failure" && pass "uninstall refused ($out)" || fail "uninstall allowed: $out"
out=$($ADB shell am force-stop io.github.denogio.familywarden 2>&1; $ADB shell pidof io.github.denogio.familywarden)
has no_config_tethering && pass "restriction survives force-stop" || fail "restriction gone after force-stop"

echo ""
echo "=== Released on the sail page ==="
docker exec "$ROUTER" sh -c 'echo $(( $(date +%s) - 1 )) > /etc/familywarden/release; familywarden tick'
broadcast io.github.denogio.familywarden.DEBUG_SYNC
has no_config_tethering && fail "tethering still restricted after release" || pass "tethering allowed again"
ui released
# Ulysses was never opted into: it disappears when the forced active pact ends.
echo "$UI" | grep -q 'text="Free sailing"' && fail "inactive opt-out Ulysses still visible" || pass "opt-out Ulysses hides after pact ends"
tap_text 'Settings'
ui features-settings
tap_text 'Ulysses'
ui features-sheet
tap_switch
ui features-confirmation
ui_has 'Turn on Ulysses?'
tap_text 'Continue'
$ADB shell input keyevent KEYCODE_BACK
ui features-enabled
tap_text 'Overview'
ui released-opted-in
ui_has 'Free sailing'
open_removal
ui removal-confirmation
ui_has 'Remove FamilyWarden?'
has no_config_date_time && pass "clock stays locked while paired" || fail "clock unlocked"

echo ""
echo "=== A new pact binds again ==="
docker exec "$ROUTER" sh -c 'echo "{\"until\":0}" | /usr/libexec/rpcd/familywarden call pact >/dev/null'
broadcast io.github.denogio.familywarden.DEBUG_SYNC
has no_config_tethering && pass "restricted again" || fail "not restricted after new pact"
# The removal confirmation was opened before the new pact: confirming it must recheck.
ui removal-race
tap_text 'Remove'
ui removal-refused
ui_has 'Removal was not authorized'
tap_text 'OK'
ui after-refusal
tap_text 'Overview'
has no_config_tethering && pass "old removal dialog cannot bypass a new pact" || fail "removal raced new pact"

echo ""
echo "=== Router unreachable: the cached pact still binds ==="
# Paused, not disconnected: the router keeps the address the phone pinned (newer
# Docker refuses to reconnect with --ip on networks without a configured subnet,
# and the emulator could not reach a custom 172.30.x subnet).
docker pause "$ROUTER" >/dev/null
$ADB reboot
sleep 10
for _ in $(seq 1 120); do
    [ "$($ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ] && break
    sleep 3
done
sleep 15
has no_config_tethering && pass "still restricted after reboot without the router" || fail "restriction lost after reboot"
ui offline
ui_has 'Hotspot is locked'
scroll_to_text 'Router out of reach; cached policy remains'
ui_has 'Router out of reach; cached policy remains'

echo ""
echo "=== Removal after a fresh signed release ==="
docker unpause "$ROUTER" >/dev/null
docker exec "$ROUTER" sh -c 'echo $(( $(date +%s) - 1 )) > /etc/familywarden/release; familywarden tick'
broadcast io.github.denogio.familywarden.DEBUG_SYNC
ui removal-allowed
ui_has 'Free sailing'
open_removal
ui removal-final-confirmation
tap_text 'Remove'
ui removed
ui_has 'One step before setting sail'
has no_config_tethering && fail "tethering restricted after removal" || pass "removal clears tethering restriction"
has no_config_date_time && fail "clock restricted after removal" || pass "removal clears clock restriction"
has disallow_config_private_dns && fail "Private DNS still locked after removal" || pass "removal unlocks Private DNS"
mode=$($ADB shell settings get global private_dns_mode | tr -d '\r')
[ "$mode" = "opportunistic" ] && pass "Private DNS back to Automatic" || fail "private_dns_mode=$mode"
$ADB shell dpm list-owners | grep -q 'DeviceOwner' && fail "still device owner" || pass "device-owner role released"

echo ""
[ "$FAILED" -eq 0 ] && echo "All emulator checks passed!" || echo "Some emulator checks FAILED"
exit "$FAILED"
