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
    # Phone identity lives in the Go core: a static linux/amd64 build runs in the rootfs.
    CORE_DIR=$(mktemp -d)
    trap 'rm -rf "$CORE_DIR"' EXIT
    if [ -n "$FW_CORE_BIN" ]; then cp "$FW_CORE_BIN" "$CORE_DIR/familywardend"
    else (cd "$REPO/core" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$CORE_DIR/familywardend" ./cmd/familywardend) || exit 1
    fi
    docker run --rm --cap-add NET_ADMIN -v "$REPO:/src:ro" -v "$CORE_DIR:/core:ro" "$IMAGE" \
        sh /src/scripts/test-integration.sh --inside
    exit $?
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
cp /core/familywardend /usr/sbin/familywardend
CORE_SOCK=/var/run/familywarden-core/core.sock

start_core() {
    mkdir -p /var/run/familywarden-core
    /usr/sbin/familywardend -data /etc/familywarden/core -socket "$CORE_SOCK" >>/tmp/core.log 2>&1 &
    CORE_PID=$!
    # OpenWrt's BusyBox sleep need not support fractional seconds.
    for _ in $(seq 1 20); do
        [ "$(/usr/sbin/familywardend -socket "$CORE_SOCK" status | jsonfilter -e '@.running')" = true ] && return 0
        sleep 1
    done
    fail "core did not start: $(tail -3 /tmp/core.log)"
}
stop_core() { kill "$CORE_PID" 2>/dev/null; wait "$CORE_PID" 2>/dev/null; }

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

start_core

# A LAN like a real router's (the rootfs ships no network config)
touch /etc/config/network
uci set network.lan=interface
uci set network.lan.device='br-lan'
uci set network.lan.proto='static'
uci commit network

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
assert_rule "block_router: block policy drops TCP" 'aa:bb:cc:00:00:03 tcp dport != 7700 .* drop' familywarden_input
assert_rule "block_router: scheduled window accepts TCP" 'aa:bb:cc:00:00:01 meta hour >= "07:00" meta hour < "20:00" .* tcp dport 0-65535 .* accept' familywarden_input
assert_rule "block_router: scheduled trailing drop" 'aa:bb:cc:00:00:01 tcp dport != 7700 counter .* drop comment "familywarden:kid:scheduled:input"' familywarden_input
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
if nft list set inet fw4 familywarden_ticket | grep -qE 'aa:bb:cc:00:00:03 timeout (5m|4m5[0-9]s)'; then
    pass "ticket element with 5m timeout"
else
    fail "ticket element missing"
fi
refute_rule "ticket is not a static rule" 'teen:override:allow'
assert_json "status allowed during ticket" "$(call status '{"profile":"teen"}')" '@.status' "allowed"

echo ""
echo "=== active tickets extend from their expiry, including concurrent additions ==="
expiry=$(call status '{"profile":"teen"}' | jsonfilter -e '@.override_expires')
call ticket '{"profile":"teen","minutes":5}' >/dev/null
assert_json "second ticket adds five minutes" "$(call status '{"profile":"teen"}')" '@.override_expires' "$((expiry + 300))"
expiry=$(call status '{"profile":"teen"}' | jsonfilter -e '@.override_expires')
echo '{"profile":"teen","minutes":1}' | "$BIN" call ticket >/tmp/ticket-one.json &
pid_one=$!
echo '{"profile":"teen","minutes":1}' | "$BIN" call ticket >/tmp/ticket-two.json &
pid_two=$!
wait "$pid_one"; wait "$pid_two"
assert_json "first concurrent ticket succeeds" "$(cat /tmp/ticket-one.json)" '@.status' "ok"
assert_json "second concurrent ticket succeeds" "$(cat /tmp/ticket-two.json)" '@.status' "ok"
assert_json "concurrent additions are not lost" "$(call status '{"profile":"teen"}')" '@.override_expires' "$((expiry + 120))"
for minutes in 0 -1 not-a-number; do
    out=$(call ticket "{\"profile\":\"teen\",\"minutes\":\"$minutes\"}")
    assert_json "invalid minutes $minutes refused" "$out" '@.error' "minutes must be a positive integer"
done
assert_json "invalid ticket leaves expiry unchanged" "$(call status '{"profile":"teen"}')" '@.override_expires' "$((expiry + 120))"

# Expired tickets, permanent allows and manual blocks all start a fresh timer.
for mode in expired allow block; do
    case "$mode" in
        expired) sed -i 's/^teen.override_expires=.*/teen.override_expires=1/' /tmp/familywarden.state ;;
        *) call "$mode" '{"profile":"teen"}' >/dev/null ;;
    esac
    before=$(date +%s)
    call ticket '{"profile":"teen","minutes":5}' >/dev/null
    after=$(date +%s)
    expiry=$(call status '{"profile":"teen"}' | jsonfilter -e '@.override_expires')
    if [ "$expiry" -ge "$((before + 300))" ] && [ "$expiry" -le "$((after + 300))" ]; then
        pass "ticket after $mode starts from now"
    else
        fail "ticket after $mode has wrong expiry: $expiry"
    fi
done

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

# Restore a valid config for the Ulysses section
uci -q del_list familywarden.parent.device='not-a-mac'
uci set familywarden.parent.policy='allow'
uci commit familywarden
"$BIN" reload >/dev/null

echo ""
echo "=== settings: default_policy ==="
uci set familywarden.settings.default_policy='block'
uci commit familywarden
"$BIN" reload >/dev/null
assert_rule "unknown devices on the LAN dropped" 'iifname "br-lan" ether saddr != \{ .* \} counter .* drop comment "familywarden:default:block"'
assert_rule "every profile's devices are known (incl. disabled)" 'ether saddr != \{ [^}]*aa:bb:cc:00:00:05[^}]* \} counter .* drop comment "familywarden:default:block"'
last=$(nft list chain inet fw4 familywarden_forward | grep 'comment' | tail -1)
echo "$last" | grep -q 'familywarden:default:block' && pass "default drop is the last rule" || fail "default drop not last: $last"
uci set familywarden.settings.default_policy='allow'
uci commit familywarden
"$BIN" reload >/dev/null
refute_rule "default_policy=allow: no default drop" 'familywarden:default:block'

echo ""
echo "=== settings: enabled ==="
uci set familywarden.settings.enabled='0'
uci commit familywarden
"$BIN" reload >/dev/null
refute_rule "disabled: no forward rules" 'familywarden:teen:block'
refute_rule "disabled: no input rules" 'familywarden:teen:block:input' familywarden_input
assert_rule "disabled: chains still there" 'hook forward priority filter \+ 1; policy accept'
assert_json "disabled: status allowed" "$(call status '{"profile":"teen"}')" '@.status' "allowed"
uci set familywarden.settings.enabled='1'
uci commit familywarden
"$BIN" reload >/dev/null
assert_rule "enabled again: rules back" 'familywarden:teen:block'

# ---- Ulysses ----
DAYS="sun mon tue wed thu fri sat"
day_name() { echo $DAYS | cut -d' ' -f$(( ($1 % 7) + 1 )); }
today=$(day_name "$(date +%w)")
tomorrow=$(day_name $(( $(date +%w) + 1 )))
cp /etc/config/familywarden /tmp/fw.before-ulysses

echo ""
echo "=== Ulysses: pact needs a captain ==="
assert_json "no captain → refused" "$(call pact '{"until":0}')" '@.error' "no Ulysses profile chosen in settings"
uci set familywarden.settings.ulysses_profile='kid'
uci set familywarden.settings.ulysses_delay='2'
uci set familywarden.settings.default_policy='block'
uci commit familywarden
"$BIN" reload >/dev/null

echo ""
echo "=== Ulysses: ongoing pact (outside its window) ==="
out=$(call pact "{\"until\":0,\"window_day\":\"$tomorrow\",\"window_start\":\"10:00\",\"window_stop\":\"11:00\"}")
assert_json "pact ok" "$out" '@.status' "ok"
out=$(call status '{"profile":"kid"}')
assert_json "kid is captain" "$out" '@.captain' "true"
assert_json "pact type ongoing" "$out" '@.pact_type' "ongoing"
assert_json "not in window" "$out" '@.pact_in_window' "false"
assert_json "parent is not captain" "$(call status '{"profile":"parent"}')" '@.captain' "false"
assert_json "allow refused for captain" "$(call allow '{"profile":"kid"}')" '@.error' "bound by Ulysses pact"
assert_json "ticket refused for captain" "$(call ticket '{"profile":"kid","minutes":30}')" '@.error' "bound by Ulysses pact"
call block '{"profile":"kid"}' >/dev/null
assert_rule "block still works (tightening)" 'aa:bb:cc:00:00:01 counter .* drop comment "familywarden:kid:override:block"'
call reset '{"profile":"kid"}' >/dev/null
refute_rule "reset back to the pact" 'kid:override'
call allow '{"profile":"parent"}' >/dev/null
assert_rule "other profiles unaffected" 'aa:bb:cc:00:00:04 counter .* accept comment "familywarden:parent:override:allow"'
call reset '{"profile":"parent"}' >/dev/null
assert_json "can't change the pact outside its window" \
    "$(call pact "{\"until\":0,\"window_day\":\"$today\",\"window_start\":\"00:00\",\"window_stop\":\"23:59\"}")" \
    '@.error' "the pact can only be extended"

