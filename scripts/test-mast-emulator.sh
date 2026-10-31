#!/bin/bash
# Ulysses Mast on real Android: an emulator (KVM) with the debug APK as device
# owner, paired with a throwaway FamilyWarden router container. Checks
# DNS-independent pairing, enrollment approval in the router, scoped pact
# enforcement, same-key address repair, DNS failure reporting/restoration and protection.
#
#   ./scripts/test-mast-emulator.sh <router-apk-dir> [familywarden-debug.apk]
#
# MAST_MODE=focused runs only the RB1 reboot/counting regression (scoped evidence,
# not the full installation/release gate). Default: full. See test/android/README.md.
#
# LEGACY_APK_DIR=<dir with an older familywarden .apk>: start with that router
# (global v1 replies must not authorize the phone), then upgrade it in place.
#
# Needs docker and /dev/kvm. The Android SDK (emulator + system image) lives in
# the docker volume "mast-sdk" (see the sdkmanager line below). MAST_SDK may name
# another volume or an absolute host directory (CI restores one from its cache).

set -e

MAST_MODE=${MAST_MODE:-full}
case "$MAST_MODE" in
    full) ;;
    focused) echo "=== Focused RB1 only: scoped evidence, NOT the full installation/release gate ===" ;;
    *) echo "Invalid MAST_MODE: $MAST_MODE (expected full or focused)" >&2; exit 1 ;;
esac

REPO="$(cd "$(dirname "$0")/.." && pwd)"
APK_DIR="$(cd "${1:?router apk dir}" && pwd)"
MAST_APK="$(realpath "${2:-$REPO/android/app/build/outputs/apk/debug/app-debug.apk}")"
SDK_IMAGE="ghcr.io/cirruslabs/android-sdk:35"
SYSIMG="system-images;android-35;google_apis;x86_64"
MAST_SDK="${MAST_SDK:-mast-sdk}"
export MAST_SDK
ID=$$
NET="mast-net-$ID"
ROUTER="mast-router-$ID"
EMU="mast-emu-$ID"
FAILED=0

pass() { echo "✓ $1"; }
# CI2: capture device state at the first failure; later failures often only cascade.
DIAG_DIR=${FW_MAST_DIAG_DIR:-}
snapshot() {
    [ -n "$DIAG_DIR" ] && [ -n "${ADB:-}" ] || return 0
    mkdir -p "$DIAG_DIR" || return 0
    printf '%s\n' "$1" > "$DIAG_DIR/first-failure.txt"
    $ADB logcat -d -v threadtime > "$DIAG_DIR/logcat.log" 2>&1 || true
    $ADB shell dumpsys activity services io.github.denogio.familywarden > "$DIAG_DIR/services.log" 2>&1 || true
    $ADB shell dumpsys activity activities > "$DIAG_DIR/activities.log" 2>&1 || true
    $ADB shell dumpsys window > "$DIAG_DIR/window.log" 2>&1 || true
    $ADB shell 'uiautomator dump /sdcard/fw-diag.xml >/dev/null && cat /sdcard/fw-diag.xml' > "$DIAG_DIR/ui.xml" 2>&1 || true
    $ADB exec-out screencap -p > "$DIAG_DIR/screen.png" 2>/dev/null || true
}
fail() { echo "❌ $1"; [ "$FAILED" = 1 ] || snapshot "$1"; FAILED=1; }

