#!/bin/sh
# Integration test: runs the real rpcd plugin against real fw4 + nftables in an
# OpenWrt rootfs container, then asserts on the live ruleset and ubus replies.
#
#   ./scripts/test-integration.sh              # host: starts the container
#   OPENWRT_IMAGE=openwrt/rootfs:x86-64-25.12.4 ./scripts/test-integration.sh
#
# FW_TEST_FOCUS=bonus, network, status or pact-network runs only that narrow bridge slice.
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
    docker run --rm --memory=768m --cpus=2 --cap-add NET_ADMIN -e FW_TEST_FOCUS="${FW_TEST_FOCUS:-}" -v "$REPO:/src:ro" -v "$CORE_DIR:/core:ro" "$IMAGE" \
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

# Deterministic lock probes: let a mutation queue behind the same platform lock
# that reload and policy snapshots use, then observe state before releasing it.
hold_state_lock() {
    rm -f /tmp/state-lock.ready /tmp/state-lock.release
    (
        flock -x 200 || exit 1
        touch /tmp/state-lock.ready
        while [ ! -f /tmp/state-lock.release ]; do sleep 1; done
    ) 200>/var/lock/familywarden.lock &
    STATE_LOCK_PID=$!
    for _ in $(seq 1 15); do [ -f /tmp/state-lock.ready ] && return 0; sleep 1; done
    fail "could not hold state lock for regression probe"
}
release_state_lock() {
    touch /tmp/state-lock.release
    wait "$STATE_LOCK_PID"
    rm -f /tmp/state-lock.ready /tmp/state-lock.release
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

# Shared failure injection for the two focused native transaction checks.
reject_next_validation() {
    nft_bin=$(command -v nft)
    mv "$nft_bin" "$nft_bin.network-real"
    cat > "$nft_bin" <<EOF
#!/bin/sh
if [ "\$1" = -c ] && [ -f /tmp/reject-network-once ]; then
    cp /tmp/familywarden.state /tmp/rejected-network.state
    rm /tmp/reject-network-once
    exit 1
fi
exec "$nft_bin.network-real" "\$@"
EOF
    chmod 755 "$nft_bin"
    touch /tmp/reject-network-once
}
restore_nft_binary() { mv "$nft_bin.network-real" "$nft_bin"; }

if [ "${FW_TEST_FOCUS:-}" = pact-network ]; then
    echo "=== focused pact permit revocation and rollback ==="
    uci set familywarden.settings.ulysses_profile=kid
    uci set familywarden.settings.ulysses_delay=0
    uci commit familywarden
    "$BIN" reload >/dev/null
    ram_override() { awk -F= -v p="$1" '$1==p".override" { print $2 }' /tmp/familywarden.state; }
    pact_api() { printf '%s' "${2:-{\}}" | /usr/sbin/familywardend -socket "$CORE_SOCK" api "$1"; }
    assert_json "legacy permit preparation succeeds" "$(call allow '{"profile":"kid"}')" '@.status' ok
    assert_json "legacy pact creation still works" "$(call pact "{\"until\":$(( $(date +%s) + 3600 ))}")" '@.status' ok
    [ "$(ram_override kid)" = none ] && pass "legacy pact still clears its old allow" || fail "legacy revocation changed"
    "$BIN" sail-request >/dev/null
    "$BIN" tick >/dev/null
    # Pause one legacy request after its ownership check, before taking its
    # first lock. Perform a real import, then let the queued request continue.
    mkdir -p /tmp/pact-queued-bin
    real_flock=$(command -v flock)
    cat > /tmp/pact-queued-bin/flock <<EOF
#!/bin/sh
if [ "\$*" = '-x 201' ]; then
    touch /tmp/pact-queued.ready
    while [ ! -f /tmp/pact-queued.resume ]; do sleep 1; done
fi
exec "$real_flock" "\$@"
EOF
    chmod 755 /tmp/pact-queued-bin/flock
    printf '%s\n' "{\"until\":$(( $(date +%s) + 3600 ))}" | PATH="/tmp/pact-queued-bin:$PATH" "$BIN" call pact > /tmp/pact-queued.reply &
    queued_pid=$!
    for _ in $(seq 1 15); do [ -f /tmp/pact-queued.ready ] && break; sleep 1; done
    [ -f /tmp/pact-queued.ready ] || { fail "legacy pact did not reach queue gate"; exit 1; }
    revision=$(pact_api configuration_export | jsonfilter -e '@.configuration.revision')
    assert_json "owned pact test imports explicitly" "$(pact_api configuration_import "{\"revision\":\"$revision\"}")" '@.status' ok
    touch /tmp/pact-queued.resume
    wait "$queued_pid" || true
    assert_json "queued legacy pact cannot mutate after cutover" "$(cat /tmp/pact-queued.reply)" '@.error' 'configuration ownership changed; retry pact'
    assert_json "queued legacy pact leaves the owned document unchanged" "$(pact_api configuration_export)" '@.configuration.pact.captain' ''
    assert_json "old untagged core apply is refused" "$("$BIN" core-apply-configuration)" '@.error' 'incompatible configuration core'
    assert_json "tagged apply still requires inherited locks" "$("$BIN" core-apply-configuration pact-network-v1)" '@.error' 'configuration transaction required'
    call block '{"profile":"teen"}' >/dev/null
    call allow '{"profile":"parent"}' >/dev/null
    for permit in permanent timed; do
        if [ "$permit" = permanent ]; then
            prepared=$(call allow '{"profile":"kid"}')
        else
            prepared=$(call ticket '{"profile":"kid","minutes":10}')
        fi
        assert_json "$permit permit preparation succeeds" "$prepared" '@.status' ok
        cp /tmp/familywarden.state /tmp/pact-before-failure
        reject_next_validation
        out=$(call pact "{\"until\":$(( $(date +%s) + 3600 ))}")
        [ -n "$(echo "$out" | jsonfilter -e '@.error')" ] && pass "$permit failed pact is reported as failed" || fail "failed pact reported success"
        restore_nft_binary
        [ "$(awk -F= '$1=="kid.override" { print $2 }' /tmp/rejected-network.state)" = none ] && pass "$permit permit was revoked before native validation" || fail "revocation did not run before validation"
        cmp -s /tmp/familywarden.state /tmp/pact-before-failure && pass "$permit failed pact restores exact prior RAM" || fail "failed pact lost prior permit"
        if [ "$permit" = permanent ]; then
            assert_rule "failed pact restores permanent native allow" 'familywarden:kid:override:allow'
        else
            nft list set inet fw4 familywarden_ticket | grep -qi 'aa:bb:cc:00:00:01.*timeout' && pass "failed pact restores native timed permit" || fail "native timed permit lost"
        fi
        [ ! -e /etc/familywarden/pact ] && pass "$permit failed pact restores old pact projection" || fail "failed pact stayed active"
        stop_core
        start_core
        assert_json "$permit failed pact restores old document" "$(pact_api configuration_export)" '@.configuration.pact.captain' ''
        assert_json "$permit owned pact succeeds" "$(call pact "{\"until\":$(( $(date +%s) + 3600 ))}")" '@.status' ok
        [ "$(ram_override kid)" = none ] && pass "$permit captain permission revoked by core" || fail "captain permission survived"
        [ "$(ram_override teen)" = block ] && [ "$(ram_override parent)" = allow ] && pass "$permit other profile state preserved" || fail "unrelated override changed"
        refute_rule "$permit no permanent forward escape" 'familywarden:kid:override:allow'
        refute_rule "$permit no permanent input escape" 'familywarden:kid:override:allow:input' familywarden_input
        nft list set inet fw4 familywarden_ticket | grep -qi 'aa:bb:cc:00:00:01' && fail "captain ticket survived pact" || pass "$permit no captain kernel ticket"
        # Root-only, zero-delay release in this disposable fixture; never a public RPC.
        printf '%s' '{"answer":"press on"}' | /usr/sbin/familywardend -socket "$CORE_SOCK" release request >/dev/null
        pact_api configuration_tick >/dev/null
        [ "$(ram_override kid)" = none ] && pass "$permit release never resurrects the old permit" || fail "release revived permission"
    done
    call block '{"profile":"kid"}' >/dev/null
    assert_json "pact with an existing block succeeds" "$(call pact "{\"until\":$(( $(date +%s) + 3600 ))}")" '@.status' ok
    [ "$(ram_override kid)" = block ] && pass "captain block is retained" || fail "captain block was reset"
    assert_rule "captain block remains enforced" 'familywarden:kid:override:block'
    # Simulate stale legacy RAM under an already-bound pact. Both reapply and
    # startup reconciliation must clear it without a recursive network transaction.
    sed -i 's/^kid.override=.*/kid.override=allow/' /tmp/familywarden.state
    assert_json "bound reapply succeeds without nested locks" "$(call reload)" '@.status' ok
    [ "$(ram_override kid)" = none ] && pass "reapply revokes a stale captain allow" || fail "reapply retained stale permit"
    stop_core
    sed -i 's/^kid.override=.*/kid.override=allow/' /tmp/familywarden.state
    start_core
    [ "$(ram_override kid)" = none ] && pass "startup revokes a stale captain allow" || fail "startup retained stale permit"
    stop_core
    [ "$FAILED" = 0 ] && echo 'All focused pact-network checks passed!'
    exit "$FAILED"
fi

if [ "${FW_TEST_FOCUS:-}" = network ]; then
    echo "=== focused owned network actions and native timeout enforcement ==="
    uci set familywarden.settings.ulysses_profile=kid
    uci commit familywarden
    "$BIN" reload >/dev/null
    assert_json "legacy block remains available before import" "$(call block '{"profile":"kid"}')" '@.status' ok
    network_api() { printf '%s' "${2:-{\}}" | /usr/sbin/familywardend -socket "$CORE_SOCK" api "$1"; }
    revision=$(network_api configuration_export | jsonfilter -e '@.configuration.revision')
    assert_json "network action cutover imports explicitly" "$(network_api configuration_import "{\"revision\":\"$revision\"}")" '@.status' ok
    assert_json "cutover preserves the existing RAM block" "$(call status '{"profile":"kid"}')" '@.override' block
    assert_json "owned allow uses the original RPC" "$(call allow '{"profile":"kid","ubus_rpc_session":"synthetic"}')" '@.status' ok
    assert_rule "native permanent allow is applied" 'familywarden:kid:override:allow'
    assert_json "owned block succeeds" "$(call block '{"profile":"kid"}')" '@.status' ok
    assert_rule "native block is applied" 'familywarden:kid:override:block'
    assert_json "owned reset succeeds" "$(call reset '{"profile":"kid"}')" '@.status' ok
    refute_rule "reset removes the static override" 'familywarden:kid:override:'
    intent_id=$(jsonfilter -i /var/run/familywarden/network-intent -e '@.id')
    receipt=$(network_api configuration_status)
    assert_json "apply receipt identifies the exact core intent" "$receipt" '@.network_application.intent_id' "$intent_id"
    assert_json "apply receipt is an acknowledgement, not live verification" "$receipt" '@.network_application.state' acknowledged
    assert_json "intent preserves configuration revision" "$(cat /var/run/familywarden/network-intent)" '@.revision' "$revision"
    assert_json "owned ticket succeeds" "$(call ticket '{"profile":"kid","minutes":1}')" '@.status' ok
    first=$(awk -F= '$1=="kid.override_expires" { print $2 }' /tmp/familywarden.state)
    assert_json "second owned ticket extends" "$(call ticket '{"profile":"kid","minutes":2}')" '@.status' ok
    second=$(awk -F= '$1=="kid.override_expires" { print $2 }' /tmp/familywarden.state)
    [ "$second" = "$((first + 120))" ] && pass "manual tickets add to the existing absolute deadline" || fail "ticket did not extend"
    nft list set inet fw4 familywarden_ticket | grep -qi 'aa:bb:cc:00:00:01.*timeout' && pass "kernel timeout set contains the ticket" || fail "kernel ticket missing"
    refute_rule "timed allow is not a permanent forward escape" 'familywarden:kid:override:allow'
    refute_rule "timed allow is not a permanent input escape" 'familywarden:kid:override:allow:input' familywarden_input
    assert_json "duplicate fields are refused by core" "$(call ticket '{"profile":"kid","minutes":1,"minutes":2}')" '@.error' 'duplicate configuration field'
    assert_json "private apply requires inherited locks" "$("$BIN" core-apply-network)" '@.error' 'network transaction required'
    assert_json "owned grant repair cannot use the legacy writer" "$("$BIN" core-ticket kid "$(( $(date +%s) + 60 ))")" '@.error' 'owned ticket requires the core API'
    stop_core
    start_core
    assert_json "daemon restart preserves the RAM ticket deadline" "$(call status '{"profile":"kid"}')" '@.override_expires' "$second"
    [ "$(network_api configuration_export | jsonfilter -e '@.configuration.revision')" = "$revision" ] && pass "network actions do not change durable configuration" || fail "network action changed configuration"
    # Reject the requested compile once; restoration must apply the previous
    # state and still return the original action as failed, never a success.
    cp /tmp/familywarden.state /tmp/network-before-failure
    reject_next_validation
    out=$(call block '{"profile":"kid"}')
    [ -n "$(echo "$out" | jsonfilter -e '@.error')" ] && pass "native validation failure is reported" || fail "native failure reported success"
    restore_nft_binary
    cmp -s /tmp/familywarden.state /tmp/network-before-failure && pass "failed apply restores prior RAM intent" || fail "failed apply left new intent"
    nft list set inet fw4 familywarden_ticket | grep -qi 'aa:bb:cc:00:00:01.*timeout' && pass "failed apply restores prior native enforcement" || fail "prior enforcement missing"
    stop_core
    start_core
    # An include swap failure must not acknowledge the requested intent or
    # delete the previously active chains. Inject only in this disposable copy.
    cp "$BIN" /tmp/network-plugin-before-swap
    cp /var/etc/familywarden.include /tmp/network-include-before-swap
    cp /tmp/familywarden.state /tmp/network-state-before-swap
    sed -i 's/^        mv "\$FW_INCLUDE_NEW" "\$FW_INCLUDE"/        false/' "$BIN"
    out=$(call block '{"profile":"kid"}')
    cp /tmp/network-plugin-before-swap "$BIN"
    [ -n "$(echo "$out" | jsonfilter -e '@.error')" ] && pass "include activation failure cannot acknowledge intent" || fail "failed include swap reported success"
    cmp -s /tmp/network-state-before-swap /tmp/familywarden.state && pass "include activation failure restores RAM" || fail "failed include swap changed RAM"
    cmp -s /tmp/network-include-before-swap /var/etc/familywarden.include && pass "include activation failure retains previous include" || fail "failed include swap replaced include"
    stop_core
    start_core
    # A disposable fixture shortens the absolute deadline to test native expiry
    # without waiting minutes or adding a polling daemon.
    expiry=$(( $(date +%s) + 5 ))
    sed -i "s/^kid.override_expires=.*/kid.override_expires=$expiry/" /tmp/familywarden.state
    call reload >/dev/null
    sleep 6
    nft list set inet fw4 familywarden_ticket | grep -qi 'aa:bb:cc:00:00:01' && fail "kernel ticket survived expiry" || pass "kernel expires the ticket without polling"
    assert_json "status expires the override at read time" "$(call status '{"profile":"kid"}')" '@.override' none
    assert_json "list agrees with expired status" "$(call list)" '@.profiles.kid.override' none
    assert_json "owned pact creation succeeds" "$(call pact "{\"until\":$(( $(date +%s) + 3600 ))}")" '@.status' ok
    assert_json "bound captain cannot be allowed" "$(call allow '{"profile":"kid"}')" '@.error' 'bound by Ulysses pact'
    assert_json "bound captain cannot receive a ticket" "$(call ticket '{"profile":"kid","minutes":1}')" '@.error' 'bound by Ulysses pact'
    assert_json "captain block remains available" "$(call block '{"profile":"kid"}')" '@.status' ok
    assert_rule "captain block reaches the native firewall" 'familywarden:kid:override:block'
    assert_json "captain reset remains available" "$(call reset '{"profile":"kid"}')" '@.status' ok
    stop_core
    cp /var/run/familywarden/network-intent.nft /tmp/network-native-before
    cp /var/etc/familywarden.include /tmp/network-include-before
    for broken in missing foreign; do
        if [ "$broken" = missing ]; then
            rm /var/run/familywarden/network-intent.nft
        else
            sed '1s/network-intent-v1/network-intent-foreign/' /tmp/network-native-before > /var/run/familywarden/network-intent.nft
        fi
        out=$( (flock -x 4; "$BIN" core-apply-network) 4>/var/lock/familywarden.lock.ticket 3>/var/lock/familywarden.lock )
        assert_json "$broken native intent fails without legacy fallback" "$out" '@.error' 'network intent unavailable'
        cmp -s /tmp/network-include-before /var/etc/familywarden.include && pass "$broken intent retains the last include" || fail "$broken intent replaced the include"
    done
    cp /tmp/network-native-before /var/run/familywarden/network-intent.nft
    cp /tmp/familywarden.state /tmp/network-core-down
    assert_json "core-down manual action cannot fall back to shell" "$(call allow '{"profile":"kid"}')" '@.error' 'core not reachable'
    cmp -s /tmp/familywarden.state /tmp/network-core-down && pass "core-down action leaves RAM untouched" || fail "core-down action wrote RAM"
    [ "$FAILED" = 0 ] && echo 'All focused network checks passed!'
    exit "$FAILED"
fi

if [ "${FW_TEST_FOCUS:-}" = bonus ]; then
    echo "=== focused bonus authority and OpenWrt bridge ==="
    uci set familywarden.kid.apps=1
    uci set familywarden.kid.screen_minutes=40
    uci set familywarden.settings.ulysses_profile=kid
    uci commit familywarden
    "$BIN" reload >/dev/null
    assert_json "legacy bonus remains available before import" "$(call bonus '{"profile":"kid","minutes":15}')" '@.minutes_today' 15
    # A future entry must survive import too (e.g. after a backwards clock step).
    echo 'kid|2099-01-15|total|7' >> /etc/familywarden/bonus
    cp /etc/familywarden/bonus /tmp/legacy-bonus.keep
    bonus_api() { printf '%s' "${2:-{\}}" | /usr/sbin/familywardend -socket "$CORE_SOCK" api "$1"; }
    revision=$(bonus_api configuration_export | jsonfilter -e '@.configuration.revision')
    assert_json "explicit import captures bonus with the configuration lock" "$(bonus_api configuration_import "{\"revision\":\"$revision\"}")" '@.status' ok
    assert_json "owned bonus uses the original administrator RPC" "$(call bonus '{"profile":"kid","minutes":30,"ubus_rpc_session":"synthetic"}')" '@.minutes_today' 45
    assert_json "list reports core-owned bonus without a locked socket callback" "$(call list)" '@.profiles.kid.bonus_today' 45
    cmp -s /etc/familywarden/bonus /tmp/legacy-bonus.keep && pass "owned writes leave legacy bonus untouched" || fail "owned bonus wrote the legacy file"
    echo 'kid|2000-01-01|total|999' > /etc/familywarden/bonus
    assert_json "stale legacy state is not reimported" "$(call bonus '{"profile":"kid","minutes":5}')" '@.minutes_today' 50
    assert_json "duplicate bonus fields are refused by core" "$(call bonus '{"profile":"kid","minutes":15,"minutes":30}')" '@.error' 'duplicate configuration field'
    stop_core
    start_core
    assert_json "owned bonus survives daemon restart" "$(call list)" '@.profiles.kid.bonus_today' 50
    # Check the actual runtime journal, not only the public configuration export.
    [ "$(jsonfilter -i /etc/familywarden/core/configuration.json -e '@.bonus.entries[@.day="2099-01-15"].minutes')" = 7 ] && pass "future bonus row retained" || fail "future bonus row lost"
    assert_json "owned pact creation succeeds" "$(call pact "{\"until\":$(( $(date +%s) + 3600 ))}")" '@.status' ok
    assert_json "a bound captain cannot receive bonus" "$(call bonus '{"profile":"kid","minutes":15}')" '@.error' 'profile is bound by Ulysses pact'
    stop_core
    assert_json "core-down bonus fails instead of using legacy" "$(call bonus '{"profile":"kid","minutes":15}')" '@.error' 'core not reachable'
    assert_json "core-down totals are unavailable, not zero" "$(call list)" '@.error' 'core not reachable'
    [ "$FAILED" = 0 ] && echo 'All focused bonus checks passed!'
    exit "$FAILED"
fi

if [ "${FW_TEST_FOCUS:-}" = status ]; then
    echo "=== focused owned status/expiry reporting through core ==="
    uci set familywarden.kid.apps=1
    uci set familywarden.settings.ulysses_profile=kid
    # One 00:00-24:00 window on every day makes the scheduled status
    # deterministic at any time of day:
    uci add familywarden schedule
    uci set familywarden.@schedule[-1].profile=kid
    uci set familywarden.@schedule[-1].days='sun mon tue wed thu fri sat'
    uci set familywarden.@schedule[-1].start='00:00'
    uci set familywarden.@schedule[-1].stop='24:00'
    uci commit familywarden
    "$BIN" reload >/dev/null
    assert_json "legacy list remains available before import" "$(call list)" '@.profiles.kid.name' 'Kid'
    assert_json "legacy status remains available before import" "$(call status '{"profile":"kid"}')" '@.name' 'Kid'
    status_api() { printf '%s' "${2:-{\}}" | /usr/sbin/familywardend -socket "$CORE_SOCK" api "$1"; }
    revision=$(status_api configuration_export | jsonfilter -e '@.configuration.revision')
    assert_json "status cutover imports explicitly" "$(status_api configuration_import "{\"revision\":\"$revision\"}")" '@.status' ok
    assert_json "owned status uses the original RPC" "$(call status '{"profile":"kid"}')" '@.name' 'Kid'
    assert_json "owned schedule window reads allowed" "$(call status '{"profile":"kid"}')" '@.status' 'allowed'
    assert_json "owned block policy reads blocked" "$(call status '{"profile":"teen"}')" '@.status' 'blocked'
    assert_json "owned allow outranks the policy" "$(call allow '{"profile":"teen"}')" '@.status' ok
    assert_json "allow reads allowed" "$(call status '{"profile":"teen"}')" '@.status' 'allowed'
    assert_json "owned block outranks allow" "$(call block '{"profile":"teen"}')" '@.status' ok
    assert_json "block reads blocked" "$(call status '{"profile":"teen"}')" '@.status' 'blocked'
    assert_json "facts expose the owned manual override" "$("$BIN" core-facts runtime-facts-v1)" '@.profiles.teen.override' 'block'
    assert_json "facts keep the unmodified profile" "$("$BIN" core-facts runtime-facts-v1)" '@.profiles.kid.override' 'none'
    assert_json "owned reset restores the policy" "$(call reset '{"profile":"teen"}')" '@.status' ok
    # Read-time expiry: the kernel set keeps the ticket until its own timeout,
    # but the report must show it as none immediately after the deadline.
    call ticket '{"profile":"teen","minutes":1}' >/dev/null
    sed -i "s/^teen.override_expires=.*/teen.override_expires=$(( $(date +%s) - 1 ))/" /tmp/familywarden.state
    assert_json "expired ticket reads none without recompile" "$(call status '{"profile":"teen"}')" '@.override' none
    assert_json "list agrees with the expired status" "$(call list)" '@.profiles.teen.override' none
    [ "$(awk -F= '$1=="teen.override" { print $2 }' /tmp/familywarden.state)" = allow ] && pass "report does not rewrite RAM state" || fail "report mutated RAM state"
    call bonus '{"profile":"kid","minutes":30}' >/dev/null
    assert_json "list reports core-owned bonus" "$(call list)" '@.profiles.kid.bonus_today' 30
    # A standing pre-pact allow must read as revoked for the bound captain.
    call allow '{"profile":"kid"}' >/dev/null
    assert_json "owned pact creation succeeds" "$(call pact "{\"until\":$(( $(date +%s) + 3600 ))}")" '@.status' ok
    assert_json "bound captain reports the pact" "$(call status '{"profile":"kid"}')" '@.captain' true
    assert_json "bound captain permit reads revoked" "$(call status '{"profile":"kid"}')" '@.override' none
    assert_json "bound captain shows the sync journal" "$(call status '{"profile":"kid"}')" '@.mast_last_sync' 0
    assert_json "list carries the frozen delay" "$(call list)" '@.configuration.pact_delay_seconds' 86400
    stop_core
    assert_json "core-down status fails instead of using legacy" "$(call status '{"profile":"kid"}')" '@.error' 'core not reachable'
    assert_json "core-down list fails instead of using legacy" "$(call list)" '@.error' 'core not reachable'
    [ "$FAILED" = 0 ] && echo 'All focused status checks passed!'
    exit "$FAILED"
fi

[ -z "${FW_TEST_FOCUS:-}" ] || { echo "Unknown test focus: $FW_TEST_FOCUS"; exit 1; }

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
echo "=== Midnight endpoints stay in the kernel and validate in both chains ==="
uci set familywarden.midnight_probe=profile
uci set familywarden.midnight_probe.name='Midnight fixture'
uci set familywarden.midnight_probe.enabled='1'
uci set familywarden.midnight_probe.policy=scheduled
uci set familywarden.midnight_probe.block_router='1'
uci add_list familywarden.midnight_probe.device='aa:bb:cc:00:00:09'
for section in midnight_end midnight_start midnight_zero; do
    uci set "familywarden.$section=schedule"
    uci set "familywarden.$section.profile=midnight_probe"
done
uci set familywarden.midnight_end.days=mon
uci set familywarden.midnight_end.start=21:00
uci set familywarden.midnight_end.stop=24:00
uci set familywarden.midnight_start.days=mon
uci set familywarden.midnight_start.start=24:00
uci set familywarden.midnight_start.stop=08:00
uci set familywarden.midnight_zero.days=fri
uci set familywarden.midnight_zero.start=22:00
uci set familywarden.midnight_zero.stop=00:00
uci commit familywarden
"$BIN" reload >/dev/null && pass "midnight schedule endpoints compile and apply" || fail "midnight schedule endpoints rejected by nft"
for chain in familywarden_forward familywarden_input; do
    assert_rule "end=24:00 keeps the whole end of Monday ($chain)" 'aa:bb:cc:00:00:09 meta hour >= "21:00" meta day "Monday" .* accept' "$chain"
    assert_rule "start=24:00 begins on Tuesday ($chain)" 'aa:bb:cc:00:00:09 meta hour < "08:00" meta day "Tuesday" .* accept' "$chain"
    assert_rule "overnight ending at midnight keeps Friday ($chain)" 'aa:bb:cc:00:00:09 meta hour >= "22:00" meta day "Friday" .* accept' "$chain"
    refute_rule "no invalid 24:00 comparison ($chain)" 'meta hour (>=|<) "24:00"' "$chain"
    refute_rule "no empty next-day midnight interval ($chain)" 'aa:bb:cc:00:00:09 meta hour < "00:00"' "$chain"
done
for section in midnight_probe midnight_end midnight_start midnight_zero; do uci delete "familywarden.$section"; done
uci commit familywarden
"$BIN" reload >/dev/null

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
echo "=== Ulysses: amendment window may end at 24:00 ==="
sed -i 's/^window_stop=.*/window_stop=24:00/' /etc/familywarden/pact
"$BIN" reload >/dev/null && pass "24:00 amendment endpoint compiles" || fail "24:00 amendment endpoint rejected by nft"
assert_rule "midnight amendment uses the day boundary" 'meta day .* meta hour >= "10:00" accept comment "familywarden:ulysses:window"' familywarden_lockdown
refute_rule "midnight amendment has no invalid clock literal" 'meta hour .*"24:00"' familywarden_lockdown
sed -i 's/^window_stop=.*/window_stop=11:00/' /etc/familywarden/pact
"$BIN" reload >/dev/null

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
hold_state_lock
call pact "{\"until\":$((until + 1200))}" >/tmp/queued-pact.json &
QUEUED_PID=$!
sleep 1
[ "$(sed -n 's/^until=//p' /etc/familywarden/pact)" = "$((until + 600))" ] && pass "pact mutation waits for the state transaction lock" || fail "pact changed while another transaction held the lock"
release_state_lock
wait "$QUEUED_PID"
assert_json "queued pact extension completes after lock release" "$(cat /tmp/queued-pact.json)" '@.status' "ok"
PACT_PIDS=""
for extra in 1800 2400 3000 3600; do
    call pact "{\"until\":$((until + extra))}" >"/tmp/concurrent-pact-$extra.json" &
    PACT_PIDS="$PACT_PIDS $!"
done
for p in $PACT_PIDS; do wait "$p"; done
[ "$(sed -n 's/^until=//p' /etc/familywarden/pact)" = "$((until + 3600))" ] && pass "concurrent pact extensions cannot shorten the longest pact" || fail "a shorter concurrent pact overwrote the longest extension"
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
# Legacy-owned routers also need valid mobile midnight times, not only the
# optional authoritative-document path exercised at the end of this suite.
legacy_stop=$(uci -q get familywarden.kid_school.stop)
uci set familywarden.kid_school.stop=24:00
uci commit familywarden
"$BIN" reload >/dev/null && pass "legacy midnight schedule applies" || fail "legacy midnight schedule failed"
assert_json "legacy signed phone window has a mobile-valid midnight" "$(v2payload phoneA fw-a1 "$N1")" '@.rules.windows[0].stop' '00:00'
assert_rule "legacy midnight schedule and phone policy share the kernel boundary" 'aa:bb:cc:00:00:01 meta hour >= "07:00" meta day .* accept'
uci set "familywarden.kid_school.stop=$legacy_stop"
uci commit familywarden
"$BIN" reload >/dev/null

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
# Adapter protocol changes must refuse an old daemon, never hide app restrictions.
assert_json "old core context format is refused" "$("$BIN" core-context)" '@.error' "incompatible core: upgrade required"
assert_json "old core policy format is refused" "$("$BIN" core-policy kid 3)" '@.error' "incompatible core: upgrade required"
assert_json "old canonical-input adapter is refused" "$("$BIN" core-context go-apps-v1)" '@.error' "incompatible core: upgrade required"
assert_json "pre-transaction adapter is refused" "$("$BIN" core-context go-apps-v2)" '@.error' "incompatible core: upgrade required"
assert_json "new adapter format is explicit" "$("$BIN" core-context go-apps-v3)" '@.app_format' "go-apps-v3"
assert_json "held context requires inherited state fd" "$("$BIN" core-context go-apps-v3 held-v1 3>&-)" '@.error' "state transaction required"
assert_json "held context rejects a foreign fd" "$("$BIN" core-context go-apps-v3 held-v1 3>/tmp/foreign-state-lock)" '@.error' "state transaction required"
assert_json "unknown transaction mode is refused" "$("$BIN" core-context go-apps-v3 unknown)" '@.error' "invalid transaction mode"
# R2: the owned-mode runtime facts carry no policy content; a missing or foreign
# handshake is refused, and held-v1 needs the inherited state lock.
assert_json "facts without a handshake are refused" "$("$BIN" core-facts)" '@.error' "incompatible core: upgrade required"
assert_json "foreign facts handshake is refused" "$("$BIN" core-facts runtime-facts-v2)" '@.error' "incompatible core: upgrade required"
assert_json "facts envelope is versioned" "$("$BIN" core-facts runtime-facts-v1)" '@.runtime_format' "runtime-facts-v1"
assert_json "facts report the volatile override" "$("$BIN" core-facts runtime-facts-v1)" '@.profiles.kid.override' "none"
[ -z "$("$BIN" core-facts runtime-facts-v1 | jsonfilter -e '@.profiles.kid.app_input')" ] && pass "facts expose no app configuration" || fail "facts leaked app input"
assert_json "held facts require the inherited state fd" "$("$BIN" core-facts runtime-facts-v1 held-v1 3>&-)" '@.error' "state transaction required"
assert_json "held facts reject a foreign fd" "$("$BIN" core-facts runtime-facts-v1 held-v1 3>/tmp/foreign-state-lock)" '@.error' "state transaction required"
assert_json "unknown facts transaction mode is refused" "$("$BIN" core-facts runtime-facts-v1 unknown)" '@.error' "invalid transaction mode"
raw_policy=$("$BIN" core-policy kid 3 go-apps-v3)
[ -z "$(echo "$raw_policy" | jsonfilter -e '@.apps')" ] && pass "shell no longer builds signed app policy" || fail "shell still supplies apps"
assert_json "shell supplies raw versioned app configuration" "$raw_policy" '@.context.profiles.kid.app_input.version' "1"
directory=$(call profiles)
assert_json "core profile directory supplies selector labels" "$directory" '@.profiles[@.id="kid"].name' "Kid"
assert_json "core profile directory includes disabled profiles" "$directory" '@.profiles[@.id="guest"].name' "Guest"
[ -z "$(echo "$directory" | jsonfilter -e '@.profiles[*].app_input')" ] && pass "profile directory exposes no app configuration" || fail "directory leaked app configuration"
# Compare the actual stateless Go compiler with a frozen pre-migration oracle.
# The reference runs separately, so its shell helpers cannot affect production code.
assert_legacy_app_canonical() {
    expected=$(UCI_CONFIG_DIR=/var/run/familywarden sh -c '
        . /lib/functions.sh
        . /src/test/app-canon-legacy.sh
        config_load familywarden
        _legacy_apps_canon "$1"
    ' _ "$1")
    actual=$("$BIN" core-context go-apps-v3 | jsonfilter -e "@.profiles.$1.app_input" | /usr/sbin/familywardend app-canon)
    [ "$actual" = "$expected" ] && pass "Go normalization preserves legacy canonical bytes ($1: $2)" || fail "canonical normalization changed ($1: $2)"
}
assert_legacy_app_canonical kid "app limits and downtime"
assert_legacy_app_canonical parent "app-only policy"
assert_legacy_app_canonical teen "app control off"
uci set familywarden.canonical_probe=profile
uci set familywarden.canonical_probe.apps=1
uci set familywarden.canonical_probe.screen_minutes=1440
uci set familywarden.canonical_probe.screen_weekday=060
uci set familywarden.canonical_probe.screen_weekend=0
uci add_list familywarden.canonical_probe.always_allowed=com.example.calls
for spec in 'first:com.example.video:0015:0' 'second:com.example.video:15:0' 'third:com.example.video:99:1' 'bad:com.example.bad:invalid:0' 'huge:com.example.huge:99999:0' 'zero:com.example.zero:0000:0'; do
    old_ifs=$IFS; IFS=:; set -- $spec; IFS=$old_ifs
    uci set "familywarden.probe_$1=app"
    uci set "familywarden.probe_$1.profile=canonical_probe"
    uci set "familywarden.probe_$1.package=$2"
    uci set "familywarden.probe_$1.minutes=$3"
    uci set "familywarden.probe_$1.blocked=$4"
done
uci commit familywarden
"$BIN" reload >/dev/null
assert_legacy_app_canonical canonical_probe "duplicates, leading zeroes, malformed and oversized limits"
for section in canonical_probe probe_first probe_second probe_third probe_bad probe_huge probe_zero; do uci delete "familywarden.$section"; done
uci commit familywarden
"$BIN" reload >/dev/null
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
assert_legacy_app_canonical kid "weekday and weekend overrides"
[ "$(echo "$payload" | jsonfilter -e '@.apps.rev')" != "$rev1" ] && pass "screen time change gives a new revision" || fail "revision unchanged"
uci set familywarden.kid.screen_minutes='120'
uci commit familywarden

[ ! -f /etc/familywarden/bonus ] || cp /etc/familywarden/bonus /tmp/bonus-before-lock-probe
hold_state_lock
call bonus '{"profile":"kid","minutes":1}' >/tmp/queued-bonus.json &
QUEUED_PID=$!
sleep 1
[ ! -s /tmp/queued-bonus.json ] && pass "bonus mutation waits for the state transaction lock" || fail "bonus changed while another transaction held the lock"
release_state_lock
wait "$QUEUED_PID"
assert_json "queued bonus completes after lock release" "$(cat /tmp/queued-bonus.json)" '@.status' "ok"
if [ -f /tmp/bonus-before-lock-probe ]; then mv /tmp/bonus-before-lock-probe /etc/familywarden/bonus; else rm -f /etc/familywarden/bonus; fi
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

# Compiler failures must not create partial pact snapshots or erase frozen rules.
break_app_compiler() {
    mv /usr/sbin/familywardend /usr/sbin/familywardend.real
    cat > /usr/sbin/familywardend <<'EOF'
#!/bin/sh
[ "$1" != app-canon ] || exit 1
exec /usr/sbin/familywardend.real "$@"
EOF
    chmod 755 /usr/sbin/familywardend
}
restore_app_compiler() { mv /usr/sbin/familywardend.real /usr/sbin/familywardend; }
break_app_compiler
assert_json "pact refuses a failed app snapshot compiler" "$(call pact '{"until":0}')" '@.error' "app policy compilation failed"
[ ! -f /etc/familywarden/pact ] && pass "failed snapshot cannot activate a pact" || fail "pact activated with missing app rules"
restore_app_compiler

# Captain: screen time and always-allowed apps freeze with the pact; no self-bonus.
call pact "{\"until\":0,\"window_day\":\"$tomorrow\",\"window_start\":\"10:00\",\"window_stop\":\"11:00\"}" >/dev/null
assert_json "no bonus for a captain under a pact" "$(call bonus '{"profile":"kid","minutes":15}')" '@.error' "bound by Ulysses pact"
cp /etc/familywarden/core/devices.json /tmp/registry-before-queued-move
hold_state_lock
call device_update '{"device":"fw-d1","profile":"parent"}' >/tmp/queued-device-move.json &
move_pid=$!
sleep 1
[ ! -s /tmp/queued-device-move.json ] && pass "core identity mutation waits for state gate" || fail "identity mutation bypassed state gate"
cmp -s /etc/familywarden/core/devices.json /tmp/registry-before-queued-move && pass "queued identity mutation does not write early" || fail "identity changed before state gate opened"
release_state_lock
wait "$move_pid"
assert_json "queued identity mutation checks the locked pact" "$(cat /tmp/queued-device-move.json)" '@.error' "bound by Ulysses pact"
uci set familywarden.kid.screen_minutes='999'
uci set familywarden.kid.screen_weekday='999'
uci del_list familywarden.kid.always_allowed='com.example.school'
uci add_list familywarden.kid.always_allowed='com.example.games'
uci commit familywarden
"$BIN" reload >/dev/null
payload=$(v3payload phoneD fw-d1 "$N1" -)
assert_json "pact: screen time frozen" "$payload" '@.apps.screen.wed' "60"
assert_json "pact: always allowed frozen" "$payload" '@.apps.always_allowed[0]' "com.example.school"
assert_legacy_app_canonical kid "frozen app snapshot"
uci set familywarden.migration_disabled=profile
uci set familywarden.migration_disabled.name='Alex tablet'
uci set familywarden.migration_disabled.enabled=0
uci set familywarden.migration_disabled.dns_client_id=fw-preserved
anonymous_schedule=$(uci add familywarden schedule)
uci set "familywarden.$anonymous_schedule.profile=migration_disabled"
uci set "familywarden.$anonymous_schedule.days=mon"
uci set "familywarden.$anonymous_schedule.start=12:00"
uci set "familywarden.$anonymous_schedule.stop=13:00"
uci commit familywarden
configuration=$(/usr/sbin/familywardend -socket "$CORE_SOCK" api configuration_export <<EOF
{}
EOF
)
assert_json "migration reader does not claim configuration ownership" "$configuration" '@.owner' legacy
assert_json "configuration snapshot has an explicit version" "$configuration" '@.configuration.version' 1
assert_json "configuration export includes disabled profiles" "$configuration" '@.configuration.current.profiles[@.id="migration_disabled"].enabled' 0
assert_json "configuration export preserves existing ClientIDs" "$configuration" '@.configuration.current.profiles[@.id="migration_disabled"].client_id' fw-preserved
assert_json "configuration export preserves anonymous section identity" "$configuration" '@.configuration.current.schedules[@.profile="migration_disabled"].id' "$anonymous_schedule"
assert_json "configuration export preserves live edits" "$configuration" '@.configuration.current.profiles[@.id="kid"].screen_weekday' 999
assert_json "configuration export preserves frozen rules separately" "$configuration" '@.configuration.pact.frozen.profiles[@.id="kid"].screen_weekday' 60
assert_json "configuration export preserves frozen captain" "$configuration" '@.configuration.pact.captain' kid
config_rev=$(echo "$configuration" | jsonfilter -e '@.configuration.revision')
[ "${#config_rev}" = 64 ] && pass "configuration revision seals the complete snapshot" || fail "missing configuration revision"
echo $(( $(date +%s) + 900 )) > /etc/familywarden/release
configuration=$(/usr/sbin/familywardend -socket "$CORE_SOCK" api configuration_export <<EOF
{}
EOF
)
[ "$(echo "$configuration" | jsonfilter -e '@.configuration.revision')" != "$config_rev" ] && pass "release deadline changes configuration revision" || fail "release deadline omitted from revision"
[ ! -f /etc/familywarden/core/configuration.json ] && pass "migration reader creates no parallel configuration store" || fail "unexpected configuration writer"
mv /etc/familywarden/pact.snap /tmp/configuration-snapshot.keep
configuration=$(/usr/sbin/familywardend -socket "$CORE_SOCK" api configuration_export <<EOF
{}
EOF
)
assert_json "incomplete frozen state is not exported as editable configuration" "$configuration" '@.error' "configuration unavailable"
mv /tmp/configuration-snapshot.keep /etc/familywarden/pact.snap
cp /etc/familywarden/pact /tmp/pact-original
cp /etc/familywarden/pact.snap /tmp/pact-snapshot-before
cp /var/run/familywarden/familywarden /tmp/pact-effective-before
cp /var/etc/familywarden.include /tmp/pact-include-before
# Force an amendment window on this disposable fixture to exercise replacement.
sed -i "s/^window_day=.*/window_day=$(date +%a | tr A-Z a-z)/; s/^window_start=.*/window_start=00:00/; s/^window_stop=.*/window_stop=24:00/" /etc/familywarden/pact
break_app_compiler
assert_json "failed amendment compiler is reported" "$(call reload)" '@.error' "effective policy compilation failed"
cmp -s /etc/familywarden/pact.snap /tmp/pact-snapshot-before && pass "failed amendment preserves the frozen snapshot" || fail "snapshot changed after compiler failure"
cmp -s /var/run/familywarden/familywarden /tmp/pact-effective-before && pass "failed amendment preserves effective policy" || fail "effective policy changed after compiler failure"
cmp -s /var/etc/familywarden.include /tmp/pact-include-before && pass "failed amendment preserves the firewall include" || fail "include changed after compiler failure"
restore_app_compiler
mv /tmp/pact-original /etc/familywarden/pact
assert_json "reload recovers after compiler repair" "$(call reload)" '@.status' "ok"
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
assert_json "administrator inventory numbered paging goes through rpcd" "$(call admin_installed '{"profile":"kid","search":"","filter":"all","page":0}')" '@.page_size' "20"
assert_json "administrator inventory rejects invalid rule filter" "$(call admin_installed '{"profile":"kid","filter":"invalid"}')" '@.error' "malformed request"
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
# A previously used grant ledger is not optional after it has been persisted.
stop_core
cp /var/etc/familywarden.include /tmp/ledger-loss-include.before
mv /etc/familywarden/core/requests.json /tmp/ledger-loss-requests.keep
/usr/sbin/familywardend -data /etc/familywarden/core -socket "$CORE_SOCK" >>/tmp/core.log 2>&1 &
CORE_PID=$!
sleep 1
assert_json "lost grant ledger prevents startup before native recovery" "$(/usr/sbin/familywardend -socket "$CORE_SOCK" status)" '@.running' false
stop_core
cmp -s /var/etc/familywarden.include /tmp/ledger-loss-include.before && pass "lost grant ledger leaves native rules unchanged" || fail "ledger loss changed native rules"
mv /tmp/ledger-loss-requests.keep /etc/familywarden/core/requests.json
cp /etc/familywarden/core/requests.json /tmp/ledger-loss-requests.before
start_core
cmp -s /etc/familywarden/core/requests.json /tmp/ledger-loss-requests.before && pass "exact grant ledger repair does not reset decisions or counters" || fail "ledger recovery rewrote decisions"
echo "=== Core: one-time import of UCI phones, restart, core down ==="
stop_core
mv /etc/familywarden/core/devices.json /tmp/devices.keep
# This stopped disposable fixture simulates pre-import identity, not recovery
# of a lost current registry. Move its presence witness out with the data.
mv /etc/familywarden/core/devices.json.present /tmp/devices.keep.present
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
echo "=== Opt-in core configuration authority / CAS / recovery ==="
stop_core
mv /etc/familywarden/core /tmp/legacy-core-tested
rm -f /etc/familywarden/pact /etc/familywarden/pact.snap /etc/familywarden/pact.dns /etc/familywarden/release
cat > /etc/config/familywarden <<'EOF'
config global 'settings'
 option enabled '1'
 option default_policy 'block'
 option ulysses_profile 'alex'
 option ulysses_delay '24'
config profile 'alex'
 option name 'Alex'
 option enabled '1'
 option policy 'scheduled'
 option apps '1'
 option screen_minutes '10'
 list device 'aa:bb:cc:00:00:01'
config profile 'taylor'
 option name 'Taylor'
 option enabled '0'
 option policy 'allow'
 option dns_client_id 'fw-preserved'
config schedule 'alex_week'
 option profile 'alex'
 option days 'mon tue wed thu fri'
 option start '07:00'
 option stop '20:00'
EOF
"$BIN" reload >/dev/null
start_core
call pact "{\"until\":$(( $(date +%s) + 3600 ))}" >/dev/null
uci set familywarden.alex.screen_minutes=999
uci set familywarden.alex_week.stop=23:00
uci commit familywarden
"$BIN" reload >/dev/null
"$BIN" sail-request >/dev/null
# Import while an amendment window is open, without promoting the loose UCI
# edits into the original frozen snapshot. A clock window alone is not consent.
owned_day=$(date +%a | tr A-Z a-z)
sed -i "s/^type=.*/type=ongoing/; s/^until=.*/until=0/; s/^window_day=.*/window_day=$owned_day/; s/^window_start=.*/window_start=00:00/; s/^window_stop=.*/window_stop=24:00/" /etc/familywarden/pact
cfg_api() { printf '%s' "${2:-{\}}" | /usr/sbin/familywardend -socket "$CORE_SOCK" api "$1"; }
exported=$(cfg_api configuration_export)
revision=$(echo "$exported" | jsonfilter -e '@.configuration.revision')
release_before=$(cat /etc/familywarden/release)
assert_json "explicit core import succeeds" "$(cfg_api configuration_import "{\"revision\":\"$revision\"}")" '@.status' ok
assert_json "configuration ownership is now core" "$(cfg_api configuration_export)" '@.owner' core
assert_json "legacy UCI editor can detect core ownership" "$(call configuration_status)" '@.owner' core
assert_json "import preserves configured app edits" "$(cfg_api configuration_export)" '@.configuration.current.profiles[@.id="alex"].screen_minutes' 999
assert_json "import preserves frozen app rules" "$(cfg_api configuration_export)" '@.configuration.pact.frozen.profiles[@.id="alex"].screen_minutes' 10
assert_json "import preserves the release deadline" "$(cfg_api configuration_export)" '@.configuration.pact.release_at' "$release_before"
assert_json "import keeps disabled profiles and ClientIDs" "$(cfg_api configuration_export)" '@.configuration.current.profiles[@.id="taylor"].client_id' fw-preserved
usign -G -p /tmp/owned-phone.pub -s /tmp/owned-phone.key >/dev/null
v2payload owned-phone fw-owned "$N1" >/dev/null
assert_json "owned mode approves a bound phone" "$(call device_approve "{\"device\":\"fw-owned\",\"profile\":\"alex\",\"fingerprint\":\"$(fpr owned-phone)\"}")" '@.status' ok
assert_json "owned policy still uses the frozen limit" "$(v3payload owned-phone fw-owned "$N1" - '' apps-v1,screen-time-v1)" '@.apps.screen.mon' 10
# Even a stale/tampered runtime projection cannot become the app-policy source.
uci -c /var/run/familywarden set familywarden.alex.screen_minutes=777
uci -c /var/run/familywarden commit familywarden
assert_json "owned app policy ignores a divergent runtime projection" "$(v3payload owned-phone fw-owned "$N1" - '' apps-v1,screen-time-v1)" '@.apps.screen.mon' 10
call reload >/dev/null
assert_json "owned binding cannot escape its pact" "$(call device_update '{"device":"fw-owned","profile":"taylor"}')" '@.error' 'bound by Ulysses pact'
# Phone pact/rules now come from the core document; the shell supplies only the
# runtime override. Both must agree on a real router (the core logs any divergence).
owned_reply=$(v3payload owned-phone fw-owned "$N1" - '' apps-v1,screen-time-v1)
assert_json "owned phone sees the active pact" "$owned_reply" '@.pact.active' true
assert_json "owned phone sees the release deadline" "$owned_reply" '@.pact.release_at' "$release_before"
assert_json "owned phone gets frozen windows, not live edits" "$owned_reply" '@.rules.windows[0].stop' '20:00'
call block '{"profile":"alex"}' >/dev/null
assert_json "owned phone keeps the runtime override" "$(v3payload owned-phone fw-owned "$N1" - '' apps-v1,screen-time-v1)" '@.rules.override' block
call reset '{"profile":"alex"}' >/dev/null
grep -q 'platform projection differs' /tmp/core.log && fail "core and shell phone policy diverged" || pass "core and shell phone policy agree"
# A complete configuration edit uses the read revision and requires confirmation.
exported=$(cfg_api configuration_export)
revision=$(echo "$exported" | jsonfilter -e '@.configuration.revision')
current=$(echo "$exported" | jsonfilter -e '@.configuration.current' | sed 's/"Taylor"/"Robin"/')
applied=$(cfg_api configuration_apply "{\"revision\":\"$revision\",\"current\":$current}")
assert_json "core edit requires connection confirmation" "$applied" '@.status' confirmation_required
assert_json "candidate is the actual policy/profile reader" "$(call profiles)" '@.profiles[@.id="taylor"].name' Robin
assert_json "stale editor cannot overwrite a pending change" "$(cfg_api configuration_apply "{\"revision\":\"$revision\",\"current\":$current}")" '@.error' 'confirmation pending'
new_revision=$(echo "$applied" | jsonfilter -e '@.revision'); confirm_token=$(echo "$applied" | jsonfilter -e '@.token')
assert_json "wrong confirmation is refused" "$(cfg_api configuration_confirm "{\"revision\":\"$new_revision\",\"token\":\"wrong\"}")" '@.error' 'configuration conflict'
assert_json "matching confirmation commits" "$(cfg_api configuration_confirm "{\"revision\":\"$new_revision\",\"token\":\"$confirm_token\"}")" '@.status' ok
assert_json "unrelated edit inside the window keeps the imported frozen app limit" "$(v3payload owned-phone fw-owned "$N1" - '' apps-v1,screen-time-v1)" '@.apps.screen.mon' 10
assert_json "unrelated edit inside the window keeps the imported frozen schedule" "$(v3payload owned-phone fw-owned "$N1" - '' apps-v1,screen-time-v1)" '@.rules.windows[0].stop' '20:00'
assert_json "stale revision is refused after confirmation" "$(cfg_api configuration_apply "{\"revision\":\"$revision\",\"current\":$current}")" '@.error' 'configuration conflict'
# Direct legacy UCI changes must not be re-imported or become policy authority.
uci set familywarden.taylor.name='Legacy edit'
uci commit familywarden
assert_json "reimport never overwrites core edits" "$(cfg_api configuration_import '{}')" '@.already_imported' true
call reload >/dev/null
assert_json "legacy reload restores the core projection" "$(call profiles)" '@.profiles[@.id="taylor"].name' Robin
# A crash/restart during confirmation restores the durable prior state, including
# its active pact. Removing the RAM projection also exercises boot reconstruction.
exported=$(cfg_api configuration_export); revision=$(echo "$exported" | jsonfilter -e '@.configuration.revision')
current=$(echo "$exported" | jsonfilter -e '@.configuration.current' | sed 's/"Robin"/"Jordan"/')
assert_json "second edit is pending" "$(cfg_api configuration_apply "{\"revision\":\"$revision\",\"current\":$current}")" '@.status' confirmation_required
# The candidate's revision does not cover the previous configuration. A
# schema-valid but corrupted rollback snapshot must never be recovered.
cp /etc/familywarden/core/configuration.json /tmp/owned-pending.keep
ucode -l fs -e 'let d=json(fs.readfile("/etc/familywarden/core/configuration.json")); d.pending.before.current.settings.default_policy="allow"; print(sprintf("%J",d));' > /tmp/owned-pending.corrupt
cp /tmp/owned-pending.corrupt /etc/familywarden/core/configuration.json
assert_json "corrupt rollback revision refuses reads before recovery" "$(cfg_api configuration_export)" '@.error' 'invalid household revision'
cmp -s /tmp/owned-pending.corrupt /etc/familywarden/core/configuration.json && pass "corrupt rollback refusal leaves the journal untouched" || fail "invalid journal was rewritten"
mv /tmp/owned-pending.keep /etc/familywarden/core/configuration.json
stop_core
rm -f /var/run/familywarden/familywarden
start_core
assert_json "restart rolls back an unconfirmed edit" "$(call profiles)" '@.profiles[@.id="taylor"].name' Robin
assert_json "restart preserves the active pact and release" "$(cfg_api configuration_export)" '@.configuration.pact.release_at' "$release_before"
cp /etc/familywarden/core/configuration.json /tmp/owned-configuration.keep
sed -i 's/"version":1/"version":99/' /etc/familywarden/core/configuration.json
[ -n "$(cfg_api configuration_export | jsonfilter -e '@.error')" ] && pass "future core configuration refuses reads" || fail "future schema fell back to UCI"
mast2 owned-phone fw-owned "$N1" | head -1 | grep -q '^Status: 500' && pass "future configuration never signs legacy fallback" || fail "future configuration produced a policy"
mv /tmp/owned-configuration.keep /etc/familywarden/core/configuration.json
assert_json "restored valid owner remains readable" "$(cfg_api configuration_export)" '@.owner' core
# Losing the household journal must not reactivate permissive legacy writers.
assert_json "activation persists a separate authority floor" "$(cat /etc/familywarden/core/authority.json)" '@.owner' core
assert_json "native adapter advertises authority protection" "$("$BIN" core-configuration-facts)" '@.authority_format' authority-v2
# Existing r18-format v1 documents have no floor. Adopting one must not rewrite
# identity or change the household/pact revision.
upgrade_revision=$(cfg_api configuration_export | jsonfilter -e '@.configuration.revision')
cp /etc/familywarden/core/devices.json /tmp/ownership-devices.before
stop_core
rm /etc/familywarden/core/authority.json
# The historical source predates per-file witnesses too; remove those only in
# this stopped, disposable fixture, never as a production recovery procedure.
find /etc/familywarden/core -name '*.present' -exec rm -f {} \;
start_core
assert_json "unmarked v1 household gains the authority floor on upgrade" "$(cat /etc/familywarden/core/authority.json)" '@.owner' core
assert_json "ownership adoption preserves configuration revision" "$(cfg_api configuration_export)" '@.configuration.revision' "$upgrade_revision"
assert_json "ownership adoption preserves active pact release" "$(cfg_api configuration_export)" '@.configuration.pact.release_at' "$release_before"
cmp -s /tmp/ownership-devices.before /etc/familywarden/core/devices.json && pass "ownership adoption preserves device keys and scopes byte-for-byte" || fail "ownership adoption changed identity"
assert_json "owned raw legacy snapshot is unavailable" "$("$BIN" core-configuration configuration-v1)" '@.error' 'owned configuration requires core API'
assert_json "owned legacy context cannot supply policy" "$("$BIN" core-context go-apps-v3)" '@.error' 'owned context requires runtime facts'
assert_json "owned legacy phone policy is refused" "$("$BIN" core-policy alex 3 go-apps-v3)" '@.error' 'owned policy requires core authority'
assert_json "owned legacy registry cannot be reimported" "$("$BIN" core-legacy-devices)" '@.error' 'owned registry requires explicit recovery'
assert_json "owned legacy registry cleanup is refused" "$("$BIN" core-legacy-devices-remove)" '@.error' 'owned registry requires explicit recovery'
cp /var/etc/familywarden.include /tmp/ownership-include.before
mv /etc/familywarden/core/configuration.json /tmp/ownership-configuration.keep
assert_json "lost configuration refuses export instead of UCI fallback" "$(cfg_api configuration_export)" '@.error' 'previously written document unavailable: configuration.json'
assert_json "authority floor keeps allow on the core path" "$(call allow '{"profile":"taylor"}')" '@.error' 'previously written document unavailable: configuration.json'
assert_json "authority floor keeps reload on the core path" "$(call reload)" '@.error' 'previously written document unavailable: configuration.json'
assert_json "lost configuration cannot be reimported from a projection" "$(cfg_api configuration_import '{}')" '@.error' 'previously written document unavailable: configuration.json'
assert_json "authenticated sail cannot release a missing owned journal" "$(sail '{"action":"release","password":"sirens","answer":"press on"}')" '@.error' 'previously written document unavailable: configuration.json'
stop_core
assert_json "core-down allow cannot bypass the authority floor" "$(call allow '{"profile":"taylor"}')" '@.error' 'core not reachable'
mv /etc/familywarden/core/authority.json /tmp/ownership-floor.keep
assert_json "ownership witnesses prevent fallback when both JSON documents are lost" "$(call allow '{"profile":"taylor"}')" '@.error' 'core not reachable'
assert_json "ownership witnesses protect authenticated sail routing" "$(sail '{"action":"cancel","password":"sirens"}')" '@.error' 'core not reachable'
mv /tmp/ownership-floor.keep /etc/familywarden/core/authority.json
assert_json "core-down raw snapshot cannot bypass the authority floor" "$("$BIN" core-configuration configuration-v1)" '@.error' 'owned configuration requires core API'
assert_json "core-down legacy ticket remains forbidden with missing configuration" "$("$BIN" core-ticket taylor "$(( $(date +%s) + 60 ))")" '@.error' 'owned ticket requires the core API'
assert_json "authenticated sail cannot cancel through core-down legacy fallback" "$(sail '{"action":"cancel","password":"sirens"}')" '@.error' 'core not reachable'
[ -n "$("$BIN" sail-status | jsonfilter -e '@.error')" ] && pass "protected recovery cannot fall back with a missing journal" || fail "legacy sail status bypassed the floor"
/usr/sbin/familywardend -data /etc/familywarden/core -socket "$CORE_SOCK" >>/tmp/core.log 2>&1 &
CORE_PID=$!
sleep 1
assert_json "restart with a missing journal serves no policy" "$(/usr/sbin/familywardend -socket "$CORE_SOCK" status)" '@.running' false
stop_core
cmp -s /var/etc/familywarden.include /tmp/ownership-include.before && pass "lost journal never replaces the last native rules" || fail "legacy fallback changed native rules"
mv /tmp/ownership-configuration.keep /etc/familywarden/core/configuration.json
start_core
assert_json "exact journal repair preserves the active pact" "$(cfg_api configuration_export)" '@.configuration.pact.release_at' "$release_before"
# A deliberately confirmed amendment may now activate configured values. The
# kernel accepts 24:00 boundaries, but signed phone windows must use LocalTime.
exported=$(cfg_api configuration_export)
revision=$(echo "$exported" | jsonfilter -e '@.configuration.revision')
current=$(echo "$exported" | ucode -l fs -e 'let d=json(fs.stdin.read("all")); for(let w in d.configuration.current.schedules) if(w.profile=="alex") w.stop="24:00"; print(sprintf("%J",d.configuration.current));')
assert_json "explicit midnight amendment commits" "$(cfg_api configuration_amend "{\"revision\":\"$revision\",\"current\":$current}")" '@.status' ok
assert_json "phone receives a parseable midnight endpoint" "$(v3payload owned-phone fw-owned "$N1" - '' apps-v1,screen-time-v1)" '@.rules.windows[0].stop' '00:00'
assert_rule "amended midnight schedule reaches the kernel" 'aa:bb:cc:00:00:01 meta hour >= "07:00" meta day .* accept'
exported=$(cfg_api configuration_export)
revision=$(echo "$exported" | jsonfilter -e '@.configuration.revision')
invalid=$(echo "$exported" | ucode -l fs -e 'let d=json(fs.stdin.read("all")); d.configuration.current.downtime=[{id:"invalid_clock",profile:"alex",days:"mon",start:"21:00",stop:"24:00"}]; print(sprintf("%J",d.configuration.current));')
assert_json "app downtime cannot be accepted then silently discarded" "$(cfg_api configuration_amend "{\"revision\":\"$revision\",\"current\":$invalid}")" '@.error' 'invalid app downtime'
assert_json "rejected app downtime leaves revision unchanged" "$(cfg_api configuration_export)" '@.configuration.revision' "$revision"

echo ""
if [ "$FAILED" -eq 0 ]; then
    echo "All integration checks passed!"
else
    echo "Some integration checks FAILED"
fi
exit "$FAILED"