echo ""
echo "=== Ulysses: captain frozen ==="
uci set familywarden.kid_school.stop='23:59'
uci del_list familywarden.kid.device='aa:bb:cc:00:00:02'
uci set familywarden.kid.policy='allow'
uci set familywarden.settings.ulysses_profile='parent'
uci set familywarden.settings.ulysses_delay='0'
uci commit familywarden
"$BIN" reload >/dev/null
assert_rule "schedule edit ignored" 'aa:bb:cc:00:00:01 meta hour >= "07:00" meta hour < "20:00"'
refute_rule "new stop time not applied" '"23:59"'
assert_rule "removed device still enforced" 'aa:bb:cc:00:00:02 .* drop comment "familywarden:kid:scheduled"'
assert_json "captain can't be swapped" "$(call status '{"profile":"kid"}')" '@.captain' "true"
uci set familywarden.settings.enabled='0'
uci set familywarden.settings.default_policy='allow'
uci commit familywarden
"$BIN" reload >/dev/null
assert_rule "can't switch FamilyWarden off during a pact" 'familywarden:kid:scheduled'
assert_rule "can't let unknown devices through during a pact" 'familywarden:default:block'
uci set familywarden.settings.enabled='1'
uci commit familywarden

echo ""
echo "=== Ulysses: captain's MACs can't escape via other profiles ==="
uci add_list familywarden.parent.device='aa:bb:cc:00:00:01'
uci commit familywarden
"$BIN" reload >/dev/null
call allow '{"profile":"parent"}' >/dev/null
refute_rule "captain MAC not accepted by another profile" 'aa:bb:cc:00:00:01 counter .* accept'
assert_rule "the other profile's own device still is" 'aa:bb:cc:00:00:04 counter .* accept'
call reset '{"profile":"parent"}' >/dev/null

echo ""
echo "=== Ulysses: router lockdown while the captain is blocked ==="
assert_rule "lockdown chain hooked" "hook input priority filter \+ 2; policy accept" familywarden_lockdown
assert_rule "loopback open" 'iifname "lo" accept' familywarden_lockdown
assert_rule "established open" 'ct state established,related accept' familywarden_lockdown
assert_rule "DNS + sail page open" 'tcp dport \{ 53, 7700 \} accept' familywarden_lockdown
assert_rule "amendment window open" 'meta day .* meta hour >= "10:00" meta hour < "11:00" accept comment "familywarden:ulysses:window"' familywarden_lockdown
assert_rule "captain's allowed hours open (any device)" '^[[:space:]]*meta hour >= "07:00" meta hour < "20:00" meta day .* accept' familywarden_lockdown
assert_rule "everything else dropped" 'meta l4proto tcp counter .* drop comment "familywarden:ulysses:lockdown"' familywarden_lockdown
assert_rule "block_router leaves the sail page open" 'aa:bb:cc:00:00:01 tcp dport != 7700 counter .* drop' familywarden_input

echo ""
echo "=== Ulysses: survives reboot ==="
rm -rf /var/run/familywarden /tmp/familywarden.state
"$BIN" reload >/dev/null
assert_json "still captain after reboot" "$(call status '{"profile":"kid"}')" '@.captain' "true"
refute_rule "still frozen after reboot" '"23:59"'

echo ""
echo "=== Ulysses: amendment window ==="
sed -i "s/^window_day=.*/window_day=$today/; s/^window_start=.*/window_start=00:00/; s/^window_stop=.*/window_stop=23:59/" /etc/familywarden/pact
"$BIN" reload >/dev/null
assert_json "in window" "$(call status '{"profile":"kid"}')" '@.pact_in_window' "true"
refute_rule "edits apply at once (policy=allow → no kid rules)" 'familywarden:kid:scheduled'
uci set familywarden.kid.policy='scheduled'
uci commit familywarden
"$BIN" reload >/dev/null
assert_rule "edited stop time applies in window" 'aa:bb:cc:00:00:01 meta hour >= "07:00" meta hour < "23:59"'
out=$(call pact "{\"until\":0,\"window_day\":\"$tomorrow\",\"window_start\":\"10:00\",\"window_stop\":\"11:00\"}")
assert_json "pact can be amended in window" "$out" '@.status' "ok"
assert_json "window closed again" "$(call status '{"profile":"kid"}')" '@.pact_in_window' "false"

