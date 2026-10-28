#!/bin/sh
# Historical backend-source -> current APK migration in disposable OpenWrt.
# This exercises tagged legacy code, not historical APK maintainer scripts.
# Usage: sh scripts/test-upgrade.sh [current-apk-dir]

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
mkdir -p /usr/libexec/rpcd /etc/familywarden /var/lock /var/etc /var/run/familywarden-core
cp /legacy/backend "$BIN"
chmod 755 "$BIN"
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
uci add firewall include >/dev/null
uci set firewall.@include[-1].fw4_compatible=1
uci set firewall.@include[-1].type=nftables
uci set firewall.@include[-1].path=/var/etc/familywarden.include
uci commit firewall
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
until=$(( $(date +%s) + 7200 ))
check 'tagged legacy backend creates pact' "$(printf '{"until":%s}' "$until" | "$BIN" call pact)" '@.status' ok
check 'tagged legacy backend binds the captain' "$(printf '{"profile":"jordan"}' | "$BIN" call status)" '@.captain' true
sha256sum /etc/familywarden/mast.key /etc/familywarden/mast.pub /etc/familywarden/agh.auth /etc/familywarden/agh.owned /etc/familywarden/dns.ack >/tmp/native-secrets.before
cp /etc/familywarden/pact /tmp/pact.before
cp /etc/familywarden/pact.dns /tmp/pact-dns.before

apk update >/dev/null 2>&1
apk add --allow-untrusted /apk/familywarden-core-*.apk /apk/familywarden-[0-9]*.apk /apk/luci-app-familywarden-*.apk >/tmp/upgrade-apk.log 2>&1 || { tail -20 /tmp/upgrade-apk.log; exit 1; }
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
enc() { printf '%s' "$1" | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g'; }
nonce=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
printf 'familywarden-sync-v2\nfw-historical\n%s\n' "$nonce" >/tmp/sync.message
usign -S -s /tmp/phone.key -m /tmp/sync.message -x /tmp/sync.sig
QUERY_STRING="device=fw-historical&nonce=$nonce&key=$(enc "$pub")&sig=$(enc "$(tail -1 /tmp/sync.sig)")" \
    REQUEST_METHOD=GET sh /www-familywarden/cgi-bin/mast | sed '1,/^$/d' >/tmp/sync.reply
{ echo 'untrusted comment: fictional migration fixture'; head -1 /tmp/sync.reply; } >/tmp/router.sig
tail -n +2 /tmp/sync.reply | tr -d '\n' >/tmp/router.payload
usign -V -p /etc/familywarden/mast.pub -m /tmp/router.payload -x /tmp/router.sig && pass 'v2 handset verifies the original router signature' || fail 'handset signature changed'
phone=$(cat /tmp/router.payload)
check 'v2 handset retains approved enrollment' "$phone" '@.enrollment' approved
check 'v2 handset retains profile binding' "$phone" '@.binding.profile_id' jordan
check 'v2 handset retains active pact' "$phone" '@.pact.active' true
check 'v2 handset retains strict DNS hostname' "$phone" '@.dns.hostname' jordan-test.dns.example.test
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
if [ "$FAILED" = 0 ]; then
    echo "Historical source migration passed: $LEGACY_REF"
else
    tail -12 /tmp/upgrade-core.log
    echo "Last export: $exported"
fi
exit "$FAILED"
