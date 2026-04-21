#!/bin/bash
echo "=== FamilyWarden Basic Test ==="

# Test 1: Script loads without errors
echo -n "1. Script syntax... "
if bash -n familywarden/usr/bin/familywarden.sh 2>/dev/null; then
    echo "OK"
else
    echo "FAIL"
    exit 1
fi

# Test 2: Check UCI config format
echo -n "2. UCI config format... "
if grep -q "config global" familywarden/etc/config/familywarden 2>/dev/null; then
    echo "OK"
else
    echo "FAIL"
    exit 1
fi

# Test 3: Check required functions
echo -n "3. Required functions... "
REQUIRED="nft_init evaluate_profile ubus_list ubus_block ubus_allow ubus_reset ubus_ticket"
for func in $REQUIRED; do
    grep -q "^${func}()" familywarden/usr/bin/familywarden.sh || { echo "FAIL: $func missing"; exit 1; }
done
echo "OK"

# Test 4: Check nftables chain name consistency
echo -n "4. nftables chain... "
CHAIN="familywarden_forward"
if grep -q "$CHAIN" familywarden/usr/bin/familywarden.sh; then
    echo "OK"
else
    echo "FAIL"
    exit 1
fi

echo ""
echo "All tests passed!"
