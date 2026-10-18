#!/bin/sh
# AdGuard Home sync test: the real rpcd plugin in an OpenWrt rootfs container
# against a real, throwaway AdGuard Home container (same version as production).
# Checks FamilyWarden's ownership boundary: only "familywarden-*" clients and
# the ClientIDs it added to allowed_clients are ever changed.
#
#   ./scripts/test-agh.sh
#
# Needs docker. Nothing outside the two containers is touched.

AGH_IMAGE="${AGH_IMAGE:-adguard/adguardhome:v0.107.79}"
OPENWRT_IMAGE="${OPENWRT_IMAGE:-openwrt/rootfs:x86-64-25.12.4}"

if [ "$1" != "--inside" ]; then
    set -e
    REPO="$(cd "$(dirname "$0")/.." && pwd)"
    ID=$$
    NET="fw-agh-net-$ID"
    AGH="fw-agh-$ID"
    CONF=$(mktemp -d)
    cleanup() {
        docker rm -f "$AGH" >/dev/null 2>&1 || true
        docker network rm "$NET" >/dev/null 2>&1 || true
        # AGH writes as root inside the container
        docker run --rm -v "$CONF:/c" alpine rm -rf /c/conf /c/work >/dev/null 2>&1 || true
        rm -rf "$CONF"
    }
    trap cleanup EXIT
    mkdir -p "$CONF/conf" "$CONF/work"
    HASH=$(printf 'test-password' | docker run --rm -i httpd:2.4-alpine htpasswd -niB -C 5 familywarden | cut -d: -f2-)
    # Minimal config: an existing client and access list that must survive.
    cat > "$CONF/conf/AdGuardHome.yaml" <<EOF
http:
  address: 0.0.0.0:80
users:
  - name: familywarden
    password: $HASH
dns:
  bind_hosts:
    - 0.0.0.0
  port: 53
  upstream_dns:
    - 127.0.0.1:1
  allowed_clients:
    - 10.0.0.0/8
    - grandma
    - teen
  disallowed_clients:
    - 192.0.2.66
  blocked_hosts:
    - version.bind
    - id.server
    - hostname.bind
clients:
  persistent:
    - name: Grandma tablet
      ids:
        - grandma
      use_global_settings: true
      filtering_enabled: true
      safebrowsing_enabled: false
      parental_enabled: false
      use_global_blocked_services: true
      blocked_services:
        ids: []
    - name: Someone else
      ids:
        - hijack
      use_global_settings: true
      filtering_enabled: true
      use_global_blocked_services: true
      blocked_services:
        ids: []
schema_version: 34
EOF
    docker network create "$NET" >/dev/null
    docker run -d --name "$AGH" --network "$NET" -v "$CONF/conf:/opt/adguardhome/conf" \
        -v "$CONF/work:/opt/adguardhome/work" "$AGH_IMAGE" >/dev/null
    AGH_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$AGH")
    sleep 3
    docker logs "$AGH" 2>&1 | grep -iE 'fatal|error' | head -5 || true
    set +e
    docker run --rm --network "$NET" -e AGH_IP="$AGH_IP" -v "$REPO:/src:ro" "$OPENWRT_IMAGE" \
        sh /src/scripts/test-agh.sh --inside
    exit $?
fi

# ---- Inside the OpenWrt container ----

BIN=/usr/libexec/rpcd/familywarden
URL="http://$AGH_IP:80"
FAILED=0
pass() { echo "✓ $1"; }
fail() { echo "❌ $1"; FAILED=1; }
assert_eq() { [ "$2" = "$3" ] && pass "$1" || fail "$1 (expected '$3', got '$2')"; }
call() { echo "${2:-{\}}" | "$BIN" call "$1"; }
# AGH sends no auth challenge, so Basic auth goes in the header up front
TOKEN=ZmFtaWx5d2FyZGVuOnRlc3QtcGFzc3dvcmQ=  # familywarden:test-password
agh() { uclient-fetch -q --no-proxy -T 10 -O - --header="Authorization: Basic $TOKEN" "$URL/control/$1"; }
agh_post() { printf '%s' "$2" > /tmp/body; uclient-fetch -q --no-proxy -T 10 -O - --header="Authorization: Basic $TOKEN" --header='Content-Type: application/json' --post-file=/tmp/body "$URL/control/$1"; }
allowed() { agh access/list | jsonfilter -e '@.allowed_clients[*]' | sort | tr '\n' ' '; }
client() { agh clients | jsonfilter -e "@.clients[@.name='$1']$2"; }

