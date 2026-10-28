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

ssh_put() {
    ssh $SSH_OPTS "$HOST" "cat > '$2'" < "$1"
}

echo "=== FamilyWarden deploy → $HOST ==="
ssh $SSH_OPTS "$HOST" "uname -a" >/dev/null || { echo "❌ Cannot connect to $HOST"; exit 1; }

ssh $SSH_OPTS "$HOST" "mkdir -p /etc/init.d /usr/libexec/rpcd /usr/share/rpcd/acl.d \
    /www/luci-static/resources/view/familywarden /www/luci-static/resources/familywarden/shared \
    /usr/share/luci/menu.d /usr/sbin"

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
ssh_put luci-app-familywarden/htdocs/luci-static/resources/familywarden/control-host.js \
    /www/luci-static/resources/familywarden/control-host.js
for f in web/*.js web/*.css; do
    ssh_put "$f" "/www/luci-static/resources/familywarden/shared/$(basename "$f")"
done
ssh_put luci-app-familywarden/root/usr/share/luci/menu.d/luci-app-familywarden.json \
    /usr/share/luci/menu.d/luci-app-familywarden.json
ssh_put luci-app-familywarden/root/usr/share/rpcd/acl.d/luci-app-familywarden.json \
    /usr/share/rpcd/acl.d/luci-app-familywarden.json

echo "Activating..."
# Mirrors the familywarden package postinst (firewall include) + luci cache flush
ssh $SSH_OPTS "$HOST" "
    chmod +x /etc/init.d/familywarden /usr/libexec/rpcd/familywarden
    ln -sf ../libexec/rpcd/familywarden /usr/sbin/familywarden
    # Remove legacy dashboard files from earlier deploys; only the shared workspace remains.
    rm -f /www/luci-static/resources/view/familywarden/overview.js \
          /www/luci-static/resources/view/familywarden/profiles.js \
          /www/luci-static/resources/view/familywarden/settings.js \
          /www/luci-static/resources/view/familywarden/devices.js \
          /www/luci-static/resources/view/familywarden/info.js \
          /www/luci-static/resources/view/familywarden/family.js \
          /www/luci-static/resources/familywarden/ui.js \
          /www/luci-static/resources/familywarden/ulysses.js \
          /www/luci-static/resources/familywarden/configuration.js \
          /www/luci-static/resources/familywarden/overview.css \
          /www/luci-static/resources/familywarden/pages.css
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
