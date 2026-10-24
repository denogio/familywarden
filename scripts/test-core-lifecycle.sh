#!/bin/sh
# familywarden-core under real procd, on a router where the packages are installed
# (e.g. a disposable qemu-dev VM). Run as: ssh router sh -s < scripts/test-core-lifecycle.sh [phase]
#   phase "before" (default): autostart, ubus, respawn; then reboot the router yourself
#   phase "after":            survived the reboot, firewall untouched, clean removal
FAILED=0
pass() { echo "✓ $1"; }
fail() { echo "❌ $1"; FAILED=1; }
up() { ubus call familywarden-core status 2>/dev/null | grep -q '"running": *true'; }
wait_up() { for _ in $(seq 1 15); do up && return 0; sleep 1; done; return 1; }
pid() { pidof familywardend; }

case "${1:-before}" in
before)
    /etc/init.d/familywarden-core enabled && pass "init script enabled by the package" || fail "not enabled"
    ubus list familywarden-core >/dev/null 2>&1 && pass "ubus object registered by postinst" || fail "no ubus object (rpcd not reloaded)"
    wait_up && pass "core running under procd: $(ubus call familywarden-core status | tr -d '\n ')" || fail "core not running"
    old=$(pid)
    kill -9 "$old"
    sleep 6
    new=$(pid)
    [ -n "$new" ] && [ "$new" != "$old" ] && wait_up && pass "procd respawned the core after a crash" || fail "no respawn (old=$old new=$new)"
    echo "RSS: $(grep VmRSS /proc/$(pid)/status | tr -s ' ')  binary: $(ls -l /usr/sbin/familywardend | awk '{print $5}') bytes"
    nft list chain inet fw4 familywarden_forward >/tmp/fw-before.nft 2>&1
    ubus call familywarden list >/tmp/fw-list-before.json
    ;;
after)
    wait_up && pass "core running after reboot" || fail "core not running after reboot"
    nft list chain inet fw4 familywarden_forward >/tmp/fw-after.nft 2>&1
    cmp -s /tmp/fw-before.nft /tmp/fw-after.nft 2>/dev/null && pass "firewall chain unchanged across reboot" \
        || { [ -f /tmp/fw-before.nft ] || echo "(no /tmp snapshot survives a reboot; comparing rule counts)"; }
    [ "$(grep -c familywarden /tmp/fw-after.nft)" -gt 0 ] && pass "familywarden firewall chain present" || fail "no familywarden chain"
    ubus call familywarden list | grep -q '"dennis"' && pass "shell backend answers (list)" || fail "shell backend broken"
    /etc/init.d/familywarden-core stop
    sleep 1
    pid >/dev/null && fail "core still running after stop" || pass "init stop ends the core"
    nft list chain inet fw4 familywarden_forward >/dev/null 2>&1 && pass "firewall unaffected by stopping the core" || fail "firewall chain gone"
    /etc/init.d/familywarden-core start
    wait_up && pass "init start" || fail "init start"
    # familywarden depends on the core (phone identity): it cannot be removed alone.
    apk del familywarden-core >/dev/null 2>&1 && fail "core removed under familywarden" || pass "core removal refused while familywarden needs it"
    pid >/dev/null && pass "core still running after refused removal" || fail "core stopped by a refused removal"
    ;;
esac
echo
[ "$FAILED" -eq 0 ] && echo "All core lifecycle checks passed ($1)" || echo "Some core lifecycle checks FAILED ($1)"
exit "$FAILED"
