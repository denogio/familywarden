#!/bin/bash
# Focused P8: actual signed router policy, child grants, then a non-Device-Owner parent.
# Disposable Android 15 + OpenWrt only. Never accepts a physical adb target.
# Usage: scripts/test-requests-emulator.sh <router-apk-dir> [debug.apk]
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
APKS=$(realpath "${1:?router apk directory}")
APP=$(realpath "${2:-$ROOT/android/app/build/outputs/apk/debug/app-debug.apk}")
OUT=${OUT:-$(mktemp -d /tmp/fw-requests-XXXXXX)}
mkdir -p "$OUT/probe"
SDK_IMAGE=ghcr.io/cirruslabs/android-sdk:35
ID=$$
ROUTER=fw-requests-router-$ID
EMU=fw-requests-emu-$ID
NET=fw-requests-net-$ID
cleanup() {
    if [ "${FW_REQUESTS_KEEP:-0}" = 1 ]; then
        printf 'Kept test resources: %s %s %s\n' "$ROUTER" "$EMU" "$NET"
    else
        docker rm -f "$EMU" "$ROUTER" >/dev/null 2>&1 || true
        docker network rm "$NET" >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT
printf 'OUT=%q\nROUTER=%q\nEMU=%q\nNET=%q\n' "$OUT" "$ROUTER" "$EMU" "$NET" > "$OUT/session.env"
docker run --rm --memory=1g --cpus=2 -v "$ROOT:/src:ro" -v "$OUT/probe:/out" \
    -v mast-android-debugkey:/root/.android "$SDK_IMAGE" sh /src/test/android/install-probe/build.sh > "$OUT/probe-build.log" 2>&1
docker network create "$NET" >/dev/null
docker run -d --name "$ROUTER" --network "$NET" --cap-add NET_ADMIN \
    -v "$APKS:/apk:ro" openwrt/rootfs:x86-64-25.12.4 sleep infinity >/dev/null
# Bound feed stalls on the host; this minimal OpenWrt image has no timeout command.
timeout --kill-after=10 180 docker exec "$ROUTER" sh -ec '
    mkdir -p /var/lock /var/run /var/etc
    apk update >/dev/null 2>&1
    apk add --allow-untrusted /apk/familywarden-core-[0-9]*.apk /apk/familywarden-[0-9]*.apk >/tmp/apk.log 2>&1
    cat > /etc/config/familywarden <<EOF
config global settings
 option enabled 1
 option default_policy allow
config profile alex
 option name Alex
 option enabled 1
 option policy block
 option apps 1
 option apps_suspend_new 1
 list device aa:bb:cc:00:00:01
config profile parent
 option name Taylor
 option enabled 0
 option policy allow
EOF
    touch /etc/config/system /etc/config/network
    uci set network.lan=interface
    uci set network.lan.device=br-lan
    uci set network.lan.proto=static
    uci commit network
    uci -q get system.@system[0] >/dev/null || uci set system.clock=system
    uci set system.@system[0].zonename=UTC
    uci set system.@system[0].timezone=UTC
    uci commit system
    uci set firewall.@defaults[0].input=ACCEPT
    uci commit firewall
    fw4 start >/dev/null 2>&1
    familywarden reload >/dev/null
    familywarden mast-pubkey >/dev/null
    mkdir -p /var/run/familywarden-core
    (familywardend >/tmp/core.log 2>&1 &)
    (uhttpd -f -p 0.0.0.0:7700 -h /www-familywarden -x /cgi-bin >/tmp/http.log 2>&1 &)
'
IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$ROUTER")
docker run -d --name "$EMU" --network "$NET" --device /dev/kvm --cpus=2 \
    --memory=4500m --memory-swap=4500m --sysctl net.ipv6.conf.lo.disable_ipv6=0 \
    -v mast-sdk:/opt/extra -v "$ROOT:/src:ro" -v "$APP:/app.apk:ro" -v "$OUT/probe:/probe:ro" \
    "$SDK_IMAGE" sleep infinity >/dev/null
docker exec "$EMU" sh -c '
    test -f /opt/extra/system-images/android-35/google_apis/x86_64/system.img
    export ANDROID_HOME=/opt/extra ANDROID_SDK_ROOT=/opt/extra
    mkdir -p /root/.android/avd/p8.avd
    printf "avd.ini.encoding=UTF-8\npath=/root/.android/avd/p8.avd\ntarget=android-35\n" > /root/.android/avd/p8.ini
    printf "%s\n" image.sysdir.1=system-images/android-35/google_apis/x86_64/ abi.type=x86_64 \
        hw.cpu.arch=x86_64 tag.id=google_apis hw.ramSize=2048 disk.dataPartition.size=4G \
        hw.lcd.width=1080 hw.lcd.height=2400 hw.lcd.density=420 > /root/.android/avd/p8.avd/config.ini
    (/opt/extra/emulator/emulator -avd p8 -no-window -no-audio -no-snapshot -no-boot-anim \
        -gpu swangle_indirect -accel on -memory 2048 -cores 2 -feature -ModemSimulator >/tmp/emu.log 2>&1 &)
'
python3 "$ROOT/test/android/request_flow.py" --router "$ROUTER" --emulator "$EMU" --address "$IP:7700" --out "$OUT"
echo "P8 native checks passed; artifacts: $OUT"
