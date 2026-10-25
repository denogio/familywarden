#!/bin/sh
# Disposable container only: create one genuinely signed request for the HTTP test.
# Called by family.spec.js, never against a user-configured router.
set -eu
[ "${FW_E2E_FIXTURE:-}" = 1 ] || exit 1
cd /tmp
case "${1:-}" in
create)
    uci set familywarden.http_test=profile
    uci set familywarden.http_test.name='Alex HTTP test'
    uci set familywarden.http_test.enabled=1
    uci set familywarden.http_test.policy=allow
    uci set familywarden.http_test.apps=1
    uci set familywarden.http_test.apps_suspend_new=0
    uci commit familywarden
    familywarden reload >/dev/null
    usign -G -p /tmp/http-test.pub -s /tmp/http-test.key
    nonce=$(head -c 32 /dev/urandom | hexdump -v -e '/1 "%02x"')
    printf 'familywarden-sync-v2\nfw-http-test\n%s\n' "$nonce" > /tmp/http-test.msg
    usign -S -s /tmp/http-test.key -m /tmp/http-test.msg -x /tmp/http-test.sig
    key=$(sed -n 2p /tmp/http-test.pub | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g')
    sig=$(sed -n 2p /tmp/http-test.sig | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g')
    QUERY_STRING="device=fw-http-test&nonce=$nonce&key=$key&sig=$sig" familywardend cgi-phone >/dev/null
    fp=$(echo '{}' | /usr/libexec/rpcd/familywarden call devices | jsonfilter -e "@.pending[@.id='fw-http-test'].fingerprint")
    test -n "$fp"
    printf '{"device":"fw-http-test","profile":"http_test","name":"Alex HTTP test","fingerprint":"%s"}' "$fp" |
        /usr/libexec/rpcd/familywarden call device_approve | grep -q '"status":"ok"'
    request=$(printf '{"type":"request","v":1,"request_id":"http-request-0001","device":"fw-http-test","profile":"http_test","target":"total","minutes":30,"created":%s}' "$(date +%s)")
    printf 'familywarden-request-v1\n%s' "$request" > /tmp/http-test.msg
    usign -S -s /tmp/http-test.key -m /tmp/http-test.msg -x /tmp/http-test.sig
    { sed -n 2p /tmp/http-test.sig; printf '%s' "$request"; } |
        REQUEST_METHOD=POST QUERY_STRING=op=request familywardend cgi-app | grep -q '"state":"pending"'
    ;;
cleanup)
    echo '{"device":"fw-http-test"}' | /usr/libexec/rpcd/familywarden call device_delete >/dev/null
    uci -q delete familywarden.http_test || true
    uci commit familywarden
    familywarden reload >/dev/null
    rm -f /tmp/http-test.key /tmp/http-test.pub /tmp/http-test.msg /tmp/http-test.sig
    ;;
*) exit 1 ;;
esac
