#!/bin/sh
# Historical -> current APK migration in disposable OpenWrt. Tagged backend
# sources always run; LEGACY_RELEASE_DIR=<dir with a released familywarden and
# LuCI .apk> also installs that real package (its maintainer scripts included)
# and upgrades it with apk.
# Usage: [LEGACY_RELEASE_DIR=dir] sh scripts/test-upgrade.sh [current-apk-dir]

set -eu
if [ "${1:-}" != --inside ]; then
    REPO=$(cd "$(dirname "$0")/.." && pwd)
    APKS=$(cd "${1:-$REPO/dist/r7c}" && pwd)
    TMP=$(mktemp -d)
    trap 'rm -rf "$TMP"' EXIT
    for ref in v0.9.1 v0.9.2; do
        mkdir "$TMP/$ref"
        git -C "$REPO" show "$ref:familywarden/usr/lib/rpcd/familywarden" > "$TMP/$ref/backend"
        docker run --rm --memory=768m --cpus=2 --cap-add NET_ADMIN \
            -e FW_TEST_UPGRADE_DISPOSABLE=1 -e LEGACY_REF="$ref" \
            -v "$REPO:/src:ro" -v "$APKS:/apk:ro" -v "$TMP/$ref:/legacy:ro" \
            "${OPENWRT_IMAGE:-openwrt/rootfs:x86-64-25.12.4}" \
            sh /src/scripts/test-upgrade.sh --inside
    done
    for release in ${LEGACY_RELEASE_DIR:-}; do # space-separated package directories
        RELEASE=$(cd "$release" && pwd)
        ref=release-$(ls "$RELEASE"/familywarden-[0-9]*.apk | sed 's|.*/familywarden-||; s|\.apk$||')
        docker run --rm --memory=768m --cpus=2 --cap-add NET_ADMIN \
            -e FW_TEST_UPGRADE_DISPOSABLE=1 -e LEGACY_REF="$ref" \
            -v "$REPO:/src:ro" -v "$APKS:/apk:ro" -v "$RELEASE:/legacy-release:ro" \
            "${OPENWRT_IMAGE:-openwrt/rootfs:x86-64-25.12.4}" \
            sh /src/scripts/test-upgrade.sh --inside
    done
    exit
