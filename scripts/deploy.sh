#!/bin/bash
# Deploy FamilyWarden to OpenWrt (cat-based, no sftp required)
# Usage: ./scripts/deploy.sh [root@]<host>

set -e

HOST="${1:-root@192.168.1.1}"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"

echo "=== FamilyWarden Deploy ==="
echo "Target: $HOST"
echo ""

# Test connection
echo "Testing connection..."
ssh $SSH_OPTS $HOST "uname -a" >/dev/null 2>&1 || {
    echo "❌ Cannot connect to $HOST"
    exit 1
}
echo "✓ Connected"
echo ""

# Function to copy file via ssh cat
ssh_cat_put() {
    local src="$1"
    local dst="$2"
    cat "$src" | ssh $SSH_OPTS $HOST "cat > '$dst'"
}

# Create directories
echo "Creating directories..."
ssh $SSH_OPTS $HOST "
    mkdir -p /etc/config /etc/init.d /usr/bin /usr/lib/rpcd /usr/share/rpcd/acl.d
"

# Copy files
echo "Copying files..."
ssh_cat_put familywarden/usr/bin/familywarden.sh /usr/bin/familywarden
ssh_cat_put familywarden/etc/init.d/familywarden /etc/init.d/familywarden
ssh_cat_put familywarden/usr/lib/rpcd/familywarden /usr/lib/rpcd/familywarden
ssh_cat_put familywarden/usr/share/rpcd/acl.d/familywarden.json /usr/share/rpcd/acl.d/familywarden.json

# Make executable
echo "Setting permissions..."
ssh $SSH_OPTS $HOST "
    chmod +x /usr/bin/familywarden /etc/init.d/familywarden /usr/lib/rpcd/familywarden
"

# Create config
echo "Creating config..."
ssh $SSH_OPTS $HOST "
    if [ ! -f /etc/config/familywarden ]; then
        cat > /etc/config/familywarden << 'EOFCONFIG'
config global 'settings'
    option enabled '1'
    option default_policy 'allow'

config profile 'test'
    option name 'Test Profile'
    option enabled '1'
    option policy 'allow'
    list device 'aa:bb:cc:dd:ee:ff'

config schedule 'test_schedule'
    option profile 'test'
    option days 'mon tue wed thu fri'
    option start '09:00'
    option stop '21:00'
    option action 'allow'
EOFCONFIG
    fi
"

# Restart services
echo "Restarting services..."
ssh $SSH_OPTS $HOST "
    /etc/init.d/rpcd restart 2>/dev/null || true
    /etc/init.d/familywarden enable 2>/dev/null || true
    /etc/init.d/familywarden restart
    sleep 1
"

echo ""
echo "✓ FamilyWarden daemon deployed!"
echo ""
echo "=== Test ==="
ssh $SSH_OPTS $HOST "ubus list | grep familywarden && ubus call familywarden list"