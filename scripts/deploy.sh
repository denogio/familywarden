#!/bin/bash
# Deploy FamilyWarden straight from the source tree (no apk build) for quick
# iteration. Installs to the same paths as the package Makefiles.
# For real installs use the apk from a GitHub release or scripts/build-apk-docker.sh.
#
# Usage: ./scripts/deploy.sh [root@]<host>    (default root@192.168.1.1)

set -e

HOST="${1:-root@192.168.1.1}"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
VIEWS="luci-app-familywarden/htdocs/luci-static/resources/view/familywarden"
VERSION="$(sed -n 's/^PKG_VERSION:=//p' luci-app-familywarden/Makefile)-dev"

ssh_put() {
    ssh $SSH_OPTS "$HOST" "cat > '$2'" < "$1"
}

echo "=== FamilyWarden deploy → $HOST ==="
ssh $SSH_OPTS "$HOST" "uname -a" >/dev/null || { echo "❌ Cannot connect to $HOST"; exit 1; }

ssh $SSH_OPTS "$HOST" "mkdir -p /etc/init.d /usr/libexec/rpcd /usr/share/rpcd/acl.d \
    /www/luci-static/resources/view/familywarden /usr/share/luci/menu.d"

echo "Backend..."
ssh_put familywarden/etc/init.d/familywarden /etc/init.d/familywarden
ssh_put familywarden/usr/lib/rpcd/familywarden /usr/libexec/rpcd/familywarden
ssh_put familywarden/usr/share/rpcd/acl.d/familywarden.json /usr/share/rpcd/acl.d/familywarden.json
ssh $SSH_OPTS "$HOST" "[ -f /etc/config/familywarden ] || cat > /etc/config/familywarden" \
    < familywarden/etc/config/familywarden

echo "LuCI..."
for f in "$VIEWS"/*.js; do
    ssh_put "$f" "/www/luci-static/resources/view/familywarden/$(basename "$f")"
done
ssh_put luci-app-familywarden/root/usr/share/luci/menu.d/luci-app-familywarden.json \
    /usr/share/luci/menu.d/luci-app-familywarden.json
ssh_put luci-app-familywarden/root/usr/share/rpcd/acl.d/luci-app-familywarden.json \
    /usr/share/rpcd/acl.d/luci-app-familywarden.json

echo "Activating..."
# Mirrors the familywarden package postinst (firewall include) + luci cache flush
ssh $SSH_OPTS "$HOST" "
    chmod +x /etc/init.d/familywarden /usr/libexec/rpcd/familywarden
    sed -i 's/@@VERSION@@/$VERSION/' /www/luci-static/resources/view/familywarden/info.js
    if ! uci show firewall | grep -q \"path='/var/etc/familywarden.include'\"; then
        uci add firewall include >/dev/null
        uci set firewall.@include[-1].fw4_compatible='1'
        uci set firewall.@include[-1].type='nftables'
        uci set firewall.@include[-1].path='/var/etc/familywarden.include'
        uci commit firewall
    fi
    rm -rf /tmp/luci-indexcache* /tmp/luci-modulecache
    /etc/init.d/rpcd restart
    /etc/init.d/familywarden enable
    /etc/init.d/familywarden restart
"

echo "✓ Deployed"
ssh $SSH_OPTS "$HOST" "ubus call familywarden list"
