#!/usr/bin/env python3
"""Disposable P8 device-owner child and ordinary-parent flow. No physical adb target."""
import argparse
import json
import re
import subprocess
import time
import urllib.parse
import xml.etree.ElementTree as ET
from pathlib import Path

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--router', required=True)
p.add_argument('--emulator', required=True)
p.add_argument('--address', required=True)
p.add_argument('--out', type=Path, required=True)
a = p.parse_args()
assert a.router.startswith('fw-requests-router-') and a.emulator.startswith('fw-requests-emu-')
a.out.mkdir(parents=True, exist_ok=True)
PKG = 'io.github.denogio.familywarden'
FIXTURE = PKG + '.fixture'
ADB = ['docker', 'exec', a.emulator, '/opt/extra/platform-tools/adb']


def run(argv, data=None, timeout=55, check=True):
    r = subprocess.run(argv, input=data, capture_output=True, text=True, timeout=timeout)
    if check and r.returncode:
        raise AssertionError((argv[:5], r.stdout[-1200:], r.stderr[-1200:]))
    return r.stdout


def adb(*argv, **kwargs):
    return run(ADB + list(argv), **kwargs).replace('\r', '')


def router(script):
    return run(['docker', 'exec', a.router, 'sh', '-ec', script])


def call(method, value=None):
    return json.loads(run(['docker', 'exec', '-i', a.router, '/usr/libexec/rpcd/familywarden', 'call', method],
                          data=json.dumps(value or {})))


def wait(label, predicate, seconds=45):
    until = time.monotonic() + seconds
    last = None
    while time.monotonic() < until:
        try:
            last = predicate()
            if last:
                print('PASS', label, flush=True)
                return last
        except (AssertionError, ValueError) as error:
            last = str(error)
        time.sleep(2)
    raise AssertionError((label, last))


def sync():
    adb('shell', 'am', 'broadcast', '-n', PKG + '/.DebugReceiver', '-a', PKG + '.DEBUG_SYNC')
    time.sleep(2)


def ui(name=None):
    for _ in range(4):
        adb('shell', 'rm', '-f', '/sdcard/fw-requests.xml')
        adb('shell', 'uiautomator', 'dump', '/sdcard/fw-requests.xml', check=False)
        text = adb('shell', 'cat', '/sdcard/fw-requests.xml', check=False)
        try:
            tree = ET.fromstring(text)
            if name:
                (a.out / (name + '.xml')).write_text(text)
                (a.out / (name + '.png')).write_bytes(subprocess.check_output(ADB + ['exec-out', 'screencap', '-p'], timeout=30))
            return tree
        except ET.ParseError:
            time.sleep(1)
    raise AssertionError('no fresh UI hierarchy')


def nodes(text):
    return [n for n in ui().iter('node') if text in (n.get('text'), n.get('content-desc'))]


