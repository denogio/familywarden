#!/bin/sh
# DISPOSABLE ANDROID LAB ONLY. This is NOT a production enrollment protocol.
# Deliberately authorizes any syntactically valid device as the test captain/admin.
# Installed explicitly by test-mast-emulator.sh, never by either OpenWrt package.
# The sentinel prevents accidental use when merely copying this file elsewhere.

if [ ! -f /tmp/familywarden-android-lab ]; then
    printf 'Status: 403 Forbidden\r\n\r\n'
    exit 1
fi

case "$QUERY_STRING" in
    pubkey*)
        printf 'Content-Type: text/plain\r\nCache-Control: no-store\r\n\r\n'
        exec /usr/libexec/rpcd/familywarden mast-pubkey
        ;;
esac

dev=$(printf '%s' "$QUERY_STRING" | sed -n 's/^.*device=\([A-Za-z0-9-]*\).*$/\1/p')
nonce=$(printf '%s' "$QUERY_STRING" | sed -n 's/^.*nonce=\([0-9a-f]*\).*$/\1/p')
if [ -z "$dev" ] || [ "${#dev}" -gt 80 ] || [ "${#nonce}" -ne 64 ]; then
    printf 'Status: 400 Bad Request\r\n\r\n'
    exit 1
fi

# Transform the real daemon's effective pact/rules snapshot into a lab-only v2
# reply, retaining real usign verification and the real application's cache path.
body=$(/usr/libexec/rpcd/familywarden mast "$dev") || exit 1
payload=$(printf '%s\n' "$body" | sed 1d)
. /usr/share/libubox/jshn.sh
json_load "$payload"
json_get_var captain captain
json_add_int v 2
json_add_string nonce "$nonce"
json_add_string timezone UTC
json_add_object binding
json_add_string device_id "$dev"
json_add_string profile_id "$captain"
json_add_string profile_name Alex
json_add_boolean captain 1
json_add_boolean administrator 1
json_add_boolean can_self_remove 1
json_close_object
# DNS is deliberately absent until the isolated strict-DNS lab is added.
tmp=$(mktemp) || exit 1
trap 'rm -f "$tmp" "$tmp.sig"' EXIT HUP INT TERM
json_dump | tr -d '\n' > "$tmp"
usign -S -m "$tmp" -s /etc/familywarden/mast.key -x "$tmp.sig" || exit 1
printf 'Content-Type: text/plain\r\nCache-Control: no-store\r\n\r\n'
sed -n 2p "$tmp.sig"
cat "$tmp"