echo ""
echo "=== Ulysses: sail page (port 7700 CGI) ==="
mkdir -p /var/run/ubus
ubusd >/dev/null 2>&1 &
sleep 1
rpcd >/dev/null 2>&1 &
sleep 1
printf 'sirens\nsirens\n' | passwd root >/dev/null 2>&1
SAIL=/src/familywarden/www-familywarden/cgi-bin/sail
sail() {
    if [ -z "$1" ]; then
        REQUEST_METHOD=GET sh "$SAIL" | sed '1,/^$/d'
    else
        printf '%s' "$1" | REQUEST_METHOD=POST CONTENT_LENGTH=${#1} sh "$SAIL" | sed '1,/^$/d'
    fi
}
out=$(sail)
assert_json "status: pact" "$out" '@.pact' "true"
assert_json "status: captain" "$out" '@.captain' "kid"
assert_json "status: cooling-off frozen at 2 h" "$out" '@.delay' "7200"
assert_json "wrong password refused" "$(sail '{"action":"release","password":"nope","answer":"press on"}')" '@.error' "wrong password"
assert_json "wrong line refused" "$(sail '{"action":"release","password":"sirens","answer":"let me go"}')" '@.error' "The sirens sing on. Still tied to the mast."
out=$(sail '{"action":"release","password":"sirens","answer":"  Press on! "}')
rel=$(echo "$out" | jsonfilter -e '@.release_at')
now=$(date +%s)
[ "$rel" -ge $((now + 7190)) ] && [ "$rel" -le $((now + 7200)) ] && pass "release in 2 h" || fail "release_at=$rel now=$now"
"$BIN" tick
assert_json "still bound before the cooling-off" "$(call status '{"profile":"kid"}')" '@.captain' "true"
assert_json "cancel needs the password" "$(sail '{"action":"cancel","password":"nope"}')" '@.error' "wrong password"
assert_json "cancel" "$(sail '{"action":"cancel","password":"sirens"}')" '@.release_at' "0"
sail '{"action":"release","password":"sirens","answer":"press on"}' >/dev/null
echo $(( $(date +%s) - 1 )) > /etc/familywarden/release
"$BIN" tick
assert_json "released after the cooling-off" "$(call status '{"profile":"kid"}')" '@.captain' "false"
refute_rule "no lockdown without a pact" 'drop' familywarden_lockdown
[ -f /etc/familywarden/pact ] && fail "pact file left" || pass "pact removed"

echo ""
echo "=== Ulysses: pact until a time ==="
cp /tmp/fw.before-ulysses /etc/config/familywarden
uci set familywarden.settings.ulysses_profile='kid'
uci commit familywarden
"$BIN" reload >/dev/null
until=$(( $(date +%s) + 3600 ))
assert_json "until pact ok" "$(call pact "{\"until\":$until}")" '@.status' "ok"
assert_rule "lockdown ends on time in the kernel" 'meta l4proto tcp meta time < "[0-9-]+ [0-9:]+" counter .* drop' familywarden_lockdown
assert_json "can be extended" "$(call pact "{\"until\":$((until + 600))}")" '@.status' "ok"
assert_json "can't be shortened" "$(call pact "{\"until\":$((until - 600))}")" '@.error' "the pact can only be extended"
assert_json "can't become ongoing" "$(call pact '{"until":0}')" '@.error' "the pact can only be extended"
sed -i "s/^until=.*/until=$(( $(date +%s) - 1 ))/" /etc/familywarden/pact
"$BIN" tick
[ -f /etc/familywarden/pact ] && fail "expired pact left" || pass "expired pact cleaned up"
assert_json "free after the pact" "$(call status '{"profile":"kid"}')" '@.captain' "false"

echo ""
echo "=== Ulysses Mast: signed sync for the phone ==="
MAST=/src/familywarden/www-familywarden/cgi-bin/mast
call pact "{\"until\":0,\"window_day\":\"$tomorrow\",\"window_start\":\"10:00\",\"window_stop\":\"11:00\"}" >/dev/null
uci set familywarden.kid_school.stop='23:59'
uci commit familywarden
"$BIN" reload >/dev/null
REQUEST_METHOD=GET QUERY_STRING="device=pixel-10" sh "$MAST" | sed '1,/^$/d' > /tmp/mast.out
sig=$(sed -n 1p /tmp/mast.out)
sed 1d /tmp/mast.out | tr -d '\n' > /tmp/mast.payload
printf 'untrusted comment: test\n%s\n' "$sig" > /tmp/mast.sig
printf 'untrusted comment: test\n%s\n' "$(call mast | jsonfilter -e '@.pubkey')" > /tmp/mast.pub
usign -V -q -m /tmp/mast.payload -p /tmp/mast.pub -x /tmp/mast.sig && pass "payload signature verifies" || fail "signature does not verify"
payload=$(cat /tmp/mast.payload)
assert_json "payload: pact active" "$payload" '@.pact.active' "true"
assert_json "payload: captain" "$payload" '@.captain' "kid"
assert_json "payload: rules from the frozen snapshot" "$payload" '@.rules.windows[0].stop' "20:00"
assert_json "payload: policy" "$payload" '@.rules.policy' "scheduled"
st=$(echo "$payload" | jsonfilter -e '@.server_time')
[ "$st" -ge $(( $(date +%s) - 5 )) ] && pass "payload: server_time is now" || fail "server_time=$st"
sed 's/"captain": "kid"/"captain": "xyz"/' /tmp/mast.payload > /tmp/mast.forged
usign -V -q -m /tmp/mast.forged -p /tmp/mast.pub -x /tmp/mast.sig && fail "forged payload verified" || pass "forged payload rejected"
out=$(call mast)
assert_json "LuCI: last device" "$out" '@.device' "pixel-10"
seen=$(echo "$out" | jsonfilter -e '@.last_sync')
[ "$seen" -ge $(( $(date +%s) - 5 )) ] && pass "LuCI: last sync recorded" || fail "last_sync=$seen"
[ -n "$(echo "$out" | jsonfilter -e '@.fingerprint')" ] && pass "LuCI: key fingerprint" || fail "no fingerprint"
REQUEST_METHOD=GET QUERY_STRING='device=../../etc/x' sh "$MAST" >/dev/null
assert_json "bad device id not recorded" "$(call mast)" '@.device' "pixel-10"
assert_json "captain status shows the phone's last sync" "$(call status '{"profile":"kid"}')" '@.mast_last_sync' "$seen"

apk update >/dev/null 2>&1; apk add qrencode >/dev/null 2>&1
out=$(call mast '{"host":"192.168.1.1"}')
uri=$(echo "$out" | jsonfilter -e '@.pair_uri')
echo "$uri" | grep -q '^ulyssesmast://pair?host=192.168.1.1&key=RW' && pass "pairing URI with host and key" || fail "pair_uri=$uri"
echo "${uri#*key=}" | grep -q '[+/=]' && fail "key not URL-encoded: $uri" || pass "key URL-encoded"
echo "$out" | jsonfilter -e '@.qr_svg' | grep -q '<svg' && pass "pairing QR (SVG)" || fail "no qr_svg (qrencode installed?)"
[ -z "$(call mast '{"host":"evil\"; rm -rf /"}' | jsonfilter -e '@.pair_uri')" ] && pass "bad host rejected" || fail "bad host accepted"

# A strict external resolver may not know router.lan. Keep a numeric control path
# in the QR without changing the legacy host/key fields or requiring a DNS lookup.
uci set network.lan.ipaddr='192.168.50.1/24'
uci commit network
numeric=$(call mast '{"host":"router.invalid"}')
assert_json "hostname pairing includes numeric LAN endpoint" "$numeric" '@.router_address' '192.168.50.1'
echo "$numeric" | jsonfilter -e '@.pair_uri' | grep -q '&address=192.168.50.1' \
    && pass "QR carries DNS-independent control address" || fail "QR has no numeric address"
assert_json "numeric endpoint retains router fingerprint" "$numeric" '@.fingerprint' "$(echo "$out" | jsonfilter -e '@.fingerprint')"
# Exercise the live-interface JSON branch as well as the configured-address fallback.
mkdir -p /tmp/fw-test-ubus
cat > /tmp/fw-test-ubus/ubus <<'EOF'
#!/bin/sh
[ "$1 $2 $3" = 'call network.interface.lan status' ] || exit 1
printf '%s\n' '{"ipv4-address":[{"address":"192.0.2.9","mask":24}]}'
EOF
chmod 755 /tmp/fw-test-ubus/ubus
live=$(PATH="/tmp/fw-test-ubus:$PATH" call mast '{"host":"router.invalid"}')
assert_json "live LAN address takes precedence over UCI fallback" "$live" '@.router_address' '192.0.2.9'
rm -rf /tmp/fw-test-ubus
uci set network.lan.ipaddr='bad.address'
uci commit network
[ -z "$(call mast '{"host":"router.invalid"}' | jsonfilter -e '@.router_address')" ] \
    && pass "invalid configured numeric address not advertised" || fail "bad address accepted"
uci -q delete network.lan.ipaddr
uci commit network
rm -rf /var/run/familywarden
"$BIN" reload >/dev/null
assert_json "same key after reboot" "$(call mast)" '@.fingerprint' "$(echo "$out" | jsonfilter -e '@.fingerprint')"
echo $(( $(date +%s) - 1 )) > /etc/familywarden/release
"$BIN" tick
payload=$(REQUEST_METHOD=GET sh "$MAST" | sed '1,/^$/d' | sed 1d)
assert_json "payload after release: no pact" "$payload" '@.pact.active' "false"
assert_json "payload after release: captain still known" "$payload" '@.captain' "kid"

echo ""
echo "=== FamilyWarden app v2: enrollment, binding and pact freeze ==="
N1=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
N2=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
enc() { printf '%s' "$1" | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g'; }
fpr() { sed -n 2p "/tmp/$1.pub" | tr -d '\n' | sha256sum | cut -d' ' -f1; }
newphone() { usign -G -s "/tmp/$1.key" -p "/tmp/$1.pub" -c phone >/dev/null 2>&1; }
# Raw CGI reply of a v2 request: $1 phone key, $2 device id, $3 nonce [, $4 nonce actually signed]
mast2() {
    printf 'familywarden-sync-v2\n%s\n%s\n' "$2" "${4:-$3}" > /tmp/m2.msg
    usign -S -m /tmp/m2.msg -s "/tmp/$1.key" -x /tmp/m2.sig
    REQUEST_METHOD=GET QUERY_STRING="device=$2&nonce=$3&key=$(enc "$(sed -n 2p "/tmp/$1.pub")")&sig=$(enc "$(sed -n 2p /tmp/m2.sig)")&name=Pixel-9" sh "$MAST"
}
v2payload() { mast2 "$@" | sed '1,/^$/d' | sed 1d; }

newphone phoneA
mast2 phoneA fw-a1 "$N1" > /tmp/m2.out
head -1 /tmp/m2.out | grep -q '^Status: 200' && pass "v2 request accepted" || fail "v2 status: $(head -1 /tmp/m2.out)"
sed '1,/^$/d' /tmp/m2.out > /tmp/m2.body
printf 'untrusted comment: test\n%s\n' "$(sed -n 1p /tmp/m2.body)" > /tmp/m2.rsig
sed 1d /tmp/m2.body > /tmp/m2.payload
usign -V -q -m /tmp/m2.payload -p /tmp/mast.pub -x /tmp/m2.rsig && pass "v2 reply signed by the router" || fail "v2 reply signature"
payload=$(cat /tmp/m2.payload)
assert_json "unknown phone is pending" "$payload" '@.enrollment' "pending"
assert_json "reply echoes the nonce" "$payload" '@.nonce' "$N1"
[ -z "$(echo "$payload" | jsonfilter -e '@.binding')" ] && pass "no binding before approval" || fail "pending phone got a binding"
[ -z "$(echo "$payload" | jsonfilter -e '@.pact')" ] && pass "no pact state before approval" || fail "pending phone got pact state"
out=$(call devices)
assert_json "LuCI lists the pending phone" "$out" '@.pending[0].id' "fw-a1"
assert_json "pending fingerprint matches the phone key" "$out" '@.pending[0].fingerprint' "$(sed -n 2p /tmp/phoneA.pub | tr -d '\n' | sha256sum | cut -d' ' -f1)"
assert_json "pending phone model" "$out" '@.pending[0].name' "Pixel-9"
mast2 phoneA fw-a1 "$N1" "$N2" | head -1 | grep -q '^Status: 403' && pass "signature over another nonce refused" || fail "wrong signature accepted"
REQUEST_METHOD=GET QUERY_STRING="device=fw-a1&nonce=xyz&key=a&sig=b" sh "$MAST" | head -1 | grep -q '^Status: 400' \
    && pass "malformed v2 request refused" || fail "malformed request not refused"
assert_json "approval needs a pending phone" "$(call device_approve '{"device":"fw-zz","profile":"kid","administrator":"0"}')" '@.error' "no pending device"
assert_json "approval needs an existing profile" "$(call device_approve "{\"device\":\"fw-a1\",\"profile\":\"nope\",\"administrator\":\"0\",\"fingerprint\":\"$(fpr phoneA)\"}")" '@.error' "profile not found"
assert_json "approval refuses an unseen fingerprint" "$(call device_approve '{"device":"fw-a1","profile":"kid","administrator":"1","fingerprint":"0000"}')" '@.error' "pending device changed; refresh and compare fingerprint"
assert_json "a name can't inject UCI commands" "$(call device_approve "{\"device\":\"fw-a1\",\"profile\":\"kid\",\"administrator\":\"0\",\"fingerprint\":\"$(fpr phoneA)\",\"name\":\"x' \\n set familywarden.settings.enabled=0 #\"}")" '@.status' "ok"
[ "$(uci -q get familywarden.settings.enabled)" = "1" ] && pass "settings untouched by a crafted name" || fail "UCI injection"
call device_delete '{"device":"fw-a1"}' >/dev/null
v2payload phoneA fw-a1 "$N1" >/dev/null
assert_json "approve the captain's phone" "$(call device_approve "{\"device\":\"fw-a1\",\"profile\":\"kid\",\"name\":\"Alex phone\",\"administrator\":\"1\",\"fingerprint\":\"$(fpr phoneA)\"}")" '@.status' "ok"
payload=$(v2payload phoneA fw-a1 "$N2")
assert_json "approved" "$payload" '@.enrollment' "approved"
assert_json "binding: device" "$payload" '@.binding.device_id' "fw-a1"
assert_json "binding: profile" "$payload" '@.binding.profile_id' "kid"
assert_json "binding: captain" "$payload" '@.binding.captain' "true"
assert_json "binding: administrator" "$payload" '@.binding.administrator' "true"
assert_json "administrator may remove its own app" "$payload" '@.binding.can_self_remove' "true"
assert_json "fresh nonce echoed" "$payload" '@.nonce' "$N2"
assert_json "captain rules for the captain's phone" "$payload" '@.rules.policy' "scheduled"
[ -z "$(call devices | jsonfilter -e '@.pending[*].id')" ] && pass "approved phone leaves the pending list" || fail "still pending"
assert_json "LuCI lists the approved phone" "$(call devices)" '@.devices[0].fingerprint' "$(sed -n 2p /tmp/phoneA.pub | tr -d '\n' | sha256sum | cut -d' ' -f1)"

newphone phoneB
payload=$(v2payload phoneB fw-a1 "$N1")
assert_json "same id with another key gets nothing" "$payload" '@.enrollment' "pending"
[ -z "$(echo "$payload" | jsonfilter -e '@.binding')" ] && pass "no binding for a foreign key" || fail "foreign key got a binding"

v2payload phoneB fw-a1 "$N1" >/dev/null
assert_json "rejecting a re-keyed request" "$(call device_reject '{"device":"fw-a1"}')" '@.status' "ok"
assert_json "...keeps the approved phone" "$(v2payload phoneA fw-a1 "$N2")" '@.enrollment' "approved"

newphone phoneC
v2payload phoneC fw-c1 "$N1" >/dev/null
call device_approve "{\"device\":\"fw-c1\",\"profile\":\"parent\",\"name\":\"Taylor phone\",\"administrator\":\"0\",\"fingerprint\":\"$(fpr phoneC)\"}" >/dev/null
payload=$(v2payload phoneC fw-c1 "$N1")
assert_json "managed phone: not captain" "$payload" '@.binding.captain' "false"
assert_json "managed phone: no captain rules" "$payload" '@.captain' ""
assert_json "managed phone: may not remove itself" "$payload" '@.binding.can_self_remove' "false"
assert_json "administrator allows removal" "$(call device_allow_removal '{"device":"fw-c1"}')" '@.status' "ok"
assert_json "removal allowed for an hour" "$(v2payload phoneC fw-c1 "$N1")" '@.binding.can_self_remove' "true"
uci set familywarden.kid.dns='1'
uci add_list familywarden.kid.dns_protection='safe_search'
uci commit familywarden

call pact "{\"until\":0,\"window_day\":\"$tomorrow\",\"window_start\":\"10:00\",\"window_stop\":\"11:00\"}" >/dev/null
grep -qx 'dns=1' /etc/familywarden/pact.dns && grep -qx 'prot=safe_search' /etc/familywarden/pact.dns \
    && pass "captain's DNS snapshotted with the pact" || fail "pact.dns: $(cat /etc/familywarden/pact.dns)"
payload=$(v2payload phoneA fw-a1 "$N1")
assert_json "pact applies to the captain's phone" "$payload" '@.pact.active' "true"
assert_json "managed phone is not bound by the captain's pact" "$(v2payload phoneC fw-c1 "$N1")" '@.pact.active' "false"
assert_json "captain's phone can't be moved during the pact" "$(call device_update '{"device":"fw-a1","profile":"parent"}')" '@.error' "bound by Ulysses pact"
assert_json "captain's phone can't be deleted during the pact" "$(call device_delete '{"device":"fw-a1"}')" '@.error' "bound by Ulysses pact"
assert_json "captain's phone can't be allowed removal during the pact" "$(call device_allow_removal '{"device":"fw-a1"}')" '@.error' "bound by Ulysses pact"
assert_json "LuCI shows it frozen" "$(call devices)" '@.devices[@.id="fw-a1"].frozen' "true"
payload=$(v2payload phoneA fw-a1 "$N1")
assert_json "the refused delete left the captain's phone bound" "$payload" '@.binding.captain' "true"
assert_json "...and the pact still applies" "$payload" '@.pact.active' "true"
newphone phoneD
v2payload phoneD fw-d1 "$N1" >/dev/null
assert_json "a phone can join the captain during the pact" "$(call device_approve "{\"device\":\"fw-d1\",\"profile\":\"kid\",\"administrator\":\"0\",\"fingerprint\":\"$(fpr phoneD)\"}")" '@.status' "ok"
assert_json "...and is bound at once" "$(v2payload phoneD fw-d1 "$N1")" '@.pact.active' "true"

[ -z "$(v2payload phoneA fw-a1 "$N1" | jsonfilter -e '@.dns')" ] && pass "no DNS without an acknowledged AdGuard sync" || fail "DNS without ack"
echo "kid alex dns.example.net filter_lists,safe_search youtube" > /etc/familywarden/dns.ack
payload=$(v2payload phoneA fw-a1 "$N1")
assert_json "DNS hostname from the acknowledged sync" "$payload" '@.dns.hostname' "alex.dns.example.net"
assert_json "DNS protections" "$payload" '@.dns.protections[1]' "safe_search"
assert_json "DNS services" "$payload" '@.dns.services[0]' "youtube"
[ -z "$(v2payload phoneC fw-c1 "$N1" | jsonfilter -e '@.dns')" ] && pass "other profiles get no DNS" || fail "DNS leaked to another profile"
rm -f /etc/familywarden/dns.ack

for i in 1 2 3 4 5 6 7 8 9 10 11 12; do v2payload phoneB "fw-p$i" "$N1" >/dev/null; done
[ "$(call devices | jsonfilter -e '@.pending[*].id' | wc -l)" -le 10 ] && pass "pending list bounded" || fail "pending list unbounded"
echo $(( $(date +%s) - 1 )) > /etc/familywarden/release
"$BIN" tick
[ -f /etc/familywarden/pact.dns ] && fail "DNS snapshot left after the pact" || pass "DNS snapshot removed with the pact"
[ -f /tmp/familywarden.dns.dirty ] && pass "DNS resynced when the pact ends" || fail "no DNS resync after the pact"
assert_json "after the pact the captain's phone can be removed" "$(call device_delete '{"device":"fw-a1"}')" '@.status' "ok"
assert_json "...and is pending again" "$(v2payload phoneA fw-a1 "$N1")" '@.enrollment' "pending"
assert_json "the phone that joined is free again" "$(v2payload phoneD fw-d1 "$N1")" '@.pact.active' "false"

echo ""
echo "=== App control: wire v3 (apps policy, acknowledgement, pact freeze) ==="
# Both signatures, as a v3 phone sends them: $1 phone key, $2 device id, $3 nonce,
# $4 applied revision ("-" = none), $5 signed revision, $6 caps ("legacy" omits them),
# $7 caps actually signed, $8 capability-signature nonce (tampering tests).
mast3() {
    local caps="${6-apps-v1,screen-time-v1}" extra=""
    printf 'familywarden-sync-v2\n%s\n%s\n' "$2" "$3" > /tmp/m3.msg2
    printf 'familywarden-sync-v3\n%s\n%s\n%s\n' "$2" "$3" "${5:-$4}" > /tmp/m3.msg3
    usign -S -m /tmp/m3.msg2 -s "/tmp/$1.key" -x /tmp/m3.sig2
    usign -S -m /tmp/m3.msg3 -s "/tmp/$1.key" -x /tmp/m3.sig3
    if [ "$caps" != legacy ]; then
        printf 'familywarden-sync-caps-v1\n%s\n%s\n%s\n%s\n' "$2" "${8:-$3}" "$4" "${7-$caps}" > /tmp/m3.capsmsg
        usign -S -m /tmp/m3.capsmsg -s "/tmp/$1.key" -x /tmp/m3.capssig
        extra="&caps=$caps&sigcaps=$(enc "$(sed -n 2p /tmp/m3.capssig)")"
    fi
    REQUEST_METHOD=GET QUERY_STRING="device=$2&nonce=$3&key=$(enc "$(sed -n 2p "/tmp/$1.pub")")&sig=$(enc "$(sed -n 2p /tmp/m3.sig2)")&sig3=$(enc "$(sed -n 2p /tmp/m3.sig3)")&applied=$4&name=Pixel-9$extra" sh "$MAST"
}
v3payload() { mast3 "$@" | sed '1,/^$/d' | sed 1d; }
count() { echo "$1" | jsonfilter -e "$2" 2>/dev/null | grep -c .; }

payload=$(v3payload phoneD fw-d1 "$N1" -)
assert_json "v3 request gets a v3 reply" "$payload" '@.v' "3"
assert_json "v3 keeps the v2 binding" "$payload" '@.binding.profile_id' "kid"
[ -z "$(echo "$payload" | jsonfilter -e '@.apps')" ] && pass "no apps object while app control is off" || fail "apps without app control"
assert_json "an old (v2-only) phone still gets v2" "$(v2payload phoneD fw-d1 "$N1")" '@.v' "2"

uci set familywarden.kid.apps='1'
for s in "kid_roblox app com.roblox.client 60 0" "kid_roblox2 app com.roblox.client 30 0" \
         "kid_tiktok app com.zhiliaoapp.musically - 1" "kid_spotify app com.spotify.music - 0" \
         "kid_bad app ../etc - 0" "parent_other app com.example.other 5 0"; do
    set -- $s
    uci set "familywarden.$1=$2"
    uci set "familywarden.$1.profile=${1%%_*}"
    uci set "familywarden.$1.package=$3"
    [ "$4" = "-" ] || uci set "familywarden.$1.minutes=$4"
    uci set "familywarden.$1.blocked=$5"
done
uci set familywarden.parent.apps='1'
uci set familywarden.kid_bed=app_downtime
uci set familywarden.kid_bed.profile='kid'
uci set familywarden.kid_bed.days='mon tue wed thu'
uci set familywarden.kid_bed.start='21:00'
uci set familywarden.kid_bed.stop='07:00'
uci set familywarden.kid_badbed=app_downtime
uci set familywarden.kid_badbed.profile='kid'
uci set familywarden.kid_badbed.days='someday'
uci set familywarden.kid_badbed.start='21:00'
uci set familywarden.kid_badbed.stop='07:00'
uci commit familywarden
"$BIN" reload >/dev/null

mast3 phoneD fw-d1 "$N1" - > /tmp/m3.out
sed '1,/^$/d' /tmp/m3.out > /tmp/m3.body
printf 'untrusted comment: test\n%s\n' "$(sed -n 1p /tmp/m3.body)" > /tmp/m3.rsig
sed 1d /tmp/m3.body > /tmp/m3.payload
usign -V -q -m /tmp/m3.payload -p /tmp/mast.pub -x /tmp/m3.rsig && pass "v3 reply signed by the router" || fail "v3 reply signature"
payload=$(cat /tmp/m3.payload)
rev=$(echo "$payload" | jsonfilter -e '@.apps.rev')
echo "$rev" | grep -qE '^[0-9a-f]{16}$' && pass "apps revision ($rev)" || fail "bad apps revision: $rev"
assert_json "new apps are suspended by default (A12)" "$payload" '@.apps.suspend_new' "true"
assert_json "duplicate rules merge to the smallest limit" "$payload" '@.apps.limits[@.package="com.roblox.client"].minutes' "30"
[ "$(count "$payload" '@.apps.limits[*]')" = 1 ] && pass "one limit (blocked/unlimited apps have none)" || fail "limits: $(echo "$payload" | jsonfilter -e '@.apps.limits')"
assert_json "blocked app" "$payload" '@.apps.blocked[0]' "com.zhiliaoapp.musically"
[ "$(count "$payload" '@.apps.approved[*]')" = 2 ] && echo "$payload" | jsonfilter -e '@.apps.approved[*]' | grep -qx com.spotify.music \
    && ! echo "$payload" | jsonfilter -e '@.apps.approved[*]' | grep -qx com.zhiliaoapp.musically \
    && pass "approved = configured and not blocked" || fail "approved: $(echo "$payload" | jsonfilter -e '@.apps.approved')"
echo "$payload" | grep -q 'etc"' && fail "invalid package name emitted" || pass "invalid package name ignored"
echo "$payload" | grep -q 'com.example.other' && fail "another profile's rule leaked" || pass "only this profile's rules"
assert_json "downtime window" "$payload" '@.apps.downtime[0].days' "mon tue wed thu"
[ "$(count "$payload" '@.apps.downtime[*]')" = 1 ] && pass "invalid downtime ignored" || fail "downtime: $(echo "$payload" | jsonfilter -e '@.apps.downtime')"
assert_json "revision is stable" "$(v3payload phoneD fw-d1 "$N2" -)" '@.apps.rev' "$rev"

assert_json "devices: expected revision" "$(call devices)" '@.devices[@.id="fw-d1"].apps_rev' "$rev"
assert_json "devices: nothing applied yet" "$(call devices)" '@.devices[@.id="fw-d1"].apps_applied' ""
v3payload phoneD fw-d1 "$N1" "$rev" >/dev/null
assert_json "devices: signed acknowledgement recorded" "$(call devices)" '@.devices[@.id="fw-d1"].apps_applied' "$rev"
mast3 phoneD fw-d1 "$N1" "$rev" "0000000000000000" | head -1 | grep -q '^Status: 403' \
    && pass "acknowledgement must be signed" || fail "forged acknowledgement accepted"
mast3 phoneD fw-d1 "$N1" "not-a-rev" | head -1 | grep -q '^Status: 400' \
    && pass "malformed acknowledgement refused" || fail "malformed acknowledgement accepted"
assert_json "forged acknowledgement not recorded" "$(call devices)" '@.devices[@.id="fw-d1"].apps_applied' "$rev"

echo "=== Authenticated capabilities and fail-closed policy requirements ==="
assert_json "app policy requires app-control support" "$payload" '@.requires[0]' "apps-v1"
assert_json "whole-phone downtime requires P7b support" "$payload" '@.requires[1]' "screen-time-v1"
assert_json "device reports authenticated feature support" "$(call devices)" '@.devices[@.id="fw-d1"].capabilities[1]' "screen-time-v1"
mast3 phoneD fw-d1 "$N1" "$rev" "" legacy > /tmp/caps-refused.out
sed '1,/^$/d' /tmp/caps-refused.out > /tmp/caps-refused.body
printf 'untrusted comment: test\n%s\n' "$(head -1 /tmp/caps-refused.body)" > /tmp/caps-refused.sig
sed 1d /tmp/caps-refused.body > /tmp/caps-refused.json
usign -V -q -m /tmp/caps-refused.json -p /tmp/mast.pub -x /tmp/caps-refused.sig \
    && pass "capability refusal is signed by the router" || fail "unsigned refusal"
refused=$(cat /tmp/caps-refused.json)
assert_json "old v3 client gets a refusal, not weaker rules" "$refused" '@.error' "unsupported_capabilities"
assert_json "refusal is request-bound" "$refused" '@.nonce' "$N1"
[ -z "$(echo "$refused" | jsonfilter -e '@.binding')" ] && pass "refusal grants no binding" || fail "refusal has binding"
[ -z "$(echo "$refused" | jsonfilter -e '@.apps')" ] && pass "refusal grants no partial app policy" || fail "refusal has apps"
assert_json "incompatible phone cannot acknowledge current rules" "$(call devices)" '@.devices[@.id="fw-d1"].apps_applied' ""
assert_json "LuCI can distinguish upgrade-needed from waiting" "$(call devices)" '@.devices[@.id="fw-d1"].apps_error' "unsupported_capabilities"
assert_json "signed empty capability set is insufficient" "$(v3payload phoneD fw-d1 "$N1" - "" -)" '@.error' "unsupported_capabilities"
assert_json "unknown capability is not authorization" "$(v3payload phoneD fw-d1 "$N1" - "" future-parent)" '@.error' "unsupported_capabilities"
for caps in 'screen-time-v1,screen-time-v1' 'screen-time-v1,apps-v1' 'SCREEN-TIME-V1' ''; do
    mast3 phoneD fw-d1 "$N1" - "" "$caps" | head -1 | grep -q '^Status: 400' \
        && pass "malformed capabilities refused: '$caps'" || fail "invalid caps accepted: '$caps'"
done
mast3 phoneD fw-d1 "$N1" - "" screen-time-v1 future-parent | head -1 | grep -q '^Status: 403' \
    && pass "capability names must be signed" || fail "capability substitution accepted"
mast3 phoneD fw-d1 "$N1" - "" screen-time-v1 screen-time-v1 "$N2" | head -1 | grep -q '^Status: 403' \
    && pass "capability signature is nonce-bound" || fail "capability signature replayed"
v3payload phoneD fw-d1 "$N1" "$rev" >/dev/null
assert_json "successful compatible sync clears the refusal" "$(call devices)" '@.devices[@.id="fw-d1"].apps_error' ""
assert_json "compatible signed acknowledgement restored" "$(call devices)" '@.devices[@.id="fw-d1"].apps_applied' "$rev"
# An acknowledgement from a client that lacks a feature the policy needs LATER must
# not look current: legacy v3 phone acks the app-only policy, then downtime is added.
prev=$(v3payload phoneC fw-c1 "$N1" - "" legacy | jsonfilter -e '@.apps.rev')
v3payload phoneC fw-c1 "$N1" "$prev" "" legacy >/dev/null
assert_json "legacy client acknowledged the app-only policy" "$(call devices)" '@.devices[@.id="fw-c1"].apps_applied' "$prev"
uci set familywarden.parent_bed=app_downtime; uci set familywarden.parent_bed.profile=parent
uci set familywarden.parent_bed.days=mon; uci set familywarden.parent_bed.start=21:00; uci set familywarden.parent_bed.stop=22:00
uci commit familywarden; "$BIN" reload >/dev/null
assert_json "old acknowledgement cannot imply P7b compatibility" "$(call devices)" '@.devices[@.id="fw-c1"].apps_applied' ""
assert_json "incompatible client is marked before another fetch" "$(call devices)" '@.devices[@.id="fw-c1"].apps_error' "unsupported_capabilities"
uci delete familywarden.parent_bed; uci commit familywarden; "$BIN" reload >/dev/null
assert_json "basic app-only policy still works on legacy v3" "$(v3payload phoneC fw-c1 "$N1" - "" legacy)" '@.binding.profile_id' "parent"
# Cannot merely append an unsigned assertion of support to an otherwise valid v3 request.
mast3 phoneD fw-d1 "$N1" - >/dev/null
REQUEST_METHOD=GET QUERY_STRING="device=fw-d1&nonce=$N1&key=$(enc "$(sed -n 2p /tmp/phoneD.pub)")&sig=$(enc "$(sed -n 2p /tmp/m3.sig2)")&sig3=$(enc "$(sed -n 2p /tmp/m3.sig3)")&applied=-&caps=screen-time-v1" sh "$MAST" | head -1 | grep -q '^Status: 400' \
    && pass "unsigned feature claims refused" || fail "unsigned capabilities trusted"
assert_json "multiple sorted feature names accepted" "$(v3payload phoneD fw-d1 "$N1" - "" apps-v1,screen-time-v1)" '@.binding.profile_id' "kid"
[ -z "$(v2payload phoneD fw-d1 "$N1" | jsonfilter -e '@.apps')" ] && pass "v2 replies never carry apps" || fail "apps in a v2 reply"
assert_json "stripping v3 negotiation cannot obtain a weaker v2 binding" "$(v2payload phoneD fw-d1 "$N1")" '@.error' "unsupported_capabilities"
[ -z "$(v2payload phoneD fw-d1 "$N1" | jsonfilter -e '@.binding')" ] && pass "v2 cannot bind an app-controlled profile" || fail "v2 binding bypasses app rules"
assert_json "pending phone gets v3 but nothing else" "$(v3payload phoneB fw-p1 "$N1" -)" '@.enrollment' "pending"
[ -z "$(v3payload phoneB fw-p1 "$N1" - | jsonfilter -e '@.apps')" ] && pass "no apps before approval" || fail "pending phone got apps"

echo "=== Parent approval scopes (signed, no grants yet) ==="
assert_json "no parent scope by default" "$(v3payload phoneC fw-c1 "$N1" - "" legacy)" '@.binding.approves[0]' ""
assert_json "cannot approve for its own profile" "$(call device_update '{"device":"fw-c1","approves":"parent"}')" '@.error' "a phone cannot approve for its own profile"
assert_json "cannot approve for an unknown profile" "$(call device_update '{"device":"fw-c1","approves":"nobody"}')" '@.error' "approves: profile not found"
assert_json "invalid profile ids refused" "$(call device_update '{"device":"fw-c1","approves":"kid;x"}')" '@.error' "invalid profile in approves"
assert_json "parent may approve for a child" "$(call device_update '{"device":"fw-c1","approves":"teen kid kid"}')" '@.status' "ok"
assert_json "approval scope stored sorted and unique" "$(call devices)" '@.devices[@.id="fw-c1"].approves' "kid teen"
payload=$(v3payload phoneC fw-c1 "$N1" - "" legacy)
assert_json "parent phone gets its signed scope" "$payload" '@.binding.approves[0].profile_id' "kid"
assert_json "scope carries display names" "$payload" '@.binding.approves[0].profile_name' "Kid"
assert_json "parent scope is not administrator" "$payload" '@.binding.administrator' "false"
payload=$(v3payload phoneD fw-d1 "$N1" -)
assert_json "child phone learns its parent's device" "$payload" '@.parents[0].device_id' "fw-c1"
assert_json "child phone learns the parent's enrolled key" "$payload" '@.parents[0].pubkey' "$(sed -n 2p /tmp/phoneC.pub)"
[ -z "$(v2payload phoneC fw-c1 "$N1" | jsonfilter -e '@.binding.approves')" ] && pass "v2 replies carry no parent scope" || fail "scope in v2"
assert_json "devices lists the stored scope" "$(call devices)" '@.devices[@.id="fw-c1"].approves' "kid teen"
assert_json "devices lists the signed scope" "$(call devices)" '@.devices[@.id="fw-c1"].approves_effective' "kid teen"
assert_json "omitting approves on update keeps the scope" "$(call device_update '{"device":"fw-c1","name":"Taylor phone"}')" '@.status' "ok"
assert_json "scope kept when not sent" "$(call devices)" '@.devices[@.id="fw-c1"].approves' "kid teen"

uci set familywarden.kid.apps_suspend_new='0'
uci commit familywarden
"$BIN" reload >/dev/null
payload=$(v3payload phoneD fw-d1 "$N1" -)
assert_json "suspend_new can be turned off" "$payload" '@.apps.suspend_new' "false"
[ "$(echo "$payload" | jsonfilter -e '@.apps.rev')" != "$rev" ] && pass "any change gives a new revision" || fail "revision unchanged"
uci set familywarden.kid.apps_suspend_new='1'
uci commit familywarden
"$BIN" reload >/dev/null

call pact "{\"until\":0,\"window_day\":\"$tomorrow\",\"window_start\":\"10:00\",\"window_stop\":\"11:00\"}" >/dev/null
uci delete familywarden.kid_tiktok
uci set familywarden.kid_roblox2.minutes='600'
uci delete familywarden.kid_bed
uci set familywarden.kid.apps='0'
uci commit familywarden
"$BIN" reload >/dev/null
payload=$(v3payload phoneD fw-d1 "$N1" -)
assert_json "pact: app rules frozen (revision)" "$payload" '@.apps.rev' "$rev"
assert_json "pact: block can't be removed live" "$payload" '@.apps.blocked[0]' "com.zhiliaoapp.musically"
[ -z "$(echo "$payload" | jsonfilter -e '@.parents[0]')" ] && pass "pact: captain's phone trusts no parent keys" || fail "parent keys for a pact-bound captain"
[ "$(v3payload phoneC fw-c1 "$N1" - "" legacy | jsonfilter -e '@.binding.approves[*].profile_id')" = "teen" ] \
    && pass "pact: parent scope over the captain is suspended" || fail "parent scope over the captain during the pact"
assert_json "pact: unrelated edits keep an existing (suspended) scope" "$(call device_update '{"device":"fw-c1","name":"Taylor"}')" '@.status' "ok"
assert_json "pact: LuCI sees the scope suspended" "$(call devices)" '@.devices[@.id="fw-c1"].approves_effective' "teen"
assert_json "pact: resending the stored scope is allowed" "$(call device_update '{"device":"fw-c1","approves":"kid teen"}')" '@.status' "ok"
assert_json "pact: scopes can always be reduced" "$(call device_update '{"device":"fw-c1","approves":""}')" '@.status' "ok"
assert_json "an empty scope clears parent power" "$(call devices)" '@.devices[@.id="fw-c1"].approves' ""
assert_json "pact: no new parent power over the captain" "$(call device_update '{"device":"fw-c1","approves":"kid"}')" '@.error' "bound by Ulysses pact"
assert_json "pact: limit can't be raised live" "$payload" '@.apps.limits[@.package="com.roblox.client"].minutes' "30"
assert_json "pact: downtime can't be deleted live" "$payload" '@.apps.downtime[0].start' "21:00"
assert_json "pact: required features also use frozen policy" "$(v3payload phoneD fw-d1 "$N1" - "" legacy)" '@.error' "unsupported_capabilities"
echo $(( $(date +%s) - 1 )) > /etc/familywarden/release
"$BIN" tick
assert_json "disabling app control clears stale capability errors" "$(call devices)" '@.devices[@.id="fw-d1"].apps_error' ""
[ -z "$(v3payload phoneD fw-d1 "$N1" - | jsonfilter -e '@.apps')" ] && pass "after the pact the live config applies (app control off)" || fail "stale apps after the pact"

echo ""
echo "=== Total screen time, always allowed and bonus (D9) ==="
uci set familywarden.kid.apps='1'
uci set familywarden.kid.screen_minutes='120'
uci set familywarden.kid.screen_weekend='180'
uci add_list familywarden.kid.always_allowed='com.example.school'
uci add_list familywarden.kid.always_allowed='not a package'
uci commit familywarden
"$BIN" reload >/dev/null
payload=$(v3payload phoneD fw-d1 "$N1" -)
assert_json "all-days value on a weekday" "$payload" '@.apps.screen.mon' "120"
assert_json "weekend value overrides" "$payload" '@.apps.screen.sun' "180"
assert_json "always allowed app" "$payload" '@.apps.always_allowed[0]' "com.example.school"
assert_json "daily screen-time policy refuses legacy v3" "$(v3payload phoneD fw-d1 "$N1" - "" legacy)" '@.error' "unsupported_capabilities"
[ "$(count "$payload" '@.apps.always_allowed[*]')" = 1 ] && pass "invalid always-allowed entry ignored" || fail "always_allowed: $(echo "$payload" | jsonfilter -e '@.apps.always_allowed')"
[ -z "$(echo "$payload" | jsonfilter -e '@.apps.bonus')" ] && pass "no bonus object without bonus" || fail "unexpected bonus"
rev1=$(echo "$payload" | jsonfilter -e '@.apps.rev')
uci set familywarden.kid.screen_weekday='60'
uci set familywarden.kid.screen_minutes='abc'
uci commit familywarden
"$BIN" reload >/dev/null
payload=$(v3payload phoneD fw-d1 "$N1" -)
assert_json "weekday value overrides all days" "$payload" '@.apps.screen.wed' "60"
assert_json "weekend still set" "$payload" '@.apps.screen.sat' "180"
[ "$(echo "$payload" | jsonfilter -e '@.apps.rev')" != "$rev1" ] && pass "screen time change gives a new revision" || fail "revision unchanged"
uci set familywarden.kid.screen_minutes='120'
uci commit familywarden

assert_json "bonus: minutes are bounded" "$(call bonus '{"profile":"kid","minutes":241}')" '@.error' "minutes must be 1..240"
assert_json "bonus: zero refused" "$(call bonus '{"profile":"kid","minutes":0}')" '@.error' "minutes must be 1..240"
assert_json "bonus: unknown profile" "$(call bonus '{"profile":"nobody","minutes":15}')" '@.error' "profile not found"
assert_json "bonus: needs app control" "$(call bonus '{"profile":"teen","minutes":15}')" '@.error' "app control is off for this profile"
assert_json "bonus: invalid package" "$(call bonus '{"profile":"kid","minutes":15,"package":"../x"}')" '@.error' "invalid package"
assert_json "bonus +30 on the total" "$(call bonus '{"profile":"kid","minutes":30}')" '@.minutes_today' "30"
assert_json "bonus adds up" "$(call bonus '{"profile":"kid","minutes":15}')" '@.minutes_today' "45"
assert_json "bonus for one app" "$(call bonus '{"profile":"kid","minutes":20,"package":"com.roblox.client"}')" '@.target' "com.roblox.client"
payload=$(v3payload phoneD fw-d1 "$N1" -)
assert_json "bonus in the signed policy: day" "$payload" '@.apps.bonus.day' "$(date +%F)"
assert_json "bonus in the signed policy: total" "$payload" '@.apps.bonus.total' "45"
assert_json "bonus in the signed policy: app" "$payload" '@.apps.bonus.apps[0].minutes' "20"
assert_json "bonus leaves the standing limit alone" "$payload" '@.apps.screen.mon' "60"
rev2=$(echo "$payload" | jsonfilter -e '@.apps.rev')
assert_json "devices: expected revision includes the bonus" "$(call devices)" '@.devices[@.id="fw-d1"].apps_rev' "$rev2"
assert_json "list: app control flag for LuCI" "$(call list)" '@.profiles.kid.apps' "true"
assert_json "list: today's bonus for LuCI" "$(call list)" '@.profiles.kid.bonus_today' "45"
assert_json "list: no app control elsewhere" "$(call list)" '@.profiles.teen.apps' "false"
grep -q "^kid|$(date +%F)|total|45$" /etc/familywarden/bonus && pass "bonus persisted (survives a router reboot)" || fail "bonus file: $(cat /etc/familywarden/bonus)"
echo "kid|2000-01-01|total|99" >> /etc/familywarden/bonus
call bonus '{"profile":"kid","minutes":1}' >/dev/null
grep -q '2000-01-01' /etc/familywarden/bonus && fail "old bonus days kept" || pass "old bonus days dropped"
sed -i "s/|$(date +%F)|/|2000-01-02|/" /etc/familywarden/bonus
[ -z "$(v3payload phoneD fw-d1 "$N1" - | jsonfilter -e '@.apps.bonus')" ] && pass "yesterday's bonus is not sent" || fail "stale bonus sent"

# Captain: screen time and always-allowed apps freeze with the pact; no self-bonus.
call pact "{\"until\":0,\"window_day\":\"$tomorrow\",\"window_start\":\"10:00\",\"window_stop\":\"11:00\"}" >/dev/null
assert_json "no bonus for a captain under a pact" "$(call bonus '{"profile":"kid","minutes":15}')" '@.error' "bound by Ulysses pact"
uci set familywarden.kid.screen_minutes='999'
uci set familywarden.kid.screen_weekday='999'
uci del_list familywarden.kid.always_allowed='com.example.school'
uci add_list familywarden.kid.always_allowed='com.example.games'
uci commit familywarden
"$BIN" reload >/dev/null
payload=$(v3payload phoneD fw-d1 "$N1" -)
assert_json "pact: screen time frozen" "$payload" '@.apps.screen.wed' "60"
assert_json "pact: always allowed frozen" "$payload" '@.apps.always_allowed[0]' "com.example.school"
echo $(( $(date +%s) - 1 )) > /etc/familywarden/release
"$BIN" tick
assert_json "after the pact: live screen time" "$(v3payload phoneD fw-d1 "$N1" -)" '@.apps.screen.wed' "999"

echo ""
echo "=== Extra-time requests and grants (P8, in the core) ==="
APP=/src/familywarden/www-familywarden/cgi-bin/app
# $1 phone key, $2 kind (request|decision), $3 exact JSON: POST signature line + bytes
upload() {
    printf 'familywarden-%s-v1\n%s' "$2" "$3" > /tmp/up.msg
    usign -S -m /tmp/up.msg -s "/tmp/$1.key" -x /tmp/up.sig
    { sed -n 2p /tmp/up.sig; printf '%s' "$3"; } > /tmp/up.body
    REQUEST_METHOD=POST QUERY_STRING="op=$2" sh "$APP" < /tmp/up.body
}
upbody() { upload "$@" | sed '1,/^$/d' | sed 1d; }
call device_update '{"device":"fw-c1","approves":"kid"}' >/dev/null
now=$(date +%s)
req="{\"type\":\"request\",\"v\":1,\"request_id\":\"req-int-0001\",\"device\":\"fw-d1\",\"profile\":\"kid\",\"target\":\"com.roblox.client\",\"minutes\":30,\"label\":\"Roblox\",\"created\":$now}"
upload phoneD request "$req" > /tmp/up.out
head -1 /tmp/up.out | grep -q '^Status: 200' && pass "request accepted on the phone port" || fail "request: $(head -1 /tmp/up.out)"
sed '1,/^$/d' /tmp/up.out > /tmp/up.reply
printf 'untrusted comment: test\n%s\n' "$(sed -n 1p /tmp/up.reply)" > /tmp/up.rsig; sed 1d /tmp/up.reply > /tmp/up.rjson
usign -V -q -m /tmp/up.rjson -p /tmp/mast.pub -x /tmp/up.rsig && pass "request reply signed by the router" || fail "unsigned request reply"
assert_json "request is pending" "$(cat /tmp/up.rjson)" '@.state' "pending"
upload phoneC request "$req" | head -1 | grep -q '^Status: 403' && pass "request signed by another phone refused" || fail "foreign request accepted"
REQUEST_METHOD=GET QUERY_STRING="op=request" sh "$APP" </dev/null | head -1 | grep -q '^Status: 400' && pass "GET refused" || fail "GET accepted"
assert_json "one open request per child" "$(upbody phoneD request "$(echo "$req" | sed 's/req-int-0001/req-int-0009/')")" '@.error' "request_open"
hash=$(printf '%s' "$req" | sha256sum | cut -d' ' -f1)
assert_json "upload reply binds the exact message" "$(cat /tmp/up.rjson)" '@.message_sha256' "$hash"
assert_json "empty decision hash is refused" "$(upbody phoneC decision "{\"type\":\"decision\",\"v\":1,\"request_id\":\"req-int-0001\",\"request_sha256\":\"\",\"device\":\"fw-c1\",\"approve\":true,\"minutes\":20,\"created\":$now}")" '@.error' "malformed"
payload=$(v3payload phoneC fw-c1 "$N1" - "" apps-v1,screen-time-v1)
assert_json "parent phone sees the pending request" "$payload" '@.requests[0].request_id' "req-int-0001"
assert_json "...with the hash of the exact request" "$payload" '@.requests[0].request_sha256' "$hash"
assert_json "child phone sees its pending request" "$(v3payload phoneD fw-d1 "$N1" - "" apps-v1,screen-time-v1)" '@.request.state' "pending"
assert_json "a child cannot answer its own request" "$(upbody phoneD decision "{\"type\":\"decision\",\"v\":1,\"request_id\":\"req-int-0001\",\"request_sha256\":\"$hash\",\"device\":\"fw-d1\",\"approve\":true,\"minutes\":30,\"created\":$now}")" '@.error' "not_parent"
dec="{\"type\":\"decision\",\"v\":1,\"request_id\":\"req-int-0001\",\"request_sha256\":\"$hash\",\"device\":\"fw-c1\",\"approve\":true,\"minutes\":20,\"created\":$now}"
out=$(upbody phoneC decision "$dec")
assert_json "parent approves 20 of 30 minutes" "$out" '@.granted' "20"
assert_json "grant opens the home internet" "$out" '@.home_internet' "true"
until=$(echo "$out" | jsonfilter -e '@.not_after')
[ "$until" -ge $((now + 7190)) ] && [ "$until" -le $((now + 7210)) ] && pass "grant valid two hours after approval" || fail "not_after $until (now $now)"
assert_json "ticket until the grant ends" "$(call status '{"profile":"kid"}')" '@.override_expires' "$until"
assert_json "identical retry gives the same result" "$(upbody phoneC decision "$dec")" '@.not_after' "$until"
assert_json "a second answer is refused" "$(call request_answer '{"request_id":"req-int-0001","approve":false}')" '@.error' "already_answered"
assert_json "phone without grant support is refused, not silently weakened" "$(v3payload phoneD fw-d1 "$N1" - "" apps-v1,screen-time-v1)" '@.error' "unsupported_capabilities"
payload=$(v3payload phoneD fw-d1 "$N1" - "" apps-v1,grants-v1,screen-time-v1)
assert_json "grant in the child's signed policy" "$payload" '@.apps.grants[0].minutes' "20"
assert_json "grant target" "$payload" '@.apps.grants[0].target' "com.roblox.client"
assert_json "grant bound to the requesting phone" "$payload" '@.apps.grants[0].device_id' "fw-d1"
assert_json "child sees the approval" "$payload" '@.request.state' "approved"
grev=$(echo "$payload" | jsonfilter -e '@.apps.rev')
assert_json "LuCI expects the grant revision" "$(call devices)" '@.devices[@.id="fw-d1"].apps_rev' "$grev"
v3payload phoneD fw-d1 "$N1" "$grev" "" apps-v1,grants-v1,screen-time-v1 >/dev/null
assert_json "acknowledged grant shows as delivered" "$(call requests)" '@.requests[0].delivered' "true"
call reset '{"profile":"kid"}' >/dev/null
req2=$(echo "$req" | sed 's/req-int-0001/req-int-0002/')
upbody phoneD request "$req2" >/dev/null
call block '{"profile":"kid"}' >/dev/null
out=$(call request_answer '{"request_id":"req-int-0002","approve":true,"minutes":10,"ubus_rpc_session":"00000000000000000000000000000000"}')
assert_json "administrator can approve from LuCI" "$out" '@.state' "approved"
assert_json "a manual block is not overridden by a grant" "$out" '@.home_internet' "false"
assert_json "...and stays in force" "$(call status '{"profile":"kid"}')" '@.override' "block"
call reset '{"profile":"kid"}' >/dev/null

echo ""
echo ""
echo "=== Usage reports (cumulative, authenticated, monotonic) ==="
day=$(date +%F)
gid=$(v3payload phoneD fw-d1 "$N1" - "" apps-v1,grants-v1,screen-time-v1 | jsonfilter -e '@.apps.grants[0].grant_id')
usage1="{\"type\":\"usage\",\"v\":1,\"device\":\"fw-d1\",\"profile\":\"kid\",\"day\":\"$day\",\"seq\":1,\"total\":900,\"apps\":{\"com.roblox.client\":600},\"labels\":{\"com.roblox.client\":\"Roblox\"},\"grants_used\":{\"$gid\":300}}"
out=$(upbody phoneD usage "$usage1")
assert_json "usage report stored" "$out" '@.state' "stored"
assert_json "usage retry is idempotent" "$(upbody phoneD usage "$usage1")" '@.state' "unchanged"
assert_json "usage can never decrease" "$(upbody phoneD usage "$(echo "$usage1" | sed 's/"seq":1/"seq":2/; s/"total":900/"total":800/')")" '@.error' "usage_regressed"
assert_json "usage for another profile refused" "$(upbody phoneD usage "$(echo "$usage1" | sed 's/"profile":"kid"/"profile":"teen"/')")" '@.error' "wrong_profile"
upload phoneC usage "$usage1" | head -1 | grep -q '^Status: 403' && pass "usage signed by another phone refused" || fail "foreign usage accepted"
assert_json "parent phone sees today's usage of its child" "$(v3payload phoneC fw-c1 "$N1" - "" apps-v1,screen-time-v1)" '@.usage[0].total' "900"
assert_json "...with app labels" "$(v3payload phoneC fw-c1 "$N1" - "" apps-v1,screen-time-v1)" '@.usage[0].apps[0].label' "Roblox"
assert_json "administrator usage view" "$(call usage)" '@.profiles[@.id="kid"].total' "900"
call requests | jsonfilter -e '@.requests[*].used' | grep -qx 300 && pass "reported grant use shown with the request" || fail "grant use missing: $(call requests)"
ls /etc/familywarden/core/usage/kid/"$day".json >/dev/null 2>&1 && pass "usage persisted in the core store" || fail "usage file missing"
usign -G -p /tmp/phoneE.pub -s /tmp/phoneE.key >/dev/null
v2payload phoneE fw-e1 "$N1" >/dev/null
call device_approve "{\"device\":\"fw-e1\",\"profile\":\"kid\",\"name\":\"Alex tablet\",\"administrator\":\"0\",\"fingerprint\":\"$(fpr phoneE)\"}" >/dev/null
payload=$(v3payload phoneE fw-e1 "$N1" - "" apps-v1,grants-v1,screen-time-v1,shared-v1)
assert_json "sibling phone gets the other phone's usage today (shared budget)" "$payload" '@.shared.total' "900"
assert_json "...per app" "$payload" '@.shared.apps["com.roblox.client"]' "600"
echo "$payload" | jsonfilter -e '@.requires[*]' | grep -qx shared-v1 && pass "shared usage is a required feature" || fail "shared-v1 not required"
assert_json "the reporting phone does not count itself twice" "$(v3payload phoneD fw-d1 "$N1" - "" apps-v1,grants-v1,screen-time-v1,shared-v1)" '@.shared' ""
assert_json "an app without shared budgets is refused, not given a weaker policy" "$(v3payload phoneE fw-e1 "$N1" - "" apps-v1,grants-v1,screen-time-v1)" '@.error' "unsupported_capabilities"
usage_new="{\"type\":\"usage\",\"v\":1,\"device\":\"fw-d1\",\"profile\":\"kid\",\"day\":\"$day\",\"seq\":3,\"total\":900,\"apps\":{\"com.roblox.client\":600},\"labels\":{\"com.roblox.client\":\"Roblox\"},\"grants_used\":{\"$gid\":300},\"new_apps\":{\"com.example.newgame\":\"New Game\"}}"
assert_json "phone reports a paused new app" "$(upbody phoneD usage "$usage_new")" '@.state' "unchanged"
assert_json "administrator sees it waiting" "$(call new_apps)" '@.profiles[@.id="kid"].pending[0].package' "com.example.newgame"
assert_json "parent phone sees it waiting" "$(v3payload phoneC fw-c1 "$N1" - "" apps-v1,screen-time-v1)" '@.new_apps[0].label' "New Game"
before=$(v3payload phoneD fw-d1 "$N1" - "" apps-v1,grants-v1,screen-time-v1,shared-v1 | jsonfilter -e '@.apps.rev')
dec="{\"type\":\"app_decision\",\"v\":1,\"device\":\"fw-c1\",\"profile\":\"kid\",\"package\":\"com.example.newgame\",\"action\":\"approve\",\"created\":$(date +%s)}"
assert_json "parent phone allows it (signed)" "$(upbody phoneC app "$dec")" '@.state' "approved"
payload=$(v3payload phoneD fw-d1 "$N1" - "" apps-v1,grants-v1,screen-time-v1,shared-v1)
echo "$payload" | jsonfilter -e '@.apps.approved[*]' | grep -qx com.example.newgame && pass "allowed app is in the child's signed policy" || fail "approval missing: $payload"
[ "$(echo "$payload" | jsonfilter -e '@.apps.rev')" != "$before" ] && pass "approval changes the app revision" || fail "revision unchanged"
assert_json "LuCI removal goes through the shim with a session" "$(call app_decision '{"profile":"kid","package":"com.example.newgame","action":"revoke","ubus_rpc_session":"00000000000000000000000000000000"}')" '@.state' "revoked"
# The syncs above correctly restored the active grant ticket; start the next case clean.
call reset '{"profile":"kid"}' >/dev/null

# An invalid compiler input must not leave RAM intent that a retry mistakes for
# a working ticket. The last valid nft rules must remain in place.
parent_policy=$(uci -q get familywarden.parent.policy)
uci set familywarden.parent.policy=block
uci add_list familywarden.parent.device=not-a-mac
uci commit familywarden
out=$("$BIN" core-ticket kid "$until")
assert_json "failed firewall apply is reported" "$out" '@.error' "compile failed"
assert_json "failed grant ticket restores the previous RAM override" "$(call status '{"profile":"kid"}')" '@.override' "none"
uci del_list familywarden.parent.device=not-a-mac
uci set "familywarden.parent.policy=$parent_policy"
uci commit familywarden
assert_json "the ticket retry really applies it" "$("$BIN" core-ticket kid "$until")" '@.opened' "true"
for trial in 1 2 3 4 5 6 7 8; do
    call reset '{"profile":"kid"}' >/dev/null
    "$BIN" core-ticket kid "$until" >/dev/null &
    ticket_pid=$!
    call block '{"profile":"kid"}' >/dev/null
    wait "$ticket_pid"
    assert_json "concurrent block wins over grant ticket ($trial)" "$(call status '{"profile":"kid"}')" '@.override' "block"
done
call reset '{"profile":"kid"}' >/dev/null
stop_core
start_core
assert_json "request grant survives a real daemon restart" "$(call requests)" '@.requests[0].state' "approved"
restarted_payload=$(v3payload phoneD fw-d1 "$N1" - "" apps-v1,grants-v1,screen-time-v1)
expected_ticket=$(echo "$restarted_payload" | jsonfilter -e '@.apps.grants[*].not_after' | sort -n | tail -1)
after_restart=$(call status '{"profile":"kid"}' | jsonfilter -e '@.override_expires')
[ "$after_restart" = "$expected_ticket" ] && pass "child sync restores a missing ticket without extending the grant" || fail "wrong grant ticket after restart"
nft list set inet fw4 familywarden_ticket | grep -q "aa:bb:cc:00:00:01" && pass "restored grant ticket is in the kernel timeout set" || fail "restored ticket missing in nft"
echo "=== Core: one-time import of UCI phones, restart, core down ==="
stop_core
mv /etc/familywarden/core/devices.json /tmp/devices.keep
sec=dev_fw_legacy
uci set "familywarden.$sec=device"; uci set "familywarden.$sec.id=fw-legacy"; uci set "familywarden.$sec.name=Old phone"
uci set "familywarden.$sec.profile=parent"; uci set "familywarden.$sec.pubkey=$(sed -n 2p /tmp/phoneA.pub)"
uci set "familywarden.$sec.administrator=1"; uci set "familywarden.$sec.approves=parent teen"
uci set familywarden.dev_bad=device; uci set familywarden.dev_bad.id='bad id!'; uci set familywarden.dev_bad.pubkey=x
uci commit familywarden; "$BIN" reload >/dev/null
start_core
out=$(call devices)
assert_json "legacy phone imported" "$out" '@.devices[@.id="fw-legacy"].name' "Old phone"
assert_json "import keeps administrator" "$out" '@.devices[@.id="fw-legacy"].administrator' "1"
assert_json "import drops a self-approval scope" "$out" '@.devices[@.id="fw-legacy"].approves' "teen"
[ "$(echo "$out" | jsonfilter -e '@.devices[*].id' | wc -l)" = 1 ] && pass "invalid legacy entry not imported" || fail "imported: $(echo "$out" | jsonfilter -e '@.devices[*].id')"
[ -z "$(uci -q show familywarden | grep '=device$')" ] && pass "UCI phones removed after a verified import" || fail "UCI phones left"
assert_json "imported phone keeps syncing with its key" "$(v3payload phoneA fw-legacy "$N1" -)" '@.enrollment' "approved"
stop_core
uci set familywarden.dev_late=device; uci set familywarden.dev_late.id=fw-late; uci set familywarden.dev_late.profile=parent
uci set "familywarden.dev_late.pubkey=$(sed -n 2p /tmp/phoneB.pub)"; uci commit familywarden
start_core
[ -z "$(call devices | jsonfilter -e '@.devices[@.id="fw-late"].id')" ] && pass "import runs only once" || fail "re-imported after restart"
assert_json "registry survives a core restart" "$(call devices)" '@.devices[@.id="fw-legacy"].profile' "parent"
stop_core
mast2 phoneA fw-legacy "$N1" | head -1 | grep -q '^Status: 503' && pass "core down: no reply, never an unsigned one" || fail "reply while the core is down"
assert_json "core down: LuCI gets an explicit error" "$(call devices)" '@.error' "core not reachable"
start_core

echo ""
if [ "$FAILED" -eq 0 ]; then
    echo "All integration checks passed!"
else
    echo "Some integration checks FAILED"
fi
exit "$FAILED"
