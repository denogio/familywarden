#!/bin/bash
# FamilyWarden test router in QEMU: real OpenWrt 25.12 with procd, fw4, LuCI
# and reboots. No root/sudo needed (user-mode networking).
#
#   ./scripts/qemu-dev.sh start [--fresh]   boot the VM (fresh = discard all changes)
#   ./scripts/qemu-dev.sh deploy [apk-dir]  build (or take) apks, install + test config
#   ./scripts/qemu-dev.sh time "21:29"      set the router clock ("YYYY-MM-DD HH:MM" works too)
#   ./scripts/qemu-dev.sh ssh [cmd]         shell on the router (closed during a lockdown!)
#   ./scripts/qemu-dev.sh console           serial console = physical access, works always
#   ./scripts/qemu-dev.sh run "cmd"         run a command via the serial console
#   ./scripts/qemu-dev.sh reboot | stop | status
#
# Your machine is a LAN client (192.168.1.2) of the router (192.168.1.1), so
# Ulysses lockdowns hit it like any other device:
#   LuCI       http://localhost:8080   (root / ulysses)
#   Sail page  http://localhost:7700
#   SSH        ssh -p 2222 root@localhost

set -e

REPO="$(cd "$(dirname "$0")/.." && pwd)"
VERSION=25.12.4
IMG_DIR="$REPO/images"
BASE="$IMG_DIR/openwrt-$VERSION-x86-64-base.img"
OVERLAY="$IMG_DIR/qemu-dev.qcow2"
RUN="$IMG_DIR/run"
CONSOLE="$RUN/console.sock"
MONITOR="$RUN/monitor.sock"
PIDFILE="$RUN/qemu.pid"
URL="https://downloads.openwrt.org/releases/$VERSION/targets/x86/64"
GZ="openwrt-$VERSION-x86-64-generic-ext4-combined.img.gz"
PASSWORD="ulysses"
# Dedicated key for the test VM, installed while root still has no password
KEY="$RUN/id_ed25519"
SSH=(ssh -p 2222 -i "$KEY" -o IdentitiesOnly=yes -o BatchMode=yes -o StrictHostKeyChecking=no
     -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=5 root@localhost)