mkdir -p /var/lock /var/etc /var/run /usr/libexec/rpcd
cp /src/familywarden/usr/lib/rpcd/familywarden "$BIN"
chmod 755 "$BIN"

for _ in $(seq 1 30); do agh status >/dev/null 2>&1 && break; sleep 1; done
agh status >/dev/null 2>&1 && pass "AdGuard Home API reachable with Basic auth" || { fail "AGH not reachable at $URL"; exit 1; }
grandma_before=$(agh clients | jsonfilter -e "@.clients[@.name='Grandma tablet']")
access_before=$(agh access/list)

cat > /etc/config/familywarden <<EOF
config global 'settings'
    option enabled '1'
    option dns_domain 'dns.example.net'
    option agh_url '$URL'

config profile 'alex'
    option name 'Ålex Junior'
    option enabled '1'
    option policy 'allow'
    option dns '1'
    list dns_protection 'filter_lists'
    list dns_protection 'safe_search'
    list dns_service 'youtube'

config profile 'gran'
    option name 'Grandma'
    option enabled '1'
    option policy 'allow'
    option dns '1'

config profile 'parent'
    option name 'Parent'
    option enabled '1'
    option policy 'allow'
EOF

echo "=== Not configured yet ==="
"$BIN" dns-sync
[ $? -ne 0 ] && pass "sync fails without credentials" || fail "sync succeeded without credentials"
assert_eq "status names the missing credentials" "$(call dns_status | jsonfilter -e '@.error')" "AdGuard Home credentials not set"
assert_eq "access list untouched" "$(agh access/list)" "$access_before"

echo ""
echo "=== First sync ==="
assert_eq "credentials stored write-only" "$(call dns_auth '{"username":"familywarden","password":"test-password"}' | jsonfilter -e '@.status')" "ok"
[ "$(ls -l /etc/familywarden/agh.auth | cut -c1-10)" = "-rw-------" ] && pass "credential file is root-only" || fail "agh.auth: $(ls -l /etc/familywarden/agh.auth)"
call dns_status | grep -q test-password && fail "status leaks the password" || pass "status never returns the password"
"$BIN" dns-sync && pass "sync succeeds" || fail "sync failed: $(cat /tmp/familywarden.dns.status)"
assert_eq "ClientID derived from the profile name" "$(uci -q get familywarden.alex.dns_client_id)" "aalex-junior"
assert_eq "taken ClientID gets a suffix" "$(uci -q get familywarden.gran.dns_client_id)" "grandma-2"
assert_eq "client created" "$(client familywarden-aalex-junior .ids[0])" "aalex-junior"
assert_eq "own settings, not global" "$(client familywarden-aalex-junior .use_global_settings)" "false"
assert_eq "filter lists on" "$(client familywarden-aalex-junior .filtering_enabled)" "true"
assert_eq "safe browsing off" "$(client familywarden-aalex-junior .safebrowsing_enabled)" "false"
assert_eq "parental off" "$(client familywarden-aalex-junior .parental_enabled)" "false"
assert_eq "SafeSearch on" "$(client familywarden-aalex-junior .safe_search.enabled)" "true"
assert_eq "SafeSearch covers YouTube" "$(client familywarden-aalex-junior .safe_search.youtube)" "true"
assert_eq "blocked service" "$(client familywarden-aalex-junior .blocked_services[0])" "youtube"
assert_eq "existing client untouched" "$(agh clients | jsonfilter -e "@.clients[@.name='Grandma tablet']")" "$grandma_before"
assert_eq "our ClientIDs added, the rest kept" "$(allowed)" "10.0.0.0/8 aalex-junior grandma grandma-2 teen "
assert_eq "disallowed clients untouched" "$(agh access/list | jsonfilter -e '@.disallowed_clients[*]')" "192.0.2.66"
assert_eq "blocked hosts untouched" "$(agh access/list | jsonfilter -e '@.blocked_hosts[*]' | tr '\n' ' ')" "version.bind id.server hostname.bind "
grep -q '^alex aalex-junior dns.example.net filter_lists,safe_search youtube$' /etc/familywarden/dns.ack && pass "acknowledged policy recorded" || fail "ack: $(cat /etc/familywarden/dns.ack)"
assert_eq "LuCI status: hostname" "$(call dns_status | jsonfilter -e '@.profiles.alex.hostname')" "aalex-junior.dns.example.net"
assert_eq "no error" "$(call dns_status | jsonfilter -e '@.error')" ""