fi
[ "${FW_TEST_UPGRADE_DISPOSABLE:-}" = 1 ] && [ -f /.dockerenv ] || exit 1
FAILED=0
pass() { echo "✓ $LEGACY_REF: $1"; }
fail() { echo "FAILED $LEGACY_REF: $1"; FAILED=1; }
check() {
    actual=$(printf '%s' "$2" | jsonfilter -e "$3") || actual=""
    [ "$actual" = "$4" ] && pass "$1" || fail "$1 (expected $4; got $actual)"
}
BIN=/usr/libexec/rpcd/familywarden
mkdir -p /usr/libexec/rpcd /var/lock /var/etc /var/run/familywarden-core
packaged=0
if [ -d /legacy-release ]; then
    packaged=1
    apk update >/dev/null 2>&1
    apk add --allow-untrusted /legacy-release/*.apk >/tmp/legacy-apk.log 2>&1 || { tail -20 /tmp/legacy-apk.log; exit 1; }
    [ -x "$BIN" ] && pass 'released legacy package installed with its maintainer scripts' || fail 'released backend missing'
else
    cp /legacy/backend "$BIN"
    chmod 755 "$BIN"
fi
mkdir -p /etc/familywarden
cat >/etc/config/familywarden <<'EOF'
config global 'settings'
    option enabled '1'
    option default_policy 'block'
    option ulysses_profile 'jordan'
    option ulysses_delay '1'
    option dns_domain 'dns.example.test'
    option agh_url 'http://127.0.0.1:1'
config profile 'jordan'
    option name 'Jordan'
    option enabled '1'
    option policy 'scheduled'
    option block_router '1'
    list device 'aa:bb:cc:00:00:02'
    option dns '1'
    option dns_client_id 'jordan-test'
    list dns_protection 'safe_browsing'
config schedule 'jordan_night'
    option profile 'jordan'
    option days 'mon tue wed thu fri sat sun'
    option start '22:00'
    option stop '06:00'
config profile 'alex'
    option name 'Alex'
    option enabled '1'
    option policy 'block'
    list device 'aa:bb:cc:00:00:01'
EOF
touch /etc/config/network
uci set network.lan=interface
uci set network.lan.device=br-lan
uci set network.lan.proto=static
uci commit network
if [ "$packaged" = 0 ]; then # the released postinst adds this itself
    uci add firewall include >/dev/null
    uci set firewall.@include[-1].fw4_compatible=1
    uci set firewall.@include[-1].type=nftables
    uci set firewall.@include[-1].path=/var/etc/familywarden.include
    uci commit firewall
fi
usign -G -s /tmp/phone.key -p /tmp/phone.pub -c fictional-phone
usign -G -s /etc/familywarden/mast.key -p /etc/familywarden/mast.pub -c fictional-router
pub=$(tail -1 /tmp/phone.pub)
uci set familywarden.phone=device
uci set familywarden.phone.id=fw-historical
uci set familywarden.phone.name='Test phone'
uci set familywarden.phone.profile=jordan
uci set "familywarden.phone.pubkey=$pub"
uci set familywarden.phone.administrator=1
uci commit familywarden
printf '%s\n' 'fixture:fixture-secret' >/etc/familywarden/agh.auth
printf '%s\n' jordan-test >/etc/familywarden/agh.owned
printf '%s\n' 'jordan jordan-test dns.example.test safe_browsing -' >/etc/familywarden/dns.ack
chmod 600 /etc/familywarden/mast.* /etc/familywarden/agh.* /etc/familywarden/dns.ack
fw4 start >/dev/null 2>&1
"$BIN" reload >/dev/null
# A released core (rc-era pilots) owned identity before any pact, as on the
# pilot router: import first, then create the pact with that core running.
# (rc4's core imported no captain phones when first started during a pact.)
OLD=""
if [ -x /usr/sbin/familywardend ]; then
    mkdir -p /var/run/familywarden-core
    /usr/sbin/familywardend >>/tmp/legacy-core.log 2>&1 &
    OLD=$!
    for _ in 1 2 3 4 5 6 7 8 9 10; do /usr/sbin/familywardend status 2>/dev/null | grep -q '"running":true' && break; sleep 1; done
    check 'released core imported the enrolled phone' "$(cat /etc/familywarden/core/devices.json 2>/dev/null)" '@.devices[0].pubkey' "$pub"
fi
until=$(( $(date +%s) + 7200 ))
check 'tagged legacy backend creates pact' "$(printf '{"until":%s}' "$until" | "$BIN" call pact)" '@.status' ok
check 'tagged legacy backend binds the captain' "$(printf '{"profile":"jordan"}' | "$BIN" call status)" '@.captain' true
[ -z "$OLD" ] || { kill "$OLD"; wait "$OLD" || true; }
sha256sum /etc/familywarden/mast.key /etc/familywarden/mast.pub /etc/familywarden/agh.auth /etc/familywarden/agh.owned /etc/familywarden/dns.ack >/tmp/native-secrets.before
cp /etc/familywarden/pact /tmp/pact.before
cp /etc/familywarden/pact.dns /tmp/pact-dns.before

apk update >/dev/null 2>&1
apk add --allow-untrusted /apk/familywarden-core-*.apk /apk/familywarden-[0-9]*.apk /apk/luci-app-familywarden-*.apk >/tmp/upgrade-apk.log 2>&1 || { tail -20 /tmp/upgrade-apk.log; exit 1; }
[ "$(uci show firewall | grep -c "path='/var/etc/familywarden.include'")" = 1 ] && pass 'upgrade keeps exactly one firewall include' || fail 'firewall include duplicated or lost'
if [ "$packaged" = 1 ]; then
    expected=$(basename /apk/familywarden-[0-9]*.apk .apk)
    installed=$(apk list -I familywarden 2>/dev/null | sed 's/ .*//')
    [ "$installed" = "$expected" ] && pass 'apk replaced the released package' || fail "released package not upgraded (installed $installed, expected $expected)"
    [ -n "$(uci -q get uhttpd.familywarden)" ] && pass 'upgrade keeps the protected-release page instance' || fail 'sail page instance lost'