cleanup() {
    docker rm -f "$ROUTER" "$EMU" >/dev/null 2>&1 || true
    docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT

case "$MAST_SDK" in
    /*) mkdir -p "$MAST_SDK" ;;
    *) docker volume inspect "$MAST_SDK" >/dev/null 2>&1 || docker volume create "$MAST_SDK" >/dev/null ;;
esac
docker run --rm -v "$MAST_SDK:/opt/extra" "$SDK_IMAGE" sh -c \
    "test -d /opt/extra/system-images || { yes | sdkmanager --sdk_root=/opt/extra --licenses >/dev/null; sdkmanager --sdk_root=/opt/extra emulator platform-tools '$SYSIMG' >/dev/null; }"

docker network create "$NET" >/dev/null

echo "=== Router with an ongoing pact (captain always blocked) ==="
docker run -d --name "$ROUTER" --network "$NET" --cap-add NET_ADMIN --memory=768m --cpus=2 \
    -v "$APK_DIR:/apk:ro" -v "${LEGACY_APK_DIR:-$APK_DIR}:/legacy:ro" \
    openwrt/rootfs:x86-64-25.12.4 sleep infinity >/dev/null
docker exec -e START=$([ -n "${LEGACY_APK_DIR:-}" ] && echo legacy || echo apk) "$ROUTER" sh -c '
    mkdir -p /var/lock /var/run /var/etc
    apk update >/dev/null 2>&1
    # Releases before 0.11 have no core package; newer ones need it (phone identity)
    apk add --allow-untrusted $(ls /$START/familywarden-core-[0-9]*.apk 2>/dev/null) /$START/familywarden-[0-9]*.apk >/dev/null 2>&1 || { echo "router: apk add failed"; exit 1; }
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
    [ -x /usr/sbin/familywardend ] && { mkdir -p /var/run/familywarden-core; (familywardend >/tmp/core.log 2>&1 &); sleep 2; }
    (uhttpd -f -p 0.0.0.0:7700 -h /www-familywarden -x /cgi-bin >/dev/null 2>&1 &)
    # Valid exclusive midnight end must not make optional voyage metadata reject policy.
    echo "{\"until\":0,\"window_day\":\"sun\",\"window_start\":\"23:00\",\"window_stop\":\"24:00\"}" | /usr/libexec/rpcd/familywarden call pact >/dev/null
'
ROUTER_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$ROUTER")
KEY=$(docker exec "$ROUTER" familywarden mast-pubkey | head -1 | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g')
# router.invalid cannot resolve: successful pairing must use the numeric address.
PAIR_URI="familywarden://pair?host=router.invalid&address=$ROUTER_IP:7700&key=$KEY"
echo "router at $ROUTER_IP"

echo "=== Emulator (Android 15) ==="
docker run -d --name "$EMU" --network "$NET" --device /dev/kvm --sysctl net.ipv6.conf.lo.disable_ipv6=0 \
    --memory=4608m --memory-swap=4608m --cpus=2 \
    -v "$MAST_SDK:/opt/extra" -v "$MAST_APK:/mast.apk:ro" "$SDK_IMAGE" sleep infinity >/dev/null
docker exec "$EMU" sh -c "
    export ANDROID_HOME=/opt/extra ANDROID_SDK_ROOT=/opt/extra
    # AVD by hand: avdmanager insists on its own SDK root
    mkdir -p ~/.android/avd/t.avd
    printf 'avd.ini.encoding=UTF-8\npath=/root/.android/avd/t.avd\ntarget=android-35\n' > ~/.android/avd/t.ini
    printf '%s\n' 'image.sysdir.1=system-images/android-35/google_apis/x86_64/' 'abi.type=x86_64' \
        'hw.cpu.arch=x86_64' 'tag.id=google_apis' 'hw.ramSize=2048' 'disk.dataPartition.size=4G' \
        'hw.lcd.width=1080' 'hw.lcd.height=2400' 'hw.lcd.density=420' > ~/.android/avd/t.avd/config.ini
    (/opt/extra/emulator/emulator -avd t -no-window -no-audio -no-snapshot -no-boot-anim \
        -gpu swangle_indirect -accel on -memory 2048 -feature -ModemSimulator -feature -WiFiPacketStream >/tmp/emu.log 2>&1 &)
"
ADB="docker exec $EMU /opt/extra/platform-tools/adb"
for _ in $(seq 1 120); do
    [ "$($ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ] && break
    sleep 3
done
[ "$($ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ] || { docker exec "$EMU" tail -20 /tmp/emu.log; echo "emulator did not boot"; exit 1; }
pass "emulator booted ($($ADB shell getprop ro.build.version.release | tr -d '\r'))"
# Emulator 37's packet-stream Wi-Fi has no guest network with this API 35 image;
# use the established user-mode backend and wait for its DHCP route, not just boot.
bash "$REPO/scripts/wait-emulator-router.sh" "$EMU" "$ROUTER_IP" 7700

restrictions() { $ADB shell dumpsys user | tr -d '\r'; }
has() { restrictions | grep -A30 "UserInfo{0:" | grep -q "$1"; }
broadcast() { $ADB shell am broadcast -n io.github.denogio.familywarden/.DebugReceiver -a "$@" >/dev/null; sleep 4; }

# Smoke-test the actual Compose activity too, not just the background restriction.
# Optional screenshots: MAST_SCREENSHOT_DIR=/tmp/mast-ui ./scripts/test-mast-emulator.sh ...
# A fresh dump each time: removing the target first keeps a failed uiautomator dump
# (e.g. "null root node" right after a reboot) from re-reading a file that survives.
dump_ui() { $ADB shell rm -f /sdcard/mast-ui.xml; $ADB shell uiautomator dump /sdcard/mast-ui.xml >/dev/null; UI=$($ADB shell cat /sdcard/mast-ui.xml 2>/dev/null | tr -d '\r'); }
ui() {
    $ADB logcat -b crash -c
    $ADB shell input keyevent KEYCODE_WAKEUP
    $ADB shell wm dismiss-keyguard
    $ADB shell am start -n io.github.denogio.familywarden/.MainActivity >/dev/null
    sleep 8
    dump_ui
    # A cold start right after a reboot can take longer than 8 s to bring the Compose
    # activity to the top. Wait a bounded amount longer for it; the assertion still
    # requires the activity to actually be on top (never accept another window).
    n=0
    for _ in $(seq 1 6); do
        echo "$UI" | grep -q 'package="io.github.denogio.familywarden"' && break
        n=$((n + 1))
        sleep 3
        $ADB shell input keyevent KEYCODE_WAKEUP
        $ADB shell wm dismiss-keyguard
        $ADB shell am start -n io.github.denogio.familywarden/.MainActivity >/dev/null 2>&1 || true
        dump_ui
    done
    if [ "$n" -gt 0 ]; then
        echo "  ui $1: on top after $n retries"
    fi
    # The retry loop re-launches the app, so a crash on the first start could pass
    # after a second launch; a crash in the (cleared) buffer must still fail.
    $ADB logcat -d -b crash | grep -q 'Process: io.github.denogio.familywarden' && fail "app crashed ($1)"
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
nodes = [n for n in root.iter("node") if sys.argv[1] in (n.get("text"), n.get("content-desc"))]
if not nodes:
    sys.exit("no node for " + sys.argv[1])
node = next((n for n in nodes if n.get("clickable") == "true"), nodes[0])
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
node=next((n for n in ET.fromstring(sys.stdin.read()).iter("node") if n.get("checkable")=="true"), None)
if node is None:
    sys.exit("no checkable node for switch")
x1,y1,x2,y2=map(int,re.findall(r"\d+",node.get("bounds")))
print((x1+x2)//2,(y1+y2)//2)
') || { fail "cannot tap switch"; return 1; }
    $ADB shell input tap $coords
    sleep 3
}
scroll_to_text() {
    local text="$1"
    echo "$UI" | grep -q "text=\"$text\"" && return 0
    # Begin at the top even if a previous check scrolled the same destination.
    for _ in $(seq 1 3); do $ADB shell input swipe 540 700 540 1900 200; done
    dump_ui
    for _ in $(seq 1 6); do
        echo "$UI" | grep -q "text=\"$text\"" && return 0
        $ADB shell input swipe 540 1800 540 700 500
        sleep 1
        dump_ui
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

# Emit only endpoint and public identity hashes, never app-private key material.
pair_state() {
    $ADB exec-out run-as io.github.denogio.familywarden cat shared_prefs/familywarden.xml | python3 -c '
import hashlib,sys,xml.etree.ElementTree as E
values={n.get("name"):n.text or "" for n in E.parse(sys.stdin).getroot()}
print(values["host"],hashlib.sha256(values["pubkey"].encode()).hexdigest(),values["device_id"],sep="|")'
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
echo "  router :7700 from emulator: $($ADB shell "nc -z -w 3 $ROUTER_IP 7700 >/dev/null 2>&1 && echo open || echo CLOSED" | tr -d '\r')"
if [ -n "${LEGACY_APK_DIR:-}" ]; then
    ui legacy-unsupported
    ui_has 'Router setup needs an update'
    has no_config_tethering && fail "global captain policy applied to an unassociated phone" || pass "global v1 policy cannot restrict this phone"
    docker exec "$ROUTER" sh -c 'apk add --allow-untrusted /apk/familywarden-core-[0-9]*.apk /apk/familywarden-[0-9]*.apk >/dev/null 2>&1 && familywarden reload >/dev/null && mkdir -p /var/run/familywarden-core && (familywardend >/tmp/core.log 2>&1 &) && sleep 2' \
        && pass "router upgraded in place" || fail "router upgrade failed"
    broadcast io.github.denogio.familywarden.DEBUG_SYNC
fi
ui awaiting-approval
ui_has 'Waiting for approval'
has no_config_tethering && fail "pending phone restricted before approval" || pass "pending phone gets no policy"
# The administrator compares the fingerprint the app shows with LuCI's
APP_FP=$(printf '%s' "$UI" | grep -oE '([0-9a-f]{4} ){15}[0-9a-f]{4}' | head -1 | tr -d ' ')
PENDING=$(docker exec "$ROUTER" sh -c 'echo "{}" | /usr/libexec/rpcd/familywarden call devices')
if ! printf '%s' "$PENDING" | python3 -c 'import json,sys; sys.exit(0 if len(json.load(sys.stdin).get("pending", [])) == 1 else 1)'; then
    fail "router did not receive exactly one enrollment request"
    exit 1
fi
DEV_ID=$(echo "$PENDING" | python3 -c 'import json,sys; print(json.load(sys.stdin)["pending"][0]["id"])')
ROUTER_FP=$(echo "$PENDING" | python3 -c 'import json,sys; print(json.load(sys.stdin)["pending"][0]["fingerprint"])')
[ -n "$APP_FP" ] && [ "$APP_FP" = "$ROUTER_FP" ] && pass "app and LuCI show the same fingerprint" || fail "fingerprints differ: app=$APP_FP router=$ROUTER_FP"
out=$(docker exec "$ROUTER" sh -c "echo '{\"device\":\"$DEV_ID\",\"profile\":\"alex\",\"name\":\"Alex phone\",\"administrator\":\"1\",\"fingerprint\":\"$ROUTER_FP\"}' | /usr/libexec/rpcd/familywarden call device_approve")
printf '%s' "$out" | python3 -c 'import json,sys; sys.exit(0 if json.load(sys.stdin).get("status") == "ok" else 1)' \
    && pass "administrator approved the phone" || fail "approval: $out"
broadcast io.github.denogio.familywarden.DEBUG_SYNC
has no_config_tethering && pass "scoped pact restricts tethering" || fail "no_config_tethering missing"
if [ "$MAST_MODE" = full ]; then
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
# Sync observations moved to the core; the old shell .seen files are not authority.
docker exec "$ROUTER" sh -c 'echo "{}" | /usr/libexec/rpcd/familywarden call devices' | python3 -c '
import json,sys
sys.exit(0 if any(d["id"] == sys.argv[1] and int(d.get("seen", 0)) > 0 for d in json.load(sys.stdin).get("devices", [])) else 1)' "$DEV_ID" \
    && pass "router saw the approved phone sync" || fail "router has no sync record"

echo ""
echo "=== Same-key address repair while bound (no DNS dependency) ==="
# Use a second address on this isolated bridge, without changing the enrolled router/key.
SUBNET=$(docker network inspect -f '{{range .IPAM.Config}}{{.Subnet}}{{end}}' "$NET")
read -r REPAIR_IP REPAIR_PREFIX < <(python3 -c '
import ipaddress,sys
n=ipaddress.ip_network(sys.argv[1]); print(n.broadcast_address-2,n.prefixlen)' "$SUBNET")
docker exec "$ROUTER" ip addr add "$REPAIR_IP/$REPAIR_PREFIX" dev eth0
ORIGINAL_PAIR=$(pair_state)
REPAIR_URI="familywarden://pair?host=router.invalid&address=$REPAIR_IP:7700&key=$KEY"
# A foreign QR key must not turn repair into re-pairing during a pact.
FOREIGN_KEY=$(docker exec "$ROUTER" sh -c 'usign -G -s /tmp/foreign.key -p /tmp/foreign.pub >/dev/null && tail -1 /tmp/foreign.pub' | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g')
$ADB shell am start -a android.intent.action.VIEW -d "'familywarden://pair?host=router.invalid&address=$REPAIR_IP:7700&key=$FOREIGN_KEY'" -n io.github.denogio.familywarden/.MainActivity >/dev/null
sleep 8
[ "$(pair_state)" = "$ORIGINAL_PAIR" ] && pass "foreign QR key cannot replace a bound phone's router pin" || fail "foreign QR changed pairing"
# An unreachable endpoint must not replace the last working address either.
$ADB shell am start -a android.intent.action.VIEW -d "'familywarden://pair?host=router.invalid&address=$REPAIR_IP:7701&key=$KEY'" -n io.github.denogio.familywarden/.MainActivity >/dev/null
sleep 8
[ "$(pair_state)" = "$ORIGINAL_PAIR" ] && pass "unreachable repair endpoint leaves the last working address intact" || fail "unreachable repair changed pairing"
# Same advertised key is insufficient: the new endpoint must return a fresh signed reply.
docker exec "$ROUTER" sh -c 'cp /www-familywarden/cgi-bin/mast /tmp/mast.signed; printf "#!/bin/sh\nprintf '\''Content-Type: application/octet-stream\\r\\n\\r\\n{}'\''\n" > /www-familywarden/cgi-bin/mast'
$ADB shell am start -a android.intent.action.VIEW -d "'$REPAIR_URI'" -n io.github.denogio.familywarden/.MainActivity >/dev/null
sleep 8
[ "$(pair_state)" = "$ORIGINAL_PAIR" ] && pass "unsigned repair reply leaves endpoint, pin and identity unchanged" || fail "unsigned reply changed pairing"
has no_config_tethering && pass "failed address repair retains cached pact protection" || fail "repair failure lifted pact"
docker exec "$ROUTER" mv /tmp/mast.signed /www-familywarden/cgi-bin/mast
$ADB shell am start -a android.intent.action.VIEW -d "'$REPAIR_URI'" -n io.github.denogio.familywarden/.MainActivity >/dev/null
for _ in $(seq 1 15); do
    [ "$(pair_state)" = "$REPAIR_IP:7700|${ORIGINAL_PAIR#*|}" ] && break
    sleep 1
done
[ "$(pair_state)" = "$REPAIR_IP:7700|${ORIGINAL_PAIR#*|}" ] && pass "fresh signed same-key reply repairs the address without re-enrollment" || fail "same-key address repair failed"
has no_config_tethering && pass "successful address repair preserves active pact" || fail "address repair lifted pact"
$ADB shell am start -a android.intent.action.VIEW -d "'$PAIR_URI'" -n io.github.denogio.familywarden/.MainActivity >/dev/null
for _ in $(seq 1 15); do
    [ "$(pair_state)" = "$ORIGINAL_PAIR" ] && break
    sleep 1
done
[ "$(pair_state)" = "$ORIGINAL_PAIR" ] && pass "phone follows the same router back to its original endpoint" || fail "repair back failed"

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
# Navigation labels are visible text: an opted-out Ulysses tab must be gone.
echo "$UI" | grep -q 'text="Ulysses"' && fail "inactive opt-out Ulysses still visible" || pass "opt-out Ulysses hides after pact ends"
echo "$UI" | grep -q 'text="Lashed to the mast' && fail "pact card shown after release" || pass "no pact card after release"
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
tap_text 'Ulysses'
ui released-opted-in-ulysses
ui_has 'The sea is calm'
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
# The overview's single status chip reports the outage; the cached pact still binds above.
# The chip reads "Syncing…" until the startup sync gives up, so wait for its verdict.
for _ in $(seq 1 10); do
    echo "$UI" | grep -q 'text="Offline · updated ' && break
    sleep 3
    dump_ui
done
echo "$UI" | grep -q 'text="Offline · updated ' && pass "UI: offline status chip" || fail "UI missing: offline status chip"

echo ""
echo "=== Removal after a fresh signed release ==="
docker unpause "$ROUTER" >/dev/null
# The freshly rebooted guest may not have its network back yet; one sync attempt
# before that fails with "router unreachable" (dispatch 41044677202).
bash "$REPO/scripts/wait-emulator-router.sh" "$EMU" "$ROUTER_IP" 7700 || fail "phone cannot reach the router after reboot"
docker exec "$ROUTER" sh -c 'echo $(( $(date +%s) - 1 )) > /etc/familywarden/release; familywarden tick'
broadcast io.github.denogio.familywarden.DEBUG_SYNC
ui removal-allowed
echo "$UI" | grep -q 'text="Hotspot is locked"' && fail "hotspot still shown locked after release" || pass "released: hotspot no longer locked"
tap_text 'Ulysses'
ui removal-allowed-ulysses
ui_has 'The sea is calm'

echo ""
else
# The active pact freezes app settings; release only this disposable test router.
echo "=== Focused prerequisite: fresh signed release ==="
docker exec "$ROUTER" sh -c 'echo $(( $(date +%s) - 1 )) > /etc/familywarden/release; familywarden tick'
broadcast io.github.denogio.familywarden.DEBUG_SYNC
has no_config_tethering && fail "tethering still restricted after release" || pass "tethering allowed again"
fi

echo "=== App limits from the signed v3 policy ==="
# A launchable, non-essential app of the system image stands in for a child's game.
TARGET=""
for p in com.android.camera2 com.android.contacts com.google.android.contacts; do
    $ADB shell pm path "$p" 2>/dev/null | grep -q package: && { TARGET=$p; break; }
done
suspended() { $ADB shell dumpsys package "$1" | tr -d '\r' | grep -q 'suspended=true'; }
rdev() { docker exec "$ROUTER" sh -c "echo '{}' | /usr/libexec/rpcd/familywarden call devices" | docker exec -i "$ROUTER" jsonfilter -e "@.devices[0].$1"; }
# Usage access before the first sample: a day with a usage gap is never acknowledged.
$ADB shell appops set --uid io.github.denogio.familywarden GET_USAGE_STATS allow
docker exec -e TARGET="$TARGET" "$ROUTER" sh -c '
    uci set familywarden.alex.apps=1
    uci set familywarden.alex_game=app
    uci set familywarden.alex_game.profile=alex
    uci set familywarden.alex_game.package="$TARGET"
    uci set familywarden.alex_game.blocked=1
    uci commit familywarden
    familywarden reload >/dev/null'
broadcast io.github.denogio.familywarden.DEBUG_SYNC
sleep 6
[ -n "$TARGET" ] && suspended "$TARGET" && pass "router-blocked app suspended ($TARGET)" || fail "blocked app not suspended (${TARGET:-no target})"
$ADB shell dumpsys activity services io.github.denogio.familywarden | grep -q AppControlService \
    && pass "app-control service runs while the policy has apps" || fail "app-control service not running"
broadcast io.github.denogio.familywarden.DEBUG_SYNC
rev=$(rdev apps_rev)
[ -n "$rev" ] && [ "$(rdev apps_applied)" = "$rev" ] && pass "router got the signed acknowledgement ($rev)" \
    || fail "acknowledgement: applied='$(rdev apps_applied)' expected='$rev'"

echo "=== A reboot mid-day debits 15 seconds and keeps counting ==="
# Run after signed release and app-policy acknowledgement: a live pact freezes apps settings.
# Keep the separate raw adb reboot offline-pact test above.
$ADB shell appops set --uid io.github.denogio.familywarden GET_USAGE_STATS allow
$ADB shell settings put global auto_time 1
broadcast io.github.denogio.familywarden.DEBUG_SYNC
sleep 6
REBOOT_BASELINE=$($ADB exec-out run-as io.github.denogio.familywarden cat files/app-usage.json)
[ -n "$REBOOT_BASELINE" ] || { fail "collector has no checkpoint before reboot"; exit 1; }
$ADB reboot
sleep 10
for _ in $(seq 1 120); do
    [ "$($ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ] && break
    sleep 3
done
sleep 15
bash "$REPO/scripts/wait-emulator-router.sh" "$EMU" "$ROUTER_IP" 7700 || fail "phone cannot reach router after reboot"
broadcast io.github.denogio.familywarden.DEBUG_SYNC
sleep 6
$ADB exec-out run-as io.github.denogio.familywarden cat shared_prefs/familywarden.xml | python3 -c '
import sys,xml.etree.ElementTree as E
values={n.get("name"):n.text or "" for n in E.parse(sys.stdin).getroot()}
sys.exit(0 if values.get("apps_status") == "ok" else 1)' \
    && pass "reboot status is ok after signed sync" || fail "reboot made the day uncounted"
# A cached apps_status=ok is not proof that the collector sampled this boot.
CURRENT_BOOT=$($ADB shell settings get global boot_count | tr -d '\r')
$ADB exec-out run-as io.github.denogio.familywarden cat files/app-usage.json | python3 -c '
import datetime,json,sys,zoneinfo
before=json.loads(sys.argv[1])
after=json.load(sys.stdin)
today=datetime.datetime.now(zoneinfo.ZoneInfo(after["zone"])).date().isoformat()
assert after["boot"] == int(sys.argv[2]) > before["boot"], "no sample on the new boot"
assert after["seq"] > before["seq"], "checkpoint did not advance"
assert after["cursor"] > before["cursor"], "usage cursor did not advance"
assert after["gaps"] == before["gaps"] == 0, "reboot introduced a usage gap"
assert after["uncertain_through_day"] < today, "today is not reliable"
debit=lambda checkpoint: checkpoint.get("reboot_debits",{}).get(today,{}).get("total",0)
assert debit(after) - debit(before) == 15000, "reboot debit missing or duplicated"
# Measured foreground growth is checked separately below, never inferred from debit.
' "$REBOOT_BASELINE" "$CURRENT_BOOT" \
    && pass "reboot checkpoint advanced without a gap" || fail "reboot checkpoint is stale or unreliable"
COUNT_TARGET=""
for p in com.android.camera2 com.android.contacts com.google.android.contacts; do
    if $ADB shell pm path "$p" 2>/dev/null | grep -q package: && ! suspended "$p"; then
        COUNT_TARGET=$p; break
    fi
done
POST_REBOOT=$($ADB exec-out run-as io.github.denogio.familywarden cat files/app-usage.json)
[ -n "$COUNT_TARGET" ] || fail "no foreground app for reboot counting check"
$ADB shell input keyevent KEYCODE_WAKEUP
$ADB shell wm dismiss-keyguard
$ADB shell monkey -p "$COUNT_TARGET" -c android.intent.category.LAUNCHER 1 >/dev/null 2>&1
# Sampling runs every 15s and may be delayed under CI load; wait for actual progress.
COUNTING_RESUMED=0
for _ in $(seq 1 20); do
    sleep 2
    AFTER_REBOOT=$($ADB exec-out run-as io.github.denogio.familywarden cat files/app-usage.json)
    if printf '%s' "$AFTER_REBOOT" | python3 -c '
import json,sys
before=json.loads(sys.argv[1]); after=json.load(sys.stdin); package=sys.argv[2]
used=lambda checkpoint: sum(day.get(package,0) for day in checkpoint["days"].values())
assert after["seq"] > before["seq"], "no fresh sample after launch"
assert used(after) > used(before), "foreground usage did not increase after reboot"
assert after["gaps"] == before["gaps"] == 0, "counting introduced a gap"
assert after["reboot_debits"] == before["reboot_debits"], "same-boot sample debited again"
' "$POST_REBOOT" "$COUNT_TARGET" 2>/dev/null; then
        COUNTING_RESUMED=1; break
    fi
done
if [ "$COUNTING_RESUMED" = 1 ]; then
    pass "foreground time keeps counting after reboot"
else
    printf 'Foreground target: %s\nBefore: %s\nAfter: %s\n' "$COUNT_TARGET" "$POST_REBOOT" "$AFTER_REBOOT"
    fail "foreground time stopped counting after reboot"
fi

# Daily total used up (0 minutes): every ordinary app pauses, calls and the alarm clock do not.
if [ "$MAST_MODE" = full ]; then
OTHER=""
for p in com.android.chrome com.google.android.calendar com.android.calendar com.google.android.contacts com.android.contacts; do
    [ "$p" != "$TARGET" ] && $ADB shell pm path "$p" 2>/dev/null | grep -q package: && { OTHER=$p; break; }
done
DIALER=$($ADB shell telecom get-default-dialer 2>/dev/null | tr -d '\r')
CLOCK=""
for p in com.google.android.deskclock com.android.deskclock; do $ADB shell pm path "$p" 2>/dev/null | grep -q package: && { CLOCK=$p; break; }; done
docker exec "$ROUTER" sh -c 'uci set familywarden.alex.screen_minutes=0; uci commit familywarden; familywarden reload >/dev/null'
broadcast io.github.denogio.familywarden.DEBUG_SYNC
sleep 6
suspended "$OTHER" && pass "daily limit pauses an ordinary app ($OTHER)" || fail "daily limit did not pause ${OTHER:-no app}"
[ -n "$DIALER" ] && ! suspended "$DIALER" && pass "phone app stays usable ($DIALER)" || fail "phone app paused or unknown ($DIALER)"
[ -z "$CLOCK" ] || { suspended "$CLOCK" && fail "alarm clock paused" || pass "alarm clock stays usable ($CLOCK)"; }
$ADB shell dumpsys notification --noredact | tr -d '\r' | grep -q 'Screen time is up for today' \
    && pass "notification: screen time is up" || fail "no 'screen time is up' notification"
out=$(docker exec "$ROUTER" sh -c "echo '{\"profile\":\"alex\",\"minutes\":30}' | /usr/libexec/rpcd/familywarden call bonus")
echo "$out" | grep -q '"minutes_today": 30' && pass "router: bonus +30 for today" || fail "bonus: $out"
broadcast io.github.denogio.familywarden.DEBUG_SYNC
sleep 6
suspended "$OTHER" && fail "bonus did not lift the daily-limit pause" || pass "bonus lifts the daily-limit pause"
suspended "$TARGET" && pass "blocked app stays blocked despite bonus" || fail "bonus lifted a blocked app"
$ADB shell appops set --uid io.github.denogio.familywarden GET_USAGE_STATS ignore
broadcast io.github.denogio.familywarden.DEBUG_SYNC
sleep 4
suspended "$TARGET" && pass "block stays without usage access" || fail "block lifted when usage access was revoked"
ui apps-usage-missing
ui_has 'Allow usage access'
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
suspended "$TARGET" && fail "app still suspended after removal" || pass "removal lifts FamilyWarden's app suspensions"

fi

echo ""
if [ "$MAST_MODE" = focused ]; then
    [ "$FAILED" -eq 0 ] && echo "Focused RB1 checks passed (scoped evidence only; NOT the full installation/release gate)." || echo "Focused RB1 checks FAILED"
else
    [ "$FAILED" -eq 0 ] && echo "All emulator checks passed!" || echo "Some emulator checks FAILED"
fi
exit "$FAILED"