echo ""
echo "=== Changes ==="
uci set familywarden.alex.name='Someone New'
uci add_list familywarden.alex.dns_protection='parental'
uci commit familywarden
"$BIN" dns-sync
assert_eq "renaming keeps the ClientID" "$(uci -q get familywarden.alex.dns_client_id)" "aalex-junior"
assert_eq "protections updated" "$(client familywarden-aalex-junior .parental_enabled)" "true"
assert_eq "still one client for the profile" "$(agh clients | jsonfilter -e '@.clients[*].name' | grep -c '^familywarden-aalex')" "1"

# A ClientID that was in allowed_clients before FamilyWarden is not ours to remove
uci set familywarden.parent.dns='1'
uci set familywarden.parent.dns_client_id='teen'
uci commit familywarden
"$BIN" dns-sync && pass "pre-existing allowed id used" || fail "sync failed: $(cat /tmp/familywarden.dns.status)"
grep -qx teen /etc/familywarden/agh.owned && fail "pre-existing id claimed as owned" || pass "pre-existing id not claimed"

echo ""
echo "=== Removal ==="
uci set familywarden.alex.dns='0'
uci set familywarden.parent.dns='0'
uci commit familywarden
"$BIN" dns-sync
[ -z "$(client familywarden-aalex-junior .name)" ] && pass "client deleted when DNS is switched off" || fail "client left behind"
[ -z "$(client familywarden-teen .name)" ] && pass "second client deleted" || fail "teen client left behind"
assert_eq "only our own ClientID removed from allowed" "$(allowed)" "10.0.0.0/8 grandma grandma-2 teen "
grep -q '^alex ' /etc/familywarden/dns.ack && fail "ack kept for disabled profile" || pass "ack dropped for disabled profile"
assert_eq "existing client still untouched" "$(agh clients | jsonfilter -e "@.clients[@.name='Grandma tablet']")" "$grandma_before"

echo ""
echo "=== Never take over someone else's client ==="
uci set familywarden.parent.dns='1'
uci set familywarden.parent.dns_client_id='hijack'
uci commit familywarden
"$BIN" dns-sync
[ $? -ne 0 ] && pass "sync reports the conflict" || fail "conflict not reported"
assert_eq "conflict message" "$(call dns_status | jsonfilter -e '@.error')" "ClientID hijack is used by another AdGuard Home client"
assert_eq "foreign client unchanged" "$(client 'Someone else' .use_global_settings)" "true"
[ -z "$(client familywarden-hijack .name)" ] && pass "no duplicate client created" || fail "duplicate created"
grep -q '^parent ' /etc/familywarden/dns.ack && fail "conflicting profile acknowledged" || pass "conflicting profile not acknowledged"
grep -q '^gran grandma-2 ' /etc/familywarden/dns.ack && pass "other profiles still synced" || fail "other profile lost: $(cat /etc/familywarden/dns.ack)"
uci set familywarden.parent.dns='0'
uci commit familywarden

echo ""
echo "=== AdGuard Home unavailable or credentials wrong ==="
cp /etc/familywarden/dns.ack /tmp/ack.before
call dns_auth '{"username":"familywarden","password":"wrong"}' >/dev/null
"$BIN" dns-sync
[ $? -ne 0 ] && pass "wrong credentials fail" || fail "wrong credentials succeeded"
assert_eq "acknowledged policy kept while AGH can't be reached" "$(cat /etc/familywarden/dns.ack)" "$(cat /tmp/ack.before)"
call dns_auth '{"username":"familywarden","password":"test-password"}' >/dev/null

