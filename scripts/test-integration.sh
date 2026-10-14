#!/bin/sh
# Integration test: runs the real rpcd plugin against real fw4 + nftables in an
# OpenWrt rootfs container, then asserts on the live ruleset and ubus replies.
#
#   ./scripts/test-integration.sh              # host: starts the container
#   OPENWRT_IMAGE=openwrt/rootfs:x86-64-25.12.4 ./scripts/test-integration.sh
#
# Needs docker and CAP_NET_ADMIN (nftables runs in the container's own netns).

if [ "$1" != "--inside" ]; then
    IMAGE="${OPENWRT_IMAGE:-openwrt/rootfs:x86-64-25.12.4}"
    REPO="$(cd "$(dirname "$0")/.." && pwd)"
    exec docker run --rm --cap-add NET_ADMIN -v "$REPO:/src:ro" "$IMAGE" \
        sh /src/scripts/test-integration.sh --inside
fi

# ---- Inside the OpenWrt container ----

BIN=/usr/libexec/rpcd/familywarden
FAILED=0

pass() { echo "✓ $1"; }
fail() { echo "❌ $1"; FAILED=1; }

# assert_rule <description> <pattern> [chain]
assert_rule() {
    if nft list chain inet fw4 "${3:-familywarden_forward}" 2>/dev/null | grep -qE -- "$2"; then
        pass "$1"
    else
        fail "$1 (no match for: $2)"
    fi
}

refute_rule() {
    if nft list chain inet fw4 "${3:-familywarden_forward}" 2>/dev/null | grep -qE -- "$2"; then
        fail "$1 (unexpected match for: $2)"
    else
        pass "$1"
    fi
}

# call <method> [json] — prints the reply; fails unless it is one valid JSON object
call() {
    out=$(echo "${2:-{\}}" | "$BIN" call "$1")
    if echo "$out" | jsonfilter -e '@' >/dev/null 2>&1 && [ "$(echo "$out" | grep -c '^{')" -le 1 ]; then
        echo "$out"
    else
        fail "$1 reply is not a single JSON object: $out"
        echo "{}"
    fi
}

assert_json() {
    got=$(echo "$2" | jsonfilter -e "$3")
    [ "$got" = "$4" ] && pass "$1" || fail "$1 (expected $3=$4, got '$got')"
}

mkdir -p /var/lock /var/etc /var/run /usr/libexec/rpcd
cp /src/familywarden/usr/lib/rpcd/familywarden "$BIN"
chmod 755 "$BIN"

cat > /etc/config/familywarden <<'EOF'
config global 'settings'
    option enabled '1'
    option default_policy 'allow'

config profile 'kid'
    option name 'Kid'
    option enabled '1'
    option policy 'scheduled'
    option block_router '1'
    list device 'aa:bb:cc:00:00:01'
    list device 'aa:bb:cc:00:00:02'

config schedule 'kid_school'
    option profile 'kid'
    option days 'mon tue wed thu fri'
    option start '07:00'
    option stop '20:00'

config schedule 'kid_weekend'
    option profile 'kid'
    option days 'sat'
    option start '22:00'
    option stop '01:30'

config profile 'split'
    option name 'Split'
    option enabled '1'
    option policy 'scheduled'
    option block_router '1'
    list device 'aa:bb:cc:00:00:06'

config schedule 'split_morning'
    option profile 'split'
    option days 'mon'
    option start '07:00'
    option stop '09:00'

config schedule 'split_afternoon'
    option profile 'split'
    option days 'mon'
    option start '15:00'
    option stop '20:00'

config profile 'teen'
    option name 'Teen'
    option enabled '1'
    option policy 'block'
    option block_router '1'
    list device 'aa:bb:cc:00:00:03'

config profile 'parent'
    option name 'Parent'
    option enabled '1'
    option policy 'allow'
    list device 'aa:bb:cc:00:00:04'

config profile 'guest'
    option name 'Guest'
    option enabled '0'
    option policy 'block'
    list device 'aa:bb:cc:00:00:05'
EOF

# Same firewall include the package postinst adds
uci add firewall include >/dev/null
uci set firewall.@include[-1].fw4_compatible='1'
uci set firewall.@include[-1].type='nftables'
uci set firewall.@include[-1].path='/var/etc/familywarden.include'
uci commit firewall
fw4 start >/dev/null 2>&1

echo "=== reload: compile + validate + fw4 reload ==="
out=$("$BIN" reload)
[ "$out" = '{"status":"ok"}' ] && pass "reload ok" || { fail "reload failed: $out"; logread 2>/dev/null | tail -5; }

assert_rule "forward chain hooked" "hook forward priority filter \+ 1; policy accept"
assert_rule "ticket set accept first" "ether saddr @familywarden_ticket .* accept"
assert_rule "scheduled day window" 'aa:bb:cc:00:00:01 meta hour >= "07:00" meta hour < "20:00" meta day \{ "Monday", .*"Friday" \} .* accept'
assert_rule "overnight window, start day" 'aa:bb:cc:00:00:01 meta hour >= "22:00" meta day "Saturday" .* accept'
assert_rule "overnight window, next day" 'aa:bb:cc:00:00:01 meta hour < "01:30" meta day "Sunday" .* accept'
assert_rule "scheduled default drop (both devices)" 'aa:bb:cc:00:00:02 .* drop comment "familywarden:kid:scheduled"'
assert_rule "block policy drop" 'aa:bb:cc:00:00:03 .* drop comment "familywarden:teen:block"'
refute_rule "allow policy emits nothing" 'aa:bb:cc:00:00:04'
refute_rule "disabled profile emits nothing" 'aa:bb:cc:00:00:05'

