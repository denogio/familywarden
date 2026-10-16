#!/bin/bash
# Ulysses Mast on real Android: an emulator (KVM) with the debug APK as device
# owner, paired with a throwaway FamilyWarden router container. Checks that the
# tethering restriction follows the pact, and that Android protects the app.
#
#   ./scripts/test-mast-emulator.sh <router-apk-dir> [mast-debug.apk]
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
    -v "$APK_DIR:/apk:ro" openwrt/rootfs:x86-64-25.12.4 sleep infinity >/dev/null
docker exec "$ROUTER" sh -c '
    mkdir -p /var/lock /var/run /var/etc
    apk update >/dev/null 2>&1
    apk add --allow-untrusted /apk/familywarden-[0-9]*.apk >/dev/null 2>&1 || { echo "router: apk add failed"; exit 1; }
    cat > /etc/config/familywarden <<EOF
config global settings
	option enabled 1
	option ulysses_profile dennis
	option ulysses_delay 1
config profile dennis
	option name Dennis
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
PAIR_URI="ulyssesmast://pair?host=$ROUTER_IP:7700&key=$KEY"
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
broadcast() { $ADB shell am broadcast -n me.hojgaard.ulysses.mast/.DebugReceiver -a "$@" >/dev/null; sleep 4; }

# Smoke-test the actual Compose activity too, not just the background restriction.
# Optional screenshots: MAST_SCREENSHOT_DIR=/tmp/mast-ui ./scripts/test-mast-emulator.sh ...
ui() {
    $ADB shell input keyevent KEYCODE_WAKEUP
    $ADB shell wm dismiss-keyguard
    $ADB shell am start -n me.hojgaard.ulysses.mast/.MainActivity >/dev/null
    sleep 8
    $ADB shell uiautomator dump /sdcard/mast-ui.xml >/dev/null
    UI=$($ADB shell cat /sdcard/mast-ui.xml | tr -d '\r')
    echo "$UI" | grep -q 'package="me.hojgaard.ulysses.mast"' && pass "Compose activity renders ($1)" || fail "activity failed ($1)"
    if [ -n "${MAST_SCREENSHOT_DIR:-}" ]; then
        mkdir -p "$MAST_SCREENSHOT_DIR"
        $ADB exec-out screencap -p > "$MAST_SCREENSHOT_DIR/$1.png"
    fi
}
ui_has() { echo "$UI" | grep -q "text=\"$1\"" && pass "UI: $1" || fail "UI missing: $1"; }
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

$ADB install -r /mast.apk >/dev/null && pass "APK installed (sideloaded, not from Play)" || fail "install"
# Android checks for accounts asynchronously after boot (hasIncompatibleAccountsTask);
# until that has finished, set-device-owner is refused. Retry for a bit.
for _ in $(seq 1 12); do
    out=$($ADB shell dpm set-device-owner me.hojgaard.ulysses.mast/.MastAdminReceiver 2>&1 | tr -d '\r')
    echo "$out" | grep -q "Success" && break
    sleep 5
done
echo "$out" | grep -q "Success" && pass "became device owner" || fail "dpm set-device-owner: $(echo "$out" | head -2)"

$ADB shell settings put system screen_off_timeout 1800000
ui unpaired
ui_has 'Pair with your router'

echo ""
echo "=== Pact active, captain blocked ==="
# Exercise the real pairing UI; the key is pinned only after fingerprint confirmation.
# Quoted for the device shell too: the URI contains "&".
$ADB shell am start -a android.intent.action.VIEW -d "'$PAIR_URI'" -n me.hojgaard.ulysses.mast/.MainActivity >/dev/null
ui pairing-confirmation
ui_has 'Confirm the router'
tap_text 'It matches, pair'
$ADB logcat -d -s UlyssesMast:* AndroidRuntime:E | tr -d '\r' | grep -A4 -E "debug:|FATAL" | head -8 | sed 's/^/  log: /'
echo "  router :7700 from emulator: $($ADB shell "echo | nc -w 3 $ROUTER_IP 7700 >/dev/null 2>&1 && echo open || echo CLOSED" | tr -d '\r')"
has no_config_tethering && pass "tethering restricted" || fail "no_config_tethering missing"
ui blocked-light
ui_has 'Hotspot is locked'
echo "$UI" | grep -q 'text="Remove Mast"' && fail "removal offered while bound" || pass "UI hides removal while bound"
$ADB shell cmd uimode night yes
ui blocked-dark
ui_has 'Hotspot is locked'
$ADB shell settings put system font_scale 2.0
ui blocked-dark-large
ui_has 'Hotspot is locked'
$ADB shell settings put system font_scale 1.0
$ADB shell cmd uimode night no
has no_config_date_time && pass "clock locked while paired" || fail "no_config_date_time missing"
docker exec "$ROUTER" sh -c 'cat /tmp/familywarden.mast' | grep -q "^mast-" && pass "router saw the phone sync" || fail "router has no sync record"

