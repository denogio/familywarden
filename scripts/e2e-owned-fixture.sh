#!/bin/sh
# Explicit cutover in a fresh disposable e2e container; never a public import RPC.
set -eu
[ "${FW_E2E_FIXTURE:-}" = 1 ] || exit 1
api() { printf '%s' "${2:-{\}}" | familywardend api "$1"; }
case "${1:-}" in
import)
    # The second phase gets a fresh container, not the legacy suite's active pact.
    [ ! -e /etc/familywarden/pact ]
    [ "$(api configuration_status | jsonfilter -e '@.owner')" = legacy ]
    uci set familywarden.settings.dns_domain=dns.example.net
    uci set familywarden.settings.agh_url=http://192.0.2.1:3000
    uci commit familywarden
    familywarden reload >/dev/null
    revision=$(api configuration_export | jsonfilter -e '@.configuration.revision')
    test -n "$revision"
    api configuration_import "{\"revision\":\"$revision\"}" | grep -q '"status":"ok"'
    ;;
restart)
    test -s /etc/familywarden/core/configuration.json
    killall familywardend
    (/usr/sbin/familywardend -data /etc/familywarden/core >/tmp/core.log 2>&1 &)
    for _ in $(seq 1 20); do
        [ "$(familywardend status | jsonfilter -e '@.running')" = true ] && exit 0
        sleep 1
    done
    exit 1
    ;;
*) exit 1 ;;
esac
