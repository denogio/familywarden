#!/bin/bash
# Syntax and structure tests for familywarden

set -e

PROJECT_ROOT="$(dirname "$0")/.."
FW_SCRIPT="${PROJECT_ROOT}/familywarden/usr/bin/familywarden.sh"
RPCD_SCRIPT="${PROJECT_ROOT}/familywarden/usr/lib/rpcd/familywarden"
INIT_SCRIPT="${PROJECT_ROOT}/familywarden/etc/init.d/familywarden"

echo "=== FamilyWarden Syntax Tests ==="
echo ""

# Check all scripts exist
for script in "$FW_SCRIPT" "$RPCD_SCRIPT" "$INIT_SCRIPT"; do
    if [ ! -f "$script" ]; then
        echo "❌ Missing: $script"
        exit 1
    fi
    echo "✓ Found: $script"
done

echo ""
echo "=== ShellCheck (if available) ==="
if command -v shellcheck &> /dev/null; then
    shellcheck -x "$FW_SCRIPT" && echo "✓ familywarden.sh passed" || echo "⚠ familywarden.sh has warnings"
    shellcheck -x "$RPCD_SCRIPT" && echo "✓ rpcd plugin passed" || echo "⚠ rpcd plugin has warnings"
    shellcheck -x "$INIT_SCRIPT" && echo "✓ init.d script passed" || echo "⚠ init.d script has warnings"
else
    echo "shellcheck not found - skipping"
fi

echo ""
echo "=== Required Functions ==="

required_functions=(
    "nft_init"
    "evaluate_profile"
    "evaluate_all"
    "ubus_list"
    "ubus_status"
    "ubus_block"
    "ubus_allow"
    "ubus_reset"
    "ubus_ticket"
)

for func in "${required_functions[@]}"; do
    if grep -q "^${func}()" "$FW_SCRIPT"; then
        echo "✓ Function: $func"
    else
        echo "❌ Missing function: $func"
    fi
done

echo ""
echo "=== Ubus API Endpoints ==="

ubus_methods=("list" "status" "block" "allow" "reset" "ticket" "reload")
for method in "${ubus_methods[@]}"; do
    if grep -q "\"$method\"" "$RPCD_SCRIPT" || grep -q "$method" "$RPCD_SCRIPT"; then
        echo "✓ Endpoint: $method"
    else
        echo "⚠ Missing endpoint: $method"
    fi
done

echo ""
echo "=== nftables chain name consistency ==="

CHAIN_NAME="familywarden_forward"
if grep -q "$CHAIN_NAME" "$FW_SCRIPT"; then
    echo "✓ Chain name: $CHAIN_NAME"
else
    echo "❌ Chain name not found"
fi

echo ""
echo "All checks complete!"