out=$($ADB shell pm clear me.hojgaard.ulysses.mast 2>&1 | tr -d '\r')
echo "$out" | grep -qi "protected\|SecurityException\|Failed" && pass "pm clear refused ($out)" || fail "pm clear allowed: $out"
out=$($ADB uninstall me.hojgaard.ulysses.mast 2>&1 | tr -d '\r')
echo "$out" | grep -qi "DELETE_FAILED\|Failure" && pass "uninstall refused ($out)" || fail "uninstall allowed: $out"
out=$($ADB shell am force-stop me.hojgaard.ulysses.mast 2>&1; $ADB shell pidof me.hojgaard.ulysses.mast)
has no_config_tethering && pass "restriction survives force-stop" || fail "restriction gone after force-stop"

echo ""
echo "=== Released on the sail page ==="
docker exec "$ROUTER" sh -c 'echo $(( $(date +%s) - 1 )) > /etc/familywarden/release; familywarden tick'
broadcast me.hojgaard.ulysses.mast.DEBUG_SYNC
has no_config_tethering && fail "tethering still restricted after release" || pass "tethering allowed again"
ui released
ui_has 'Free sailing'
tap_text 'Remove Mast'
ui removal-confirmation
ui_has 'Remove the Mast?'
has no_config_date_time && pass "clock stays locked while paired" || fail "clock unlocked"

echo ""
echo "=== A new pact binds again ==="
docker exec "$ROUTER" sh -c 'echo "{\"until\":0}" | /usr/libexec/rpcd/familywarden call pact >/dev/null'
broadcast me.hojgaard.ulysses.mast.DEBUG_SYNC
has no_config_tethering && pass "restricted again" || fail "not restricted after new pact"
# The removal confirmation was opened before the new pact: confirming it must recheck.
ui removal-race
tap_text 'Remove'
ui removal-refused
ui_has 'The pact still holds'
tap_text 'OK'
has no_config_tethering && pass "old removal dialog cannot bypass a new pact" || fail "removal raced new pact"

echo ""
echo "=== Router unreachable: the cached pact still binds ==="
docker network disconnect "$NET" "$ROUTER"
$ADB reboot
sleep 10
for _ in $(seq 1 120); do
    [ "$($ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ] && break
    sleep 3
done
sleep 15
has no_config_tethering && pass "still restricted after reboot without the router" || fail "restriction lost after reboot"
ui offline
ui_has 'Router out of reach'
ui_has 'Hotspot is locked'

echo ""
echo "=== Removal after a fresh signed release ==="
docker network connect --ip "$ROUTER_IP" "$NET" "$ROUTER"
docker exec "$ROUTER" sh -c 'echo $(( $(date +%s) - 1 )) > /etc/familywarden/release; familywarden tick'
broadcast me.hojgaard.ulysses.mast.DEBUG_SYNC
ui removal-allowed
ui_has 'Free sailing'
tap_text 'Remove Mast'
ui removal-final-confirmation
tap_text 'Remove'
ui removed
ui_has 'One step before setting sail'
has no_config_tethering && fail "tethering restricted after removal" || pass "removal clears tethering restriction"
has no_config_date_time && fail "clock restricted after removal" || pass "removal clears clock restriction"
$ADB shell dpm list-owners | grep -q 'DeviceOwner' && fail "still device owner" || pass "device-owner role released"

echo ""
[ "$FAILED" -eq 0 ] && echo "All emulator checks passed!" || echo "Some emulator checks FAILED"
exit "$FAILED"