die() { echo "❌ $*" >&2; exit 1; }
running() { [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; }

fetch_base() {
    [ -f "$BASE" ] && return
    echo "Downloading OpenWrt $VERSION (x86-64)..."
    mkdir -p "$IMG_DIR"
    local tmp
    tmp=$(mktemp -d)
    wget -q -O "$tmp/sha256sums" "$URL/sha256sums"
    wget -q -O "$tmp/$GZ" "$URL/$GZ"
    (cd "$tmp" && grep " \*$GZ\$" sha256sums | sha256sum -c - >/dev/null) || die "checksum mismatch"
    gunzip -c "$tmp/$GZ" > "$BASE" 2>/dev/null || true   # OpenWrt images carry trailing padding
    rm -rf "$tmp"
    chmod a-w "$BASE"
}

# Send a command to the serial console (root shell, no login on OpenWrt x86).
# Enter first to activate the console, and pauses so the shell is ready.
console_run() {
    { printf '\n'; sleep 1; printf '%s\n' "$1"; sleep "${2:-2}"; } | timeout $(( ${2:-2} + 5 )) ncat -U "$CONSOLE" >/dev/null 2>&1 || true
}

wait_ssh() {
    echo -n "Waiting for the router"
    for _ in $(seq 1 90); do
        if "${SSH[@]}" true 2>/dev/null; then
            echo " up."
            # A fresh router lets root in without a password ("none" auth):
            # install the key now so SSH keeps working once deploy sets one
            "${SSH[@]}" "grep -qF '$(cut -d' ' -f2 "$KEY.pub")' /etc/dropbear/authorized_keys 2>/dev/null \
                || cat >> /etc/dropbear/authorized_keys" < "$KEY.pub"
            return 0
        fi
        echo -n "."; sleep 2
    done
    echo ""; die "no SSH after 3 minutes (try: $0 console)"
}

cmd_start() {
    running && { echo "Already running (pid $(cat "$PIDFILE"))."; return; }
    fetch_base
    mkdir -p "$RUN"
    [ -f "$KEY" ] || ssh-keygen -q -t ed25519 -N '' -C familywarden-qemu-dev -f "$KEY"
    if [ "$1" = "--fresh" ] || [ ! -f "$OVERLAY" ]; then
        rm -f "$OVERLAY"
        qemu-img create -q -f qcow2 -b "$BASE" -F raw "$OVERLAY"
        echo "Fresh router (all changes live in $OVERLAY)."
    fi
    # LAN: user-mode net shaped like a real LAN, host = 192.168.1.2.
    # WAN: second user-mode net with DHCP, so the router has internet.
    qemu-system-x86_64 \
        -enable-kvm -m 256 -smp 2 \
        -drive "file=$OVERLAY,if=virtio,format=qcow2" \
        -netdev "user,id=lan,net=192.168.1.0/24,host=192.168.1.2,dhcpstart=192.168.1.200,hostfwd=tcp:127.0.0.1:8080-192.168.1.1:80,hostfwd=tcp:127.0.0.1:7700-192.168.1.1:7700,hostfwd=tcp:127.0.0.1:2222-192.168.1.1:22" \
        -device virtio-net-pci,netdev=lan \
        -netdev user,id=wan \
        -device virtio-net-pci,netdev=wan \
        -serial "unix:$CONSOLE,server=on,wait=off" \
        -monitor "unix:$MONITOR,server=on,wait=off" \
        -display none -daemonize -pidfile "$PIDFILE"
    wait_ssh
    echo "LuCI http://localhost:8080 · sail page http://localhost:7700 · ssh -p 2222 root@localhost"
}

cmd_deploy() {
    running || die "not running (start it first)"
    local dir="$1"
    if [ -z "$dir" ]; then
        dir="$RUN/apk"
        rm -rf "$dir"
        echo "Building apks..."
        OUT="$dir" "$REPO/scripts/build-apk-docker.sh" > "$RUN/build.log" 2>&1 || die "build failed, see $RUN/build.log"
    fi
    local fw luci
    fw=$(ls "$dir"/familywarden-[0-9]*.apk | sort -V | tail -1)
    luci=$(ls "$dir"/luci-app-familywarden-[0-9]*.apk | sort -V | tail -1)
    echo "Installing $(basename "$fw") + $(basename "$luci")"
    "${SSH[@]}" "cat > /tmp/fw.apk" < "$fw"
    "${SSH[@]}" "cat > /tmp/luci.apk" < "$luci"
    "${SSH[@]}" sh -s <<EOF
set -e
apk add --allow-untrusted /tmp/fw.apk /tmp/luci.apk >/dev/null
# Danish time, a known LuCI password
uci set system.@system[0].zonename='Europe/Copenhagen'
uci set system.@system[0].timezone='CET-1CEST,M3.5.0,M10.5.0/3'
uci commit system
/etc/init.d/system reload
printf '%s\n%s\n' '$PASSWORD' '$PASSWORD' | passwd root >/dev/null 2>&1
# Test profiles (only on first deploy): Dennis is the Ulysses captain
if ! uci -q get familywarden.dennis >/dev/null; then
    cat > /etc/config/familywarden <<'CFG'
config global 'settings'
	option enabled '1'
	option default_policy 'allow'
	option ulysses_profile 'dennis'
	option ulysses_delay '1'

config profile 'dennis'
	option name 'Dennis'
	option enabled '1'
	option policy 'scheduled'
	option block_router '1'
	list device 'aa:bb:cc:00:00:01'

config schedule
	option profile 'dennis'
	list days 'mon'
	list days 'tue'
	list days 'wed'
	list days 'thu'
	list days 'fri'
	list days 'sat'
	list days 'sun'
	option start '06:00'
	option stop '21:30'

config profile 'josva'
	option name 'Josva'
	option enabled '1'
	option policy 'allow'
	list device 'aa:bb:cc:00:00:02'
CFG
fi
/etc/init.d/familywarden enable
/etc/init.d/familywarden restart
rm -rf /tmp/luci-indexcache* /tmp/luci-modulecache
/etc/init.d/rpcd restart
EOF
    echo "✓ Deployed. LuCI http://localhost:8080 (root / $PASSWORD), sail page http://localhost:7700"
    echo "  Cooling-off is 1 hour in the test config. Router time: $("${SSH[@]}" "date '+%a %H:%M'")"
}

cmd_time() {
    running || die "not running"
    [ -n "$1" ] || die "usage: $0 time \"21:29\" | \"YYYY-MM-DD HH:MM\""
    local t="$1"
    case "$t" in *-*) ;; *) t="$("${SSH[@]}" date +%Y-%m-%d 2>/dev/null || date +%Y-%m-%d) $t" ;; esac
    # Stop NTP from undoing it, then set the clock and reload the rules.
    # Via the console, so it also works during a lockdown.
    console_run "uci set system.ntp.enabled=0; uci commit system; /etc/init.d/sysntpd stop; date -s '$t:00'; familywarden reload" 4
    echo "Router clock set to $t (NTP off; '$0 time now' turns it back on)."
}

cmd_time_now() {
    console_run "uci set system.ntp.enabled=1; uci commit system; /etc/init.d/sysntpd restart; sleep 3; familywarden reload" 6
    echo "NTP back on."
}

cmd_stop() {
    running || { echo "Not running."; return; }
    printf 'quit\n' | ncat -U --send-only "$MONITOR" 2>/dev/null || kill "$(cat "$PIDFILE")"
    rm -f "$PIDFILE"
    echo "Stopped (state kept in $OVERLAY; 'start --fresh' discards it)."
}

case "$1" in
    start)   shift; cmd_start "$@" ;;
    deploy)  shift; cmd_deploy "$@" ;;
    time)    shift; if [ "$1" = "now" ]; then cmd_time_now; else cmd_time "$@"; fi ;;
    ssh)     shift; exec "${SSH[@]}" "$@" ;;
    console) running || die "not running"
             echo "Serial console (physical access). Press Enter; Ctrl-C to leave."
             exec ncat -U "$CONSOLE" ;;
    run)     shift; running || die "not running"; console_run "$*" ;;
    reboot)  running || die "not running"; console_run "reboot" 1; sleep 5; wait_ssh ;;
    stop)    cmd_stop ;;
    status)  if running; then echo "Running (pid $(cat "$PIDFILE"))."; else echo "Not running."; fi ;;
    *)       sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'; exit 1 ;;
esac