fi
/usr/sbin/familywardend >>/tmp/upgrade-core.log 2>&1 &
PID=$!
trap 'kill "$PID" 2>/dev/null || true' EXIT
for _ in 1 2 3 4 5 6 7 8 9 10; do
    /usr/sbin/familywardend status | grep -q '"running":true' && break
    sleep 1
done
api() { printf '%s' "${2:-{\}}" | /usr/sbin/familywardend api "$1"; }
check 'new daemon starts after tagged state' "$(/usr/sbin/familywardend status)" '@.running' true
check 'legacy public key imported exactly' "$(cat /etc/familywarden/core/devices.json)" '@.devices[0].pubkey' "$pub"
check 'administrator retained' "$(api devices)" '@.devices[0].administrator' 1
exported=$(api configuration_export)
check 'upgrade alone does not activate ownership' "$exported" '@.owner' legacy
revision=$(printf '%s' "$exported" | jsonfilter -e '@.configuration.revision')
check 'explicit import succeeds' "$(api configuration_import "{\"revision\":\"$revision\"}")" '@.status' ok
exported=$(api configuration_export)
check 'pact captain preserved' "$exported" '@.configuration.pact.captain' jordan
# An old v2 handset must still trust the original router key and binding.
# The pin is copied off the router: later checks model the handset's own trust.
cp /etc/familywarden/mast.pub /tmp/pinned.pub
enc() { printf '%s' "$1" | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g'; }
# The handset keeps its last accepted household generation (HouseholdGeneration.kt).
phone_epoch=""
phone_counter=0
phone_follows() {
    epoch=$(printf '%s' "$phone" | jsonfilter -e '@.generation.epoch') || epoch=""
    counter=$(printf '%s' "$phone" | jsonfilter -e '@.generation.counter') || counter=""
    [ -z "$phone_epoch" ] && return 0
    [ -n "$epoch" ] || return 1
    [ "$epoch" = "$phone_epoch" ] && { [ "$counter" -ge "$phone_counter" ]; return; }
    fork=$(printf '%s' "$phone" | jsonfilter -e "@.generation.lineage[@.epoch='$phone_epoch'].counter") || fork=""
    [ -n "$fork" ] && [ "$fork" -ge "$phone_counter" ]
}
phone_fetch() {
    nonce=$(head -c 32 /dev/urandom | sha256sum | cut -c1-64)
    printf 'familywarden-sync-v2\nfw-historical\n%s\n' "$nonce" >/tmp/sync.message
    usign -S -s /tmp/phone.key -m /tmp/sync.message -x /tmp/sync.sig
    QUERY_STRING="device=fw-historical&nonce=$nonce&key=$(enc "$pub")&sig=$(enc "$(tail -1 /tmp/sync.sig)")" \
        REQUEST_METHOD=GET sh /www-familywarden/cgi-bin/mast | sed '1,/^$/d' >/tmp/sync.reply
    { echo 'untrusted comment: fictional migration fixture'; head -1 /tmp/sync.reply; } >/tmp/router.sig
    tail -n +2 /tmp/sync.reply | tr -d '\n' >/tmp/router.payload
    usign -V -q -p /tmp/pinned.pub -m /tmp/router.payload -x /tmp/router.sig && pass "$1: v2 handset verifies its pinned router" || fail "$1: handset signature changed"
    phone=$(cat /tmp/router.payload)
}
phone_sync() {
    phone_fetch "$1"
    phone_follows && pass "$1: household generation continues the handset's state" || fail "$1: handset would refuse the generation"
    phone_epoch=$epoch
    phone_counter=$counter
    check "$1: v2 handset retains approved enrollment" "$phone" '@.enrollment' approved
    check "$1: v2 handset retains profile binding" "$phone" '@.binding.profile_id' jordan
    check "$1: v2 handset retains active pact" "$phone" '@.pact.active' true
    check "$1: v2 handset retains strict DNS hostname" "$phone" '@.dns.hostname' jordan-test.dns.example.test
}
phone_sync migration
check 'pact deadline preserved' "$exported" '@.configuration.pact.until' "$until"
check 'frozen default deny preserved' "$exported" '@.configuration.pact.default_policy' block
check 'frozen cooling-off preserved' "$exported" '@.configuration.pact.delay_seconds' 3600
check 'ClientID preserved' "$exported" '@.configuration.current.profiles[@.id="jordan"].client_id' jordan-test
check 'authority floor created' "$(cat /etc/familywarden/core/authority.json)" '@.owner' core
sha256sum -c /tmp/native-secrets.before >/dev/null && pass 'router keys and native DNS secrets/acknowledgements preserved' || fail 'native secrets changed'
uci set familywarden.jordan.policy=allow
uci commit familywarden
check 'reimport does not replace the owned pact' "$(api configuration_import '{}')" '@.already_imported' true
check 'captain allow remains refused' "$(api configuration_allow '{"profile":"jordan"}')" '@.error' 'bound by Ulysses pact'
kill "$PID"; wait "$PID" || true
/usr/sbin/familywardend >>/tmp/upgrade-core.log 2>&1 &
PID=$!
for _ in 1 2 3 4 5 6 7 8 9 10; do
    /usr/sbin/familywardend status | grep -q '"running":true' && break
    sleep 1
done
check 'restart keeps imported key' "$(cat /etc/familywarden/core/devices.json)" '@.devices[0].pubkey' "$pub"
check 'restart keeps pact deadline' "$(api configuration_export)" '@.configuration.pact.until' "$until"
# Offline backup must not stop a running daemon or repair/expire household state.
/usr/sbin/familywardend backup /tmp/household.tar.gz >/tmp/live-backup.out 2>/tmp/live-backup.err \
    && fail 'live backup was accepted' || pass 'backup refuses the running writer'
[ ! -e /tmp/household.tar.gz ] && pass 'refused live backup publishes no archive' || fail 'live backup artifact exists'
kill "$PID"; wait "$PID" || true
sha256sum /etc/familywarden/core/configuration.json /etc/familywarden/core/devices.json /etc/config/familywarden >/tmp/backup-source.before
/usr/sbin/familywardend backup /tmp/household.tar.gz >/tmp/backup-summary.json
check 'offline backup created a signed format' "$(cat /tmp/backup-summary.json)" '@.format' familywarden-backup-v1
check 'backup creation is not restore authorization' "$(cat /tmp/backup-summary.json)" '@.restore_authorized' false
[ "$(ls -l /tmp/household.tar.gz | cut -c1-10)" = '-rw-------' ] && pass 'backup archive is private' || fail 'backup permissions'
tar -tzf /tmp/household.tar.gz >/tmp/backup.names
for name in household/mast.key household/agh.auth household/core/configuration.json household/core/authority.json \
    household/core/devices.json household/pact platform-config manifest.json manifest.sig; do
    grep -qx "$name" /tmp/backup.names || fail "backup lacks $name"
done
grep -q 'writer.lock' /tmp/backup.names && fail 'backup captured the lifetime lock' || pass 'backup contains keys, journals, pact and UCI but not the writer lock'
/usr/sbin/familywardend -trusted-pub /etc/familywarden/mast.pub backup-inspect /tmp/household.tar.gz >/tmp/inspect-summary.json
check 'external router pin verifies archive' "$(cat /tmp/inspect-summary.json)" '@.status' ok
check 'inspection does not authorize restore' "$(cat /tmp/inspect-summary.json)" '@.restore_authorized' false
sha256sum -c /tmp/backup-source.before >/dev/null && pass 'backup leaves journal identity and UCI unchanged' || fail 'backup mutated source'
sha256sum /tmp/household.tar.gz >/tmp/backup-file.before
/usr/sbin/familywardend backup /tmp/household.tar.gz >/dev/null 2>&1 && fail 'backup silently overwrote output' || pass 'existing backup is never overwritten'
sha256sum -c /tmp/backup-file.before >/dev/null && pass 'no-clobber preserves archive bytes' || fail 'backup changed an existing archive'
cp /tmp/household.tar.gz /tmp/changed-backup.tar.gz
printf x >>/tmp/changed-backup.tar.gz
/usr/sbin/familywardend -trusted-pub /etc/familywarden/mast.pub backup-inspect /tmp/changed-backup.tar.gz >/dev/null 2>&1 \
    && fail 'inspection accepted appended archive bytes' || pass 'inspection refuses appended archive bytes'
/usr/sbin/familywardend restore /tmp/household.tar.gz >/dev/null 2>&1 \
    && fail 'restore without an external pin ran' || pass 'restore requires an external pin'
/usr/sbin/familywardend -trusted-pub /tmp/pinned.pub restore /tmp/household.tar.gz >/tmp/overlay-restore.err 2>&1 \
    && fail 'restore overlaid an existing household' || { grep -q 'not cleared' /tmp/overlay-restore.err && pass 'restore never overlays an existing household' || fail "overlay refused for another reason: $(tail -1 /tmp/overlay-restore.err)"; }
sha256sum -c /tmp/backup-source.before >/dev/null && pass 'refused overlay leaves the household unchanged' || fail 'refused overlay changed state'

# Router replacement: a cleared router with only a fresh, unpaired identity.
rm -rf /etc/familywarden /tmp/familywarden.state
printf "config global 'settings'\n    option enabled '1'\n" >/etc/config/familywarden
mkdir -p /etc/familywarden
usign -G -s /etc/familywarden/mast.key -p /etc/familywarden/mast.pub -c fictional-replacement
/usr/sbin/familywardend >>/tmp/upgrade-core.log 2>&1 &
PID=$!
for _ in 1 2 3 4 5 6 7 8 9 10; do /usr/sbin/familywardend status | grep -q '"running":true' && break; sleep 1; done
/usr/sbin/familywardend -trusted-pub /tmp/pinned.pub restore /tmp/household.tar.gz >/dev/null 2>&1 \
    && fail 'restore ran beside a live daemon' || pass 'restore refuses a running daemon'
kill "$PID"; wait "$PID" || true
/usr/sbin/familywardend -trusted-pub /etc/familywarden/mast.pub restore /tmp/household.tar.gz >/dev/null 2>&1 \
    && fail 'restore trusted the replacement router key' || pass 'restore refuses the local unpinned key'
/usr/sbin/familywardend -trusted-pub /tmp/pinned.pub restore /tmp/changed-backup.tar.gz >/dev/null 2>&1 \
    && fail 'restore accepted a changed archive' || pass 'restore refuses a changed archive'
[ ! -e /etc/familywarden/core/configuration.json ] && pass 'refused restores activate nothing' || fail 'refused restore activated state'
/usr/sbin/familywardend -trusted-pub /tmp/pinned.pub restore /tmp/household.tar.gz >/tmp/restore.json
check 'pinned restore admitted onto the cleared router' "$(cat /tmp/restore.json)" '@.status' restored
previous=$(jsonfilter -i /tmp/restore.json -e '@.previous')
[ -f "$previous/mast.key" ] && pass 'cleared identity kept aside, not deleted' || fail 'previous household missing'
cmp -s /tmp/pinned.pub /etc/familywarden/mast.pub && pass 'original router identity restored' || fail 'restored identity differs'
/usr/sbin/familywardend >>/tmp/upgrade-core.log 2>&1 &
PID=$!
for _ in 1 2 3 4 5 6 7 8 9 10; do /usr/sbin/familywardend status | grep -q '"running":true' && break; sleep 1; done
check 'restored daemon starts' "$(/usr/sbin/familywardend status)" '@.running' true
exported=$(api configuration_export)
check 'restored household is core-owned' "$exported" '@.owner' core
check 'restored pact deadline' "$exported" '@.configuration.pact.until' "$until"
check 'restored default deny' "$exported" '@.configuration.pact.default_policy' block
[ "$(uci -q get familywarden.jordan.name)" = Jordan ] && pass 'startup materialized the UCI projection' || fail 'UCI projection missing'
nft list chain inet fw4 familywarden_forward 2>/dev/null | grep -q 'aa:bb:cc:00:00:02' && pass 'restored policy reached the kernel' || fail 'restored rules missing from nftables'
phone_sync restore
check 'restored captain allow remains refused' "$(api configuration_allow '{"profile":"jordan"}')" '@.error' 'bound by Ulysses pact'
kill "$PID"; wait "$PID" || true
/usr/sbin/familywardend -trusted-pub /tmp/pinned.pub restore /tmp/household.tar.gz >/dev/null 2>/tmp/second-restore.err \
    && fail 'second restore overlaid the restored household' || { grep -q 'not cleared' /tmp/second-restore.err && pass 'second restore is refused' || fail "second restore refused for another reason: $(tail -1 /tmp/second-restore.err)"; }

# Older-backup rollback: the handset saw later state, then an older backup is
# restored. Its fork point is too low however many router writes follow.
start_core() {
    /usr/sbin/familywardend >>/tmp/upgrade-core.log 2>&1 &
    PID=$!
    for _ in 1 2 3 4 5 6 7 8 9 10; do /usr/sbin/familywardend status | grep -q '"running":true' && break; sleep 1; done
}
clear_router() {
    rm -rf /etc/familywarden /etc/familywarden.pre-restore-* /tmp/familywarden.state
    printf "config global 'settings'\n    option enabled '1'\n" >/etc/config/familywarden
    mkdir -p /etc/familywarden
    usign -G -s /etc/familywarden/mast.key -p /etc/familywarden/mast.pub -c fictional-replacement
}
mv /tmp/household.tar.gz /tmp/older.tar.gz
start_core   # startup persists the journal: the generation advances
phone_sync newer
kill "$PID"; wait "$PID" || true
/usr/sbin/familywardend backup /tmp/newest.tar.gz >/dev/null
clear_router
/usr/sbin/familywardend -trusted-pub /tmp/pinned.pub restore /tmp/older.tar.gz >/dev/null
for attempt in 1 2 3; do
    start_core
    phone_fetch "older restore ($attempt)"
    phone_follows && fail "older restore ($attempt): handset would accept rolled-back state" \
        || pass "older restore ($attempt): handset keeps its newer policy"
    kill "$PID"; wait "$PID" || true
done
clear_router
/usr/sbin/familywardend -trusted-pub /tmp/pinned.pub restore /tmp/newest.tar.gz >/dev/null
start_core
phone_sync 'newest restore'
kill "$PID"; wait "$PID" || true
if [ "$FAILED" = 0 ]; then
    echo "Historical source migration passed: $LEGACY_REF"
else
    tail -12 /tmp/upgrade-core.log
    echo "Last export: $exported"
fi
exit "$FAILED"