assert_rule "input chain hooked" "hook input priority filter \+ 1; policy accept" familywarden_input
assert_rule "block_router: block policy drops TCP" 'aa:bb:cc:00:00:03 tcp dport 0-65535 .* drop' familywarden_input
assert_rule "block_router: scheduled window accepts TCP" 'aa:bb:cc:00:00:01 meta hour >= "07:00" meta hour < "20:00" .* tcp dport 0-65535 .* accept' familywarden_input
assert_rule "block_router: scheduled trailing drop" 'aa:bb:cc:00:00:01 tcp dport 0-65535 counter .* drop comment "familywarden:kid:scheduled:input"' familywarden_input
assert_rule "block_router: overnight window, next day" 'aa:bb:cc:00:00:01 meta hour < "01:30" meta day "Sunday" tcp dport 0-65535 .* accept' familywarden_input
# Regression: two windows on one day must both allow (inverted per-window drops
# used to cancel each other out and lock the router all day)
assert_rule "block_router: 1st window same day" 'aa:bb:cc:00:00:06 meta hour >= "07:00" meta hour < "09:00" meta day "Monday" tcp dport 0-65535 .* accept' familywarden_input
assert_rule "block_router: 2nd window same day" 'aa:bb:cc:00:00:06 meta hour >= "15:00" meta hour < "20:00" meta day "Monday" tcp dport 0-65535 .* accept' familywarden_input
refute_rule "block_router: no time-bounded drops" 'meta hour .* drop' familywarden_input
# Input chain mirrors forward: same profile has the same windows in both
fwd=$(nft list chain inet fw4 familywarden_forward | grep -c 'aa:bb:cc:00:00:06 meta hour')
inp=$(nft list chain inet fw4 familywarden_input | grep -c 'aa:bb:cc:00:00:06 meta hour')
[ "$fwd" = "$inp" ] && [ "$fwd" = "2" ] && pass "forward/input windows match ($fwd)" || fail "forward has $fwd windows, input has $inp"

echo ""
echo "=== reload is idempotent ==="
before=$(nft list ruleset | grep -c familywarden)
"$BIN" reload >/dev/null
after=$(nft list ruleset | grep -c familywarden)
[ "$before" = "$after" ] && pass "rule count stable ($after)" || fail "rule count changed $before → $after"

echo ""
echo "=== ubus: list / status ==="
out=$(call list)
assert_json "list has kid" "$out" '@.profiles.kid.name' "Kid"
out=$(call status '{"profile":"teen"}')
assert_json "teen status blocked" "$out" '@.status' "blocked"
out=$(call status '{"profile":"parent"}')
assert_json "parent status allowed" "$out" '@.status' "allowed"
out=$(call status '{"profile":"guest"}')
assert_json "disabled profile → error" "$out" '@.error' "profile not found or disabled"

echo ""
echo "=== ubus: block / allow / reset ==="
call block '{"profile":"parent"}' >/dev/null
assert_rule "block → override drop" 'aa:bb:cc:00:00:04 counter .* drop comment "familywarden:parent:override:block"'
assert_json "status follows override" "$(call status '{"profile":"parent"}')" '@.status' "blocked"

call allow '{"profile":"teen"}' >/dev/null
assert_rule "allow → override accept" 'aa:bb:cc:00:00:03 counter .* accept comment "familywarden:teen:override:allow"'
assert_rule "allow → router access restored" 'aa:bb:cc:00:00:03 tcp dport 0-65535 .* accept' familywarden_input

call reset '{"profile":"parent"}' >/dev/null
call reset '{"profile":"teen"}' >/dev/null
refute_rule "reset removes overrides" 'override'

echo ""
echo "=== ubus: ticket (kernel timeout set) ==="
out=$(call ticket '{"profile":"teen","minutes":5}')
assert_json "ticket reply ok" "$out" '@.status' "ok"
if nft list set inet fw4 familywarden_ticket | grep -qE 'aa:bb:cc:00:00:03 timeout 5m'; then
    pass "ticket element with 5m timeout"
else
    fail "ticket element missing"
fi
refute_rule "ticket is not a static rule" 'teen:override:allow'
assert_json "status allowed during ticket" "$(call status '{"profile":"teen"}')" '@.status' "allowed"

echo ""
echo "=== expired ticket is GC'd on next reload ==="
sed -i 's/^teen.override_expires=.*/teen.override_expires=1/' /tmp/familywarden.state
"$BIN" reload >/dev/null
if nft list set inet fw4 familywarden_ticket | grep -q 'aa:bb:cc:00:00:03'; then
    fail "expired ticket still in set"
else
    pass "expired ticket removed"
fi
assert_json "override cleared" "$(call status '{"profile":"teen"}')" '@.override' "none"

echo ""
echo "=== broken include is rejected, firewall keeps last good rules ==="
uci set familywarden.parent.policy='block'
uci add_list familywarden.parent.device='not-a-mac'
uci commit familywarden
out=$("$BIN" reload)
[ "$out" = '{"error":"include validation failed"}' ] && pass "invalid include rejected" || fail "invalid include not rejected: $out"
assert_rule "previous ruleset still loaded" 'familywarden:teen:block'

echo ""
if [ "$FAILED" -eq 0 ]; then
    echo "All integration checks passed!"
else
    echo "Some integration checks FAILED"
fi
exit "$FAILED"
