#!/bin/bash
# Deploy LuCI UI (cat-based)
set -e

HOST="${1:-root@192.168.1.1}"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"

ssh_cat_put() {
    cat "$1" | ssh $SSH_OPTS $HOST "cat > '$2'"
}

echo "=== LuCI Deploy ==="

# Create directories
ssh $SSH_OPTS $HOST "
    mkdir -p /usr/lib/lua/luci/controller/familywarden
    mkdir -p /usr/lib/lua/luci/model/cbi/familywarden
    mkdir -p /usr/lib/lua/luci/view/familywarden
    mkdir -p /usr/lib/lua/luci/i18n
"

# Copy files
echo "Copying controller..."
ssh_cat_put luci-app-familywarden/luasrc/controller/familywarden.lua \
    /usr/lib/lua/luci/controller/familywarden.lua

echo "Copying models..."
for f in luci-app-familywarden/luasrc/model/cbi/familywarden/*.lua; do
    ssh_cat_put "$f" "/usr/lib/lua/luci/model/cbi/familywarden/$(basename "$f")"
done

echo "Copying views..."
for f in luci-app-familywarden/luasrc/view/familywarden/*.htm; do
    ssh_cat_put "$f" "/usr/lib/lua/luci/view/familywarden/$(basename "$f")"
done

echo "Copying i18n..."
ssh_cat_put luci-app-familywarden/luasrc/i18n/familywarden.da.lua \
    /usr/lib/lua/luci/i18n/familywarden.da.lua

echo "Copying CSS..."
ssh_cat_put luci-app-familywarden/htdocs/luci-static/familywarden.css \
    /www/luci-static/resources/familywarden.css

# Restart web
echo "Restarting web..."
ssh $SSH_OPTS $HOST "/etc/init.d/uhttpd restart"

echo ""
echo "✓ LuCI deployed!"
echo "Web UI: http://192.168.1.1/luci/ → Services → FamilyWarden"