echo ""
echo "=== Empty allowed list means everyone is allowed: never touched ==="
agh_post access/set '{"allowed_clients":[],"disallowed_clients":["192.0.2.66"],"blocked_hosts":["version.bind"]}' >/dev/null
uci set familywarden.alex.dns='1'
uci commit familywarden
"$BIN" dns-sync && pass "sync with an open access list" || fail "sync failed: $(cat /tmp/familywarden.dns.status)"
assert_eq "allowed list stays empty" "$(allowed)" ""
assert_eq "client still created" "$(client familywarden-aalex-junior .ids[0])" "aalex-junior"

echo ""
echo "=== Pact: DNS can only be tightened ==="
# alex is the captain of an ongoing pact (window not now), snapshot taken with
# filter lists, SafeSearch, parental and youtube (pact.dns format as written by the backend)
uci set familywarden.settings.ulysses_profile='alex'
uci commit familywarden
day=$(date -d "@$(( $(date +%s) + 86400 ))" +%a | tr 'A-Z' 'a-z')
printf 'captain=alex\ntype=ongoing\nuntil=0\nwindow_day=%s\nwindow_start=00:00\nwindow_stop=00:01\n' "$day" > /etc/familywarden/pact
printf 'dns=1\nid=aalex-junior\ndomain=dns.example.net\nprot=filter_lists\nprot=safe_search\nprot=parental\nsvc=youtube\n' > /etc/familywarden/pact.dns
# Try to loosen: switch DNS off, drop protections and the service, change the domain;
# and tighten: add Safe Browsing and another service
uci set familywarden.alex.dns='0'
uci -q delete familywarden.alex.dns_protection
uci add_list familywarden.alex.dns_protection='safe_browsing'
uci -q delete familywarden.alex.dns_service
uci add_list familywarden.alex.dns_service='tiktok'
uci set familywarden.settings.dns_domain='unfiltered.example.org'
uci commit familywarden
"$BIN" dns-sync && pass "sync during the pact" || fail "sync failed: $(cat /tmp/familywarden.dns.status)"
assert_eq "switching DNS off is ignored" "$(client familywarden-aalex-junior .ids[0])" "aalex-junior"
assert_eq "removed protection kept: SafeSearch" "$(client familywarden-aalex-junior .safe_search.enabled)" "true"
assert_eq "removed protection kept: parental" "$(client familywarden-aalex-junior .parental_enabled)" "true"
assert_eq "added protection applies now" "$(client familywarden-aalex-junior .safebrowsing_enabled)" "true"
assert_eq "services: kept and added" "$(client familywarden-aalex-junior '.blocked_services[*]' | sort | tr '\n' ' ')" "tiktok youtube "
assert_eq "DNS domain frozen" "$(call dns_status | jsonfilter -e '@.profiles.alex.hostname')" "aalex-junior.dns.example.net"
uci set familywarden.settings.agh_url=''
uci commit familywarden
"$BIN" dns-sync
grep -q '^alex ' /etc/familywarden/dns.ack && pass "clearing the AdGuard Home URL doesn't drop the policy" || fail "ack dropped"
uci set familywarden.settings.agh_url="$URL"
uci commit familywarden
rm -f /etc/familywarden/pact /etc/familywarden/pact.dns
"$BIN" dns-sync
[ -z "$(client familywarden-aalex-junior .name)" ] && pass "after the pact the loosening applies" || fail "client kept after the pact"
uci set familywarden.settings.dns_domain='dns.example.net'
uci commit familywarden

echo ""
echo "=== LuCI helpers ==="
call dns_services | jsonfilter -e '@.services[*].id' | grep -qx youtube && pass "service catalogue from AGH" || fail "no services"
call dns_services | jsonfilter -e "@.services[@.id='youtube'].name" | grep -qx YouTube && pass "service names" || fail "no service names"
touch /tmp/familywarden.dns.dirty
"$BIN" dns-tick
[ -f /tmp/familywarden.dns.dirty ] && fail "tick did not sync a dirty config" || pass "tick syncs after a config change"

echo ""
[ "$FAILED" -eq 0 ] && echo "All AdGuard Home checks passed!" || echo "Some AdGuard Home checks FAILED"
exit "$FAILED"
