#!/bin/sh
# Runs INSIDE the README screenshot container after e2e-container-setup.sh.
# Fictional family only: Alex and Sam (children), Jordan (adult), a guest profile.
set -e
# The procd stand-in answers per call, so a fictional hostname needs no rpcd restart.
sed -i 's/"hostname":"e2e"/"hostname":"home-router"/' /usr/libexec/rpcd/system
now=$(date +%s)
cat > /tmp/dhcp.leases <<EOF
$((now + 3600)) aa:bb:cc:00:10:01 192.168.1.101 alex-phone *
$((now + 3600)) aa:bb:cc:00:10:02 192.168.1.102 alex-tablet *
$((now + 3600)) aa:bb:cc:00:20:01 192.168.1.111 sam-phone *
$((now + 3600)) aa:bb:cc:00:20:02 192.168.1.112 sam-console *
$((now + 3600)) aa:bb:cc:00:30:01 192.168.1.121 jordan-laptop *
$((now + 3600)) de:ad:be:ef:00:99 192.168.1.150 new-laptop *
EOF
cat > /etc/config/familywarden <<'EOF'
config global 'settings'
	option enabled '1'
	option default_policy 'allow'
	option ulysses_profile 'jordan'
	option ulysses_delay '24'

config profile 'alex'
	option name 'Alex'
	option enabled '1'
	option policy 'scheduled'
	option block_router '1'
	option apps '1'
	option apps_suspend_new '1'
	option screen_weekday '120'
	option screen_weekend '180'
	list always_allowed 'com.example.school'
	list device 'aa:bb:cc:00:10:01'
	list device 'aa:bb:cc:00:10:02'

config schedule
	option profile 'alex'
	option days 'mon tue wed thu fri'
	option start '07:00'
	option stop '20:30'

config schedule
	option profile 'alex'
	option days 'sat sun'
	option start '08:00'
	option stop '21:30'

config app
	option profile 'alex'
	option package 'com.example.video'
	option minutes '30'

config app
	option profile 'alex'
	option package 'com.example.game'
	option minutes '60'

config app
	option profile 'alex'
	option package 'com.example.chat'
	option blocked '1'

config app_downtime
	option profile 'alex'
	option days 'sun mon tue wed thu'
	option start '21:00'
	option stop '07:00'

config profile 'sam'
	option name 'Sam'
	option enabled '1'
	option policy 'scheduled'
	option apps '1'
	option screen_minutes '90'
	list device 'aa:bb:cc:00:20:01'
	list device 'aa:bb:cc:00:20:02'

config schedule
	option profile 'sam'
	option days 'mon tue wed thu fri sat sun'
	option start '07:30'
	option stop '19:30'

config profile 'jordan'
	option name 'Jordan'
	option enabled '1'
	option policy 'allow'
	list device 'aa:bb:cc:00:30:01'

config profile 'guest'
	option name 'Guest'
	option enabled '0'
	option policy 'block'
EOF
familywarden reload >/dev/null
# Sam has a 45-minute ticket, so the overview shows an active override.
echo '{"profile":"sam","minutes":45}' | /usr/libexec/rpcd/familywarden call ticket >/dev/null

# Fictional phones with disposable keys: enrolled exactly like real ones (signed sync,
# fingerprint approval through the core API). Keys stay in this container.
enroll() { # $1 device id, $2 display name, $3 profile, $4 approves
    nonce=$(head -c 32 /dev/urandom | hexdump -v -e '/1 "%02x"')
    usign -G -p "/tmp/$1.pub" -s "/tmp/$1.key"
    printf 'familywarden-sync-v2\n%s\n%s\n' "$1" "$nonce" > "/tmp/$1.msg"
    usign -S -s "/tmp/$1.key" -m "/tmp/$1.msg" -x "/tmp/$1.sig"
    key=$(sed -n 2p "/tmp/$1.pub" | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g')
    sig=$(sed -n 2p "/tmp/$1.sig" | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g')
    QUERY_STRING="device=$1&nonce=$nonce&key=$key&sig=$sig&name=Pixel" /usr/sbin/familywardend cgi-phone >/dev/null
    fp=$(echo '{}' | /usr/libexec/rpcd/familywarden call devices | jsonfilter -e "@.pending[@.id='$1'].fingerprint")
    printf '{"device":"%s","profile":"%s","name":"%s","fingerprint":"%s","approves":"%s"}' "$1" "$3" "$2" "$fp" "$4" \
        | /usr/libexec/rpcd/familywarden call device_approve >/dev/null
}
enroll fw-alex-phone "Alex's phone" alex ""
enroll fw-sam-phone "Sam's phone" sam ""
enroll fw-jordan-phone "Jordan's phone" jordan "alex sam"
# One phone still waiting for approval, to show the fingerprint check.
nonce=$(head -c 32 /dev/urandom | hexdump -v -e '/1 "%02x"')
usign -G -p /tmp/new.pub -s /tmp/new.key
printf 'familywarden-sync-v2\nfw-new-phone\n%s\n' "$nonce" > /tmp/new.msg
usign -S -s /tmp/new.key -m /tmp/new.msg -x /tmp/new.sig
key=$(sed -n 2p /tmp/new.pub | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g')
sig=$(sed -n 2p /tmp/new.sig | sed 's/+/%2B/g; s|/|%2F|g; s/=/%3D/g')
QUERY_STRING="device=fw-new-phone&nonce=$nonce&key=$key&sig=$sig&name=Pixel-8" /usr/sbin/familywardend cgi-phone >/dev/null
# Signed uploads from the fictional phones: one pending request, today's usage.
upload() { # $1 device id, $2 op, $3 exact JSON
    printf 'familywarden-%s-v1\n%s' "$2" "$3" > /tmp/up.msg
    usign -S -m /tmp/up.msg -s "/tmp/$1.key" -x /tmp/up.sig
    { sed -n 2p /tmp/up.sig; printf '%s' "$3"; } | REQUEST_METHOD=POST QUERY_STRING="op=$2" /usr/sbin/familywardend cgi-app >/dev/null
}
now=$(date +%s); day=$(date +%F)
upload fw-alex-phone request "{\"type\":\"request\",\"v\":1,\"request_id\":\"readme-request-1\",\"device\":\"fw-alex-phone\",\"profile\":\"alex\",\"target\":\"com.example.video\",\"minutes\":30,\"label\":\"Video\",\"created\":$now}"
upload fw-sam-phone request "{\"type\":\"request\",\"v\":1,\"request_id\":\"readme-request-2\",\"device\":\"fw-sam-phone\",\"profile\":\"sam\",\"target\":\"total\",\"minutes\":15,\"created\":$now}"
upload fw-alex-phone usage "{\"type\":\"usage\",\"v\":1,\"device\":\"fw-alex-phone\",\"profile\":\"alex\",\"day\":\"$day\",\"seq\":1,\"total\":4920,\"apps\":{\"com.example.video\":1800,\"com.example.game\":1500,\"com.example.school\":900,\"com.example.music\":720},\"labels\":{\"com.example.video\":\"Video\",\"com.example.game\":\"Game\",\"com.example.school\":\"School\",\"com.example.music\":\"Music\"}}"
upload fw-sam-phone usage "{\"type\":\"usage\",\"v\":1,\"device\":\"fw-sam-phone\",\"profile\":\"sam\",\"day\":\"$day\",\"seq\":1,\"total\":3060,\"apps\":{\"com.example.game\":2400,\"com.example.video\":660},\"labels\":{\"com.example.game\":\"Game\",\"com.example.video\":\"Video\"}}"
echo "README fixture ready"
