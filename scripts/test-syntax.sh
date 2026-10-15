#!/bin/bash
# Syntax and structure tests for familywarden
#
# All backend logic lives in the rpcd plugin (installed as
# /usr/libexec/rpcd/familywarden). The init script only calls "$BIN reload".

PROJECT_ROOT="$(dirname "$0")/.."
RPCD_SCRIPT="${PROJECT_ROOT}/familywarden/usr/lib/rpcd/familywarden"
INIT_SCRIPT="${PROJECT_ROOT}/familywarden/etc/init.d/familywarden"
MAKEFILE="${PROJECT_ROOT}/familywarden/Makefile"

FAILED=0
fail() { echo "❌ $1"; FAILED=1; }

echo "=== FamilyWarden Syntax Tests ==="
echo ""

SAIL_CGI="${PROJECT_ROOT}/familywarden/www-familywarden/cgi-bin/sail"
MAST_CGI="${PROJECT_ROOT}/familywarden/www-familywarden/cgi-bin/mast"
for script in "$RPCD_SCRIPT" "$INIT_SCRIPT" "$SAIL_CGI" "$MAST_CGI"; do
    if [ ! -f "$script" ]; then
        fail "Missing: $script"
        continue
    fi
    echo "✓ Found: $script"
    # bash, not sh: Ubuntu's dash rejects fd>9 redirects (200>) that OpenWrt's ash
    # accepts. test-integration.sh parses + runs everything under real ash.
    bash -n "$script" && echo "✓ Parses: $script" || fail "Syntax error: $script"
done

echo ""
echo "=== ShellCheck (if available) ==="
if command -v shellcheck &> /dev/null; then
    shellcheck -x "$RPCD_SCRIPT" > /dev/null && echo "✓ rpcd plugin passed" || echo "⚠ rpcd plugin has warnings"
    shellcheck -x "$INIT_SCRIPT" > /dev/null && echo "✓ init.d script passed" || echo "⚠ init.d script has warnings"
else
    echo "shellcheck not found - skipping"
fi

echo ""
echo "=== Required Functions ==="

required_functions=(
    "generate_include"
    "compile_and_apply"
    "gc_expired_tickets"
    "_sched_accept_rules"
    "gen_input_profile_scheduled"
    "ulysses_sync"
    "gen_lockdown_rules"
    "gc_pact"
    "ubus_list"
    "ubus_status"
    "ubus_block"
    "ubus_allow"
    "ubus_reset"
    "ubus_ticket"
    "ubus_reload"
    "ubus_version"
)

for func in "${required_functions[@]}"; do
    if grep -q "^${func}()" "$RPCD_SCRIPT"; then
        echo "✓ Function: $func"
    else
        fail "Missing function: $func"
    fi
done

echo ""
echo "=== Ubus API Endpoints ==="

# Each method must be both advertised (rpcd "list" JSON) and dispatched ("call").
ubus_methods=("list" "status" "block" "allow" "reset" "ticket" "pact" "mast" "reload" "version")
ACL_FILES=("${PROJECT_ROOT}/familywarden/usr/share/rpcd/acl.d/familywarden.json"
           "${PROJECT_ROOT}/luci-app-familywarden/root/usr/share/rpcd/acl.d/luci-app-familywarden.json")
for method in "${ubus_methods[@]}"; do
    if ! grep -q "^  \"$method\":" "$RPCD_SCRIPT"; then
        fail "Not advertised in list: $method"
    elif ! grep -qE "^ +$method\)$" "$RPCD_SCRIPT"; then
        fail "Not dispatched in call: $method"
    else
        echo "✓ Endpoint: $method"
    fi
    for acl in "${ACL_FILES[@]}"; do
        grep -q "\"$method\"" "$acl" || fail "$method missing from ACL $(basename "$acl")"
    done
done

