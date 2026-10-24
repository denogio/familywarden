#!/bin/sh
set -eu
# Run only after test-integration.sh in its disposable OpenWrt container.
[ "${FW_TEST_FIXTURES:-}" = 1 ] && [ -f /tmp/phoneD.key ] || {
    echo "requires FW_TEST_FIXTURES=1 and integration-generated phoneD key" >&2
    exit 1
}
BIN=/usr/libexec/rpcd/familywarden
nonce=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
key=$(sed -n 2p /tmp/phoneD.pub)
uci set familywarden.fixture=profile
uci set familywarden.fixture.name='Alex'
uci set familywarden.fixture.enabled=1
uci set familywarden.fixture.policy=allow
uci set familywarden.fixture.apps=1
for row in 'f_video com.roblox.client 45 0' 'f_block com.zhiliaoapp.musically - 1' 'f_audio com.spotify.music - 0'; do
    set -- $row
    uci set "familywarden.$1=app"
    uci set "familywarden.$1.profile=fixture"
    uci set "familywarden.$1.package=$2"
    [ "$3" = - ] || uci set "familywarden.$1.minutes=$3"
    uci set "familywarden.$1.blocked=$4"
done
uci set familywarden.f_down=app_downtime
uci set familywarden.f_down.profile=fixture
uci set familywarden.f_down.days='mon fri'
uci set familywarden.f_down.start='21:30'
uci set familywarden.f_down.stop='07:00'
uci set familywarden.dev_fw_fixture=device
uci set familywarden.dev_fw_fixture.id='fw-fixture'
uci set familywarden.dev_fw_fixture.name='Example phone'
uci set familywarden.dev_fw_fixture.profile=fixture
uci set familywarden.dev_fw_fixture.administrator=0
uci set "familywarden.dev_fw_fixture.pubkey=$key"
uci commit familywarden
"$BIN" reload >/dev/null
printf 'familywarden-sync-v2\nfw-fixture\n%s\n' "$nonce" > /tmp/f-2.msg
printf 'familywarden-sync-v3\nfw-fixture\n%s\n-\n' "$nonce" > /tmp/f-3.msg
printf 'familywarden-sync-caps-v1\nfw-fixture\n%s\n-\napps-v1,screen-time-v1\n' "$nonce" > /tmp/f-c.msg
for i in 2 3 c; do usign -S -m "/tmp/f-$i.msg" -s /tmp/phoneD.key -x "/tmp/f-$i.sig"; done
"$BIN" mast2 fw-fixture "$nonce" "$key" "$(sed -n 2p /tmp/f-2.sig)" Fixture "$(sed -n 2p /tmp/f-3.sig)" - apps-v1,screen-time-v1 "$(sed -n 2p /tmp/f-c.sig)" > /tmp/f-body
sed -n 1p /tmp/f-body > /tmp/v3-payload.sig
printf '%s' "$(sed 1d /tmp/f-body)" > /tmp/v3-payload.json
sed -n 2p /etc/familywarden/mast.pub > /tmp/v3-router.pub
"$BIN" mast2 fw-fixture "$nonce" "$key" "$(sed -n 2p /tmp/f-2.sig)" Fixture "$(sed -n 2p /tmp/f-3.sig)" - '' '' > /tmp/f-refused
sed -n 1p /tmp/f-refused > /tmp/v3-refused.sig
printf '%s' "$(sed 1d /tmp/f-refused)" > /tmp/v3-refused.json
