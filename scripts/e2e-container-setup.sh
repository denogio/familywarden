#!/bin/sh
# Runs INSIDE the OpenWrt rootfs container for scripts/test-e2e.sh: installs
# the apks from /apk, a test config, and starts LuCI by hand (no procd).
set -e

mkdir -p /var/lock /var/run/ubus /var/etc
apk update >/dev/null 2>&1 || true
apk add uhttpd-mod-ucode qrencode >/dev/null 2>&1
apk add --allow-untrusted /apk/familywarden-[0-9]*.apk /apk/luci-app-familywarden-[0-9]*.apk >/dev/null
printf '%s\n%s\n' "$PASSWORD" "$PASSWORD" | passwd root >/dev/null 2>&1

# procd normally provides the "system" ubus object that LuCI's header needs
cat > /usr/libexec/rpcd/system <<'EOF'
#!/bin/sh
case "$1" in
list) echo '{"board":{},"info":{}}' ;;
call) case "$2" in
    board) echo '{"hostname":"e2e","model":"OpenWrt container","board_name":"container","kernel":"6","system":"x86","release":{"distribution":"OpenWrt","version":"25.12.4","revision":"r0","target":"x86/64","description":"OpenWrt 25.12.4"}}' ;;
    info) echo "{\"uptime\":100,\"localtime\":$(date +%s),\"load\":[0,0,0],\"memory\":{\"total\":1,\"free\":1,\"shared\":0,\"buffered\":0,\"available\":1,\"cached\":0},\"swap\":{\"total\":0,\"free\":0}}" ;;
    esac ;;
esac
EOF
chmod +x /usr/libexec/rpcd/system

# A LAN like a real router's, and leases for the "devices in no profile" notice
touch /etc/config/network
uci set network.lan=interface
uci set network.lan.device='br-lan'
uci commit network
now=$(date +%s)
cat > /tmp/dhcp.leases <<EOF
$((now + 3600)) aa:bb:cc:00:00:01 192.168.1.101 dennis-phone *
$((now + 3600)) aa:bb:cc:00:00:02 192.168.1.102 alex-pc *
$((now + 3600)) de:ad:be:ef:00:99 192.168.1.150 new-laptop *
EOF

# Dennis is the Ulysses captain, allowed around the clock so a pact made by
# the tests never locks the test browser out of LuCI
cat > /etc/config/familywarden <<'EOF'
config global 'settings'
	option enabled '1'
	option default_policy 'allow'
	option ulysses_profile 'dennis'
	option ulysses_delay '1'

config profile 'dennis'
	option name 'Dennis'
	option enabled '1'
	option policy 'scheduled'
	option block_router '1'
	list device 'aa:bb:cc:00:00:01'

config schedule
	option profile 'dennis'
	option days 'mon tue wed thu fri sat sun'
	option start '00:00'
	option stop '23:59'

config profile 'alex'
	option name 'Alex'
	option enabled '1'
	option policy 'allow'
	list device 'aa:bb:cc:00:00:02'
EOF

# The browser reaches the container through docker's bridge (no fw4 zone)
uci set firewall.@defaults[0].input='ACCEPT'
uci commit firewall
fw4 start >/dev/null 2>&1

ubusd >/dev/null 2>&1 &
sleep 1
rpcd >/dev/null 2>&1 &
sleep 2
uhttpd -f -p 0.0.0.0:80 -h /www -x /cgi-bin -u /ubus -t 60 \
    -o /cgi-bin/luci -O /usr/share/ucode/luci/uhttpd.uc >/dev/null 2>&1 &
sleep 1
familywarden reload >/dev/null