# Releasing a Ulysses pact must only be possible via the sail page (CGI on
# port 7700 calling the CLI), never as a ubus method LuCI or HA could call
if grep -qE 'sail-(request|cancel)' "${ACL_FILES[@]}" \
    || sed -n '/^    call)$/,/^esac$/p' "$RPCD_SCRIPT" | grep -q 'sail-'; then
    fail "sail-* is exposed via ubus/ACL - release must stay on the sail page"
else
    echo "✓ release only via the sail page"
fi

echo ""
echo "=== nftables chain/set consistency ==="

for name in familywarden_forward familywarden_input familywarden_lockdown familywarden_ticket; do
    # Must be declared in the include and deleted before fw4 reload
    if grep -qE "echo \"(chain|set) $name \{" "$RPCD_SCRIPT" \
        && grep -qE "nft delete (chain|set) inet fw4 $name" "$RPCD_SCRIPT"; then
        echo "✓ $name: declared + cleaned up"
    else
        fail "$name not declared or not cleaned up before fw4 reload"
    fi
done

echo ""
echo "=== LuCI views + JSON ==="

for f in "${PROJECT_ROOT}"/luci-app-familywarden/htdocs/luci-static/resources/view/familywarden/*.js \
         "${PROJECT_ROOT}"/luci-app-familywarden/htdocs/luci-static/resources/familywarden/*.js; do
    # LuCI views use a top-level `return`, so parse them as a function body
    if ! command -v node &> /dev/null; then
        echo "node not found - skipping JS parse"; break
    fi
    node -e 'new Function(require("fs").readFileSync(process.argv[1], "utf8"))' "$f" \
        && echo "✓ Parses: $(basename "$f")" || fail "JS syntax error: $f"
done

for f in "${PROJECT_ROOT}"/familywarden/usr/share/rpcd/acl.d/*.json \
         "${PROJECT_ROOT}"/luci-app-familywarden/root/usr/share/*/*/*.json; do
    python3 -m json.tool "$f" > /dev/null \
        && echo "✓ Valid JSON: $(basename "$(dirname "$f")")/$(basename "$f")" || fail "Invalid JSON: $f"
done

# Every view/module in the source tree must be installed by the luci Makefile
for f in "${PROJECT_ROOT}"/luci-app-familywarden/htdocs/luci-static/resources/view/familywarden/*.js \
         "${PROJECT_ROOT}"/luci-app-familywarden/htdocs/luci-static/resources/familywarden/*.js; do
    rel="${f#*/htdocs/luci-static/resources/}"
    grep -q "resources/$rel " "${PROJECT_ROOT}/luci-app-familywarden/Makefile" \
        || fail "Not installed by luci Makefile: $rel"
done

echo ""
echo "=== Packaging consistency ==="

grep -q 'usr/libexec/rpcd/familywarden' "$MAKEFILE" \
    && grep -q 'BIN="/usr/libexec/rpcd/familywarden"' "$INIT_SCRIPT" \
    && echo "✓ init.d BIN matches Makefile install path" \
    || fail "init.d BIN does not match Makefile install path"

for f in www-familywarden/index.html www-familywarden/cgi-bin/sail www-familywarden/cgi-bin/mast; do
    grep -q "./$f " "$MAKEFILE" && [ -f "${PROJECT_ROOT}/familywarden/$f" ] \
        && echo "✓ sail page installed: $f" || fail "sail page file not installed: $f"
done
grep -q "listen_http='0.0.0.0:7700'" "$MAKEFILE" && grep -q '^FW_SAIL_PORT=7700$' "$RPCD_SCRIPT" \
    && echo "✓ sail port matches (uhttpd ↔ lockdown)" || fail "sail port mismatch between Makefile and rpcd plugin"

grep -q "/var/etc/familywarden.include" "$MAKEFILE" \
    && grep -q 'FW_INCLUDE="/var/etc/familywarden.include"' "$RPCD_SCRIPT" \
    && echo "✓ firewall include path matches" \
    || fail "firewall include path mismatch between Makefile and rpcd plugin"

echo ""
if [ "$FAILED" -eq 0 ]; then
    echo "All checks passed!"
else
    echo "Some checks FAILED"
fi
exit "$FAILED"