def tap(text, scroll=False, last=False):
    for _ in range(9 if scroll else 3):
        found = nodes(text)
        if found:
            n = found[-1] if last else next((n for n in found if n.get('clickable') == 'true'), found[0])
            x1, y1, x2, y2 = map(int, re.findall(r'\d+', n.get('bounds')))
            assert x2 > x1 and y2 > y1, (text, n.attrib)
            adb('shell', 'input', 'tap', str((x1+x2)//2), str((y1+y2)//2))
            time.sleep(1)
            return
        if scroll:
            adb('shell', 'input', 'swipe', '500', '1800', '500', '650', '350')
        time.sleep(1)
    raise AssertionError('missing UI control: ' + text)


def minutes(value):
    fields = [n for n in ui().iter('node') if n.get('class') == 'android.widget.EditText']
    assert fields, 'missing minutes field'
    x1, y1, x2, y2 = map(int, re.findall(r'\d+', fields[0].get('bounds')))
    adb('shell', 'input', 'tap', str((x1+x2)//2), str((y1+y2)//2))
    adb('shell', 'input', 'keyevent', 'KEYCODE_MOVE_END')
    adb('shell', 'input', 'keyevent', '67', '67', '67', '67')
    adb('shell', 'input', 'text', str(value))
    # No BACK key: without a visible keyboard it would dismiss the dialog itself.
    # The keyboard moves the dialog up; tapping before it settles lands outside the
    # dialog and dismisses it. Wait until two consecutive layouts agree.
    last = None
    for _ in range(10):
        time.sleep(0.7)
        now = [n.get('bounds') for n in ui().iter('node') if n.get('class') == 'android.widget.EditText']
        if now and now == last:
            break
        last = now
    time.sleep(1)


def home():
    adb('shell', 'input', 'keyevent', 'KEYCODE_WAKEUP')
    adb('shell', 'wm', 'dismiss-keyguard')
    adb('shell', 'am', 'start', '-W', '-n', PKG + '/.MainActivity')
    time.sleep(2)


def suspended():
    return 'suspended=true' in adb('shell', 'dumpsys', 'package', FIXTURE)


def usage():
    return json.loads(adb('shell', 'run-as', PKG, 'cat', 'files/app-usage.json'))


def spent(grant_id):
    return next(r['used'] for r in usage()['grants']['records'] if r['id'] == grant_id)


def synthetic_phone(profile='alex'):
    """A disposable second phone of the child; its key never leaves the router container."""
    router(f'''
[ -f /tmp/requests-kid.key ] || usign -G -p /tmp/requests-kid.pub -s /tmp/requests-kid.key
key=$(sed -n 2p /tmp/requests-kid.pub)
printf 'familywarden-sync-v2\\nfw-test-kid\\n{'c'*64}\\n' >/tmp/kid-sync.msg
usign -S -s /tmp/requests-kid.key -m /tmp/kid-sync.msg -x /tmp/kid-sync.sig
sig=$(sed -n 2p /tmp/kid-sync.sig)
QUERY_STRING="device=fw-test-kid&nonce={'c'*64}&key=$key&sig=$sig" /usr/sbin/familywardend cgi-phone >/dev/null
''')
    pending = next((d for d in call('devices')['pending'] if d['id'] == 'fw-test-kid'), None)
    if pending:
        assert call('device_approve', {'device': pending['id'], 'profile': profile, 'name': 'Alex tablet',
                                       'fingerprint': pending['fingerprint']})['status'] == 'ok'


def synthetic_upload(op, value):
    raw = json.dumps(value, separators=(',', ':'))
    # Exact UTF-8 bytes on stdin, not through shell interpolation.
    return run(['docker', 'exec', '-i', a.router, 'sh', '-ec', f'''
cat >/tmp/kid-up.json
{{ printf 'familywarden-{op}-v1\\n'; cat /tmp/kid-up.json; }} >/tmp/kid-up.msg
usign -S -s /tmp/requests-kid.key -m /tmp/kid-up.msg -x /tmp/kid-up.sig
{{ sed -n 2p /tmp/kid-up.sig; cat /tmp/kid-up.json; }} | REQUEST_METHOD=POST QUERY_STRING=op={op} /usr/sbin/familywardend cgi-app
'''], data=raw)


def synthetic_request(request_id, profile='alex', amount=30):
    synthetic_phone(profile)
    output = synthetic_upload('request', {'type': 'request', 'v': 1, 'request_id': request_id, 'device': 'fw-test-kid',
        'profile': profile, 'target': FIXTURE, 'minutes': amount, 'label': 'Video',
        'created': int(router('date +%s').strip())})
    assert '"state":"pending"' in output, output


try:
    wait('Android 15 booted', lambda: adb('shell', 'getprop', 'sys.boot_completed', check=False).strip() == '1', 240)
    adb('install', '-r', '/app.apk')
    adb('install', '-r', '/probe/fixture-1.apk')
    adb('shell', 'settings', 'put', 'system', 'screen_off_timeout', '1800000')
    adb('shell', 'settings', 'put', 'global', 'window_animation_scale', '0')
    adb('shell', 'settings', 'put', 'global', 'transition_animation_scale', '0')
    adb('shell', 'settings', 'put', 'global', 'animator_duration_scale', '0')
    wait('Device Owner enrolled', lambda: 'Success' in adb('shell', 'dpm', 'set-device-owner', PKG + '/.AdminReceiver', check=False)
         or 'DeviceOwner' in adb('shell', 'dpm', 'list-owners'), 60)
    adb('shell', 'appops', 'set', '--uid', PKG, 'GET_USAGE_STATS', 'allow')
    home()
    key = router('familywarden mast-pubkey').splitlines()[0]
    uri = 'familywarden://pair?host=router.invalid&address=' + a.address + '&key=' + urllib.parse.quote(key, safe='')
    adb('shell', 'am', 'broadcast', '-n', PKG + '/.DebugReceiver', '-a', PKG + '.DEBUG_PAIR', '--es', 'uri', "'" + uri + "'")
    device = wait('phone appears for fingerprint approval', lambda: next(iter(call('devices')['pending']), None))
    device_id = device['id']
    assert call('device_approve', {'device': device_id, 'profile': 'alex', 'name': 'Alex phone', 'fingerprint': device['fingerprint']})['status'] == 'ok'
    sync()
    wait('one usage collector checkpoint exists', lambda: usage()['zone'] == 'UTC')
    # New-app approval: the fixture is not approved yet, so it waits for a parent.
    wait('an unapproved new app is paused', suspended, 60)
    def reported_new():
        sync()
        alex = next((p for p in call('new_apps')['profiles'] if p['id'] == 'alex'), {})
        return any(a['package'] == FIXTURE for a in alex.get('pending', []))
    wait('the paused new app is reported to the router', reported_new, 150)
    home()
    tap('Requests')
    wait('the child sees it waiting for a parent', lambda: bool(nodes('Waiting for a parent')))
    assert call('app_decision', {'profile': 'alex', 'package': FIXTURE, 'action': 'approve'})['state'] == 'approved'
    def unpaused():
        sync()
        return not suspended()
    wait('the allowed app runs only after the signed policy arrives', unpaused, 60)
    router("""uci add familywarden app >/dev/null
uci set familywarden.@app[-1].profile=alex
uci set familywarden.@app[-1].package=io.github.denogio.familywarden.fixture
uci set familywarden.@app[-1].minutes=1
uci commit familywarden
familywarden reload >/dev/null""")
    adb('shell', 'input', 'keyevent', 'KEYCODE_HOME')
    # Shared budget (D4): the tablet used the whole minute, so the app pauses here too.
    synthetic_phone()
    out = synthetic_upload('usage', {'type': 'usage', 'v': 1, 'device': 'fw-test-kid', 'profile': 'alex',
        'day': router('date +%F').strip(), 'seq': 1, 'total': 60, 'apps': {FIXTURE: 60}})
    assert '"state":"stored"' in out, out
    def paused_by_other_phone():
        sync()
        return suspended()
    wait("another phone's usage pauses the app before any local use", paused_by_other_phone, 60)
    day = router('date +%F').strip()
    assert usage()['days'].get(day, {}).get(FIXTURE, 0) < 1000, usage()['days'].get(day)
    adb('shell', 'am', 'start', '-W', '-n', FIXTURE + '/.ProbeActivity')
    wait('the standing app minute is exhausted and suspended', suspended, 115)
    home()
    tap('Requests')
    tap('FamilyWarden install fixture', scroll=True)
    minutes(1)
    router("""cat > /www-familywarden/cgi-bin/app <<'SH'
#!/bin/sh
if [ -e /tmp/drop-next-app-response ]; then
    rm /tmp/drop-next-app-response
    /usr/sbin/familywardend -socket /var/run/familywarden-core/core.sock cgi-app >/tmp/lost-app-response
    printf 'Status: 503 Service Unavailable\\nContent-Type: text/plain\\n\\n'
else
    exec /usr/sbin/familywardend -socket /var/run/familywarden-core/core.sock cgi-app
fi
SH
chmod 755 /www-familywarden/cgi-bin/app
touch /tmp/drop-next-app-response
""")
    tap('Send request', last=True)
    request = wait('child UI submits one real signed request', lambda: next((r for r in call('requests')['requests'] if r['state'] == 'pending'), None))
    assert request['minutes'] == 1 and request['device'] == device_id
    wait('uncertain delivery retains a retry rather than another request', lambda: bool(nodes('Retry action')))
    old_pid = adb('shell', 'pidof', PKG).strip()
    adb('shell', f"run-as {PKG} sh -c 'kill -9 $(pidof {PKG})'", check=False)
    wait('request process really died', lambda: adb('shell', 'pidof', PKG, check=False).strip() != old_pid)
    home()
    tap('Requests')
    wait('exact-byte retry survives process death', lambda: bool(nodes('Retry action')))
    tap('Retry action')
    wait('verified retry resolves the local outbox', lambda: not nodes('Retry action'))
    assert len([r for r in call('requests')['requests'] if r['request_id'] == request['request_id']]) == 1
    ui('native-child-pending')
    decision = call('request_answer', {'request_id': request['request_id'], 'approve': True, 'minutes': 1})
    assert decision['state'] == 'approved' and decision['home_internet'] is True, decision
    sync()
    wait('approved minute unpauses only through verified policy', lambda: not suspended())
    row = next(r for r in call('requests')['requests'] if r['request_id'] == request['request_id'])
    grant_id = next(r['id'] for r in usage()['grants']['records'] if r['device'] == device_id)
    before = spent(grant_id)
    assert before == 0, before
    adb('shell', 'am', 'start', '-W', '-n', FIXTURE + '/.ProbeActivity')
    wait('granted usage is persisted', lambda: spent(grant_id) >= 15_000, 45)
    checkpoint = spent(grant_id)
    # Same-UID kill simulates process death, not a privileged uninstall or reset.
    old_pid = adb('shell', 'pidof', PKG).strip()
    adb('shell', f"run-as {PKG} sh -c 'kill -9 $(pidof {PKG})'", check=False)
    wait('grant process really died', lambda: adb('shell', 'pidof', PKG, check=False).strip() != old_pid)
    time.sleep(3)
    sync()
    wait('restart retains consumption instead of refilling the grant', lambda: spent(grant_id) >= checkpoint)
    wait('the approved minute is exhausted and paused again', suspended, 115)
    assert spent(grant_id) == 60_000, usage()['grants']
    sync()
    time.sleep(2)
    assert suspended() and spent(grant_id) == 60_000
    print('PASS repeated delivery cannot refill consumed minutes', flush=True)
    def reported():
        sync()
        report = call('usage')
        alex = next((x for x in report['profiles'] if x['id'] == 'alex'), None)
        used = next((r.get('used') for r in call('requests')['requests'] if r['request_id'] == request['request_id']), None)
        return alex and alex['total'] >= 60 and used is not None and used >= 59 and alex
    alex_usage = wait('signed usage report reaches the router with grant use', reported, 150)
    assert any(a['package'] == FIXTURE for a in alex_usage['apps']), alex_usage
    home()
    ui('native-child-exhausted')
    assert call('device_allow_removal', {'device': device_id})['status'] == 'ok'
    sync()
    home()
    tap('Settings')
    tap('Remove FamilyWarden', scroll=True)
    tap('Remove', last=True)
    wait('normal authorized removal releases Device Owner', lambda: 'DeviceOwner' not in adb('shell', 'dpm', 'list-owners'))
    assert call('device_update', {'device': device_id, 'profile': 'parent', 'name': 'Taylor phone', 'approves': 'alex'})['status'] == 'ok'
    home()
    tap('Use this phone as a parent', scroll=True)
    # A fresh parent app may need the standard runtime notification prompt.
    for label in ('Allow',):
        if nodes(label):
            tap(label)
    adb('shell', 'am', 'start', '-W', '-a', 'android.intent.action.VIEW', '-d', "'" + uri + "'", PKG)
    time.sleep(2)
    tap('It matches, pair')
    wait('ordinary parent is paired without Device Owner', lambda: bool(nodes('No requests waiting')))
    assert 'DeviceOwner' not in adb('shell', 'dpm', 'list-owners')
    synthetic_request('request-native-parent-001')
    sync()
    wait('parent receives the scoped request', lambda: bool(nodes('Approve')))
    wait('parent sees the child\'s screen time today', lambda: bool(nodes('Screen time today')))
    out = synthetic_upload('usage', {'type': 'usage', 'v': 1, 'device': 'fw-test-kid', 'profile': 'alex',
        'day': router('date +%F').strip(), 'seq': 2, 'total': 60, 'apps': {FIXTURE: 60},
        'new_apps': {'com.example.draw': 'Draw'}})
    assert '"state"' in out, out
    sync()
    wait('parent sees the new app', lambda: bool(nodes('Allow')))
    ui('native-parent-new-app')
    tap('Allow')
    wait('non-owner parent allows a new app (signed)', lambda: any(a['package'] == 'com.example.draw'
        for p in call('new_apps')['profiles'] if p['id'] == 'alex' for a in p['approved']))
    ui('native-parent-pending')
    tap('Approve')
    minutes(15)
    tap('Approve', last=True)
    answered = wait('non-owner parent signs a real scoped approval', lambda: next((r for r in call('requests')['requests']
        if r['request_id'] == 'request-native-parent-001' and r['state'] == 'approved'), None))
    assert answered['granted'] == 15 and answered['answered_by'] == 'Taylor phone', answered
    assert 'DeviceOwner' not in adb('shell', 'dpm', 'list-owners')
    synthetic_request('request-native-parent-002', amount=10)
    sync()
    tap('Decline')
    tap('Decline', last=True)
    wait('non-owner parent can decline without minting another grant', lambda: any(r['request_id'] == 'request-native-parent-002' and r['state'] == 'denied'
        for r in call('requests')['requests']))
    ui('native-parent-complete')
    (a.out / 'result.json').write_text(json.dumps({'device': device_id, 'spent_millis': spent(grant_id),
        'parent_device_owner': False, 'child_grant_id': grant_id, 'checks': 'passed'}, indent=2))
    print('All focused P8 native checks passed', flush=True)
except BaseException:
    (a.out / 'android.log').write_text(adb('logcat', '-d', '-s', 'UlyssesMast:*', 'AndroidRuntime:E', check=False))
    (a.out / 'core.log').write_text(router('cat /tmp/core.log'))
    try:
        ui('failure')
    except BaseException:
        pass
    raise
