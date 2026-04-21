#!/bin/bash
# Deploy FamilyWarden to running QEMU VM

set -e

VM_IP="192.168.1.1"  # Default OpenWrt IP in user-mode networking
SSH_PORT="2222"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p ${SSH_PORT}"

# Check if VM is running
if ! ssh ${SSH_OPTS} root@localhost echo "VM is running" 2>/dev/null; then
    echo "VM not running. Start with ./scripts/qemu-test.sh first"
    exit 1
fi

echo "Deploying FamilyWarden to VM..."

# Create directories
ssh ${SSH_OPTS} root@localhost "
    mkdir -p /etc/config
    mkdir -p /etc/init.d
    mkdir -p /usr/bin
    mkdir -p /usr/lib/rpcd
    mkdir -p /usr/share/rpcd/acl.d
"

# Copy files
scp ${SSH_OPTS} familywarden/usr/bin/familywarden.sh root@localhost:/usr/bin/familywarden
scp ${SSH_OPTS} familywarden/etc/init.d/familywarden root@localhost:/etc/init.d/familywarden
scp ${SSH_OPTS} familywarden/usr/lib/rpcd/familywarden root@localhost:/usr/lib/rpcd/familywarden
scp ${SSH_OPTS} familywarden/usr/share/rpcd/acl.d/familywarden.json root@localhost:/usr/share/rpcd/acl.d/familywarden.json

# Copy config if not exists
ssh ${SSH_OPTS} root@localhost "
    if [ ! -f /etc/config/familywarden ]; then
        cat > /etc/config/familywarden << 'EOF'
config global 'settings'
    option enabled '1'
    option default_policy 'allow'

config profile 'josva'
    option name 'Josva'
    option enabled '1'
    option policy 'scheduled'
    list device '00:11:22:33:44:55'
    option dns_profile 'child'

config schedule 'josva_weekday'
    option profile 'josva'
    option days 'mon tue wed thu fri'
    option start '07:00'
    option stop '20:00'
    option action 'allow'
EOF
    fi
"

# Make executable
ssh ${SSH_OPTS} root@localhost "
    chmod +x /usr/bin/familywarden
    chmod +x /etc/init.d/familywarden
    chmod +x /usr/lib/rpcd/familywarden

    # Restart rpcd to load ACL
    /etc/init.d/rpcd restart

    # Start familywarden
    /etc/init.d/familywarden enable
    /etc/init.d/familywarden restart
"

echo ""
echo "Deployed! Test with:"
echo "  ssh ${SSH_OPTS} root@localhost 'ubus call familywarden list'"
echo "  ssh ${SSH_OPTS} root@localhost 'ubus call familywarden block {\"profile\":\"josva\"}'"
