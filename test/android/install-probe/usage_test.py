#!/usr/bin/env python3
"""Local collector/enforcer smoke test. Existing account and enrollment are preserved.
python3 usage_test.py /tmp/fw-install-probe [--reboot]
Leaves the local usage checkpoint for persistence inspection, stops the debug service,
restores usage access to allow, and removes only our synthetic fixture.
"""
import argparse
import datetime
import json
from pathlib import Path
import subprocess
import time
from zoneinfo import ZoneInfo

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('apks', type=Path)
parser.add_argument('--serial', default='emulator-5556')
parser.add_argument('--reboot', action='store_true')
args = parser.parse_args()
assert args.serial.startswith('emulator-'), 'Only disposable emulators are supported'
FW = 'io.github.denogio.familywarden'
PKG = FW + '.fixture'


def adb(*argv, check=True):
    return subprocess.run(['adb', '-s', args.serial, *argv], capture_output=True,
                          text=True, check=check, timeout=45).stdout


def control(enabled, seconds=86400):
    adb('shell', 'am', 'broadcast', '-n', FW + '/.DebugProbeReceiver', '-a',
        FW + '.PROBE_APP_CONTROL', '--ez', 'enabled', str(enabled).lower(), '--ei', 'seconds', str(seconds))


def snapshot():
    data = json.loads(adb('shell', 'run-as', FW, 'cat', 'files/app-usage.json'))
    day = datetime.datetime.now(ZoneInfo(data['zone'])).date().isoformat()
    return data, data['days'].get(day, {}).get(PKG, 0)


def launch():
    adb('shell', 'input', 'keyevent', 'KEYCODE_WAKEUP')
    adb('shell', 'wm', 'dismiss-keyguard')
    adb('shell', 'am', 'start', '-W', '-n', PKG + '/.ProbeActivity')


def suspended():
    return 'suspended=true' in adb('shell', 'dumpsys', 'package', PKG)


assert FW in adb('shell', 'dpm', 'list-owners')
try:
    adb('shell', 'appops', 'set', '--uid', FW, 'GET_USAGE_STATS', 'allow')
    adb('install', '-r', str(args.apks / 'fixture-1.apk'))
    adb('shell', 'input', 'keyevent', 'KEYCODE_HOME')
    control(True)
    time.sleep(3)
    _, initial = snapshot()
    launch()
    time.sleep(5)
    _, foreground = snapshot()
    assert 3000 <= foreground - initial <= 9000, (initial, foreground)
    print('Foreground time counted:', foreground - initial, 'ms')

    adb('shell', 'input', 'keyevent', 'KEYCODE_SLEEP')
    time.sleep(3)
    _, off_start = snapshot()
    time.sleep(5)
    _, off_end = snapshot()
    assert off_end == off_start, (off_start, off_end)
    print('Screen-off increment:', off_end - off_start, 'ms')

    launch()
    time.sleep(3)
    adb('shell', 'input', 'keyevent', 'KEYCODE_HOME')
    time.sleep(3)
    _, background = snapshot()
    time.sleep(3)
    _, background_end = snapshot()
    assert background_end == background
    print('Background increment after activity stops:', background_end - background, 'ms')

    data, before_restart = snapshot()
    control(False)
    time.sleep(2)
    control(True)
    time.sleep(3)
    after, after_restart = snapshot()
    assert after_restart == before_restart
    assert after['seq'] > data['seq']
    print('Collector restart preserved totals without double counting:', after_restart, 'ms')

    seconds = (after_restart + 999) // 1000 + 5
    control(True, seconds)
    launch()
    for _ in range(15):
        time.sleep(1)
        if suspended():
            break
    else:
        raise RuntimeError('Daily local budget did not suspend fixture')
    # Suspension is asynchronous; collect the final STOP event before comparing totals.
    time.sleep(3)
    _, exhausted = snapshot()
    assert seconds * 1000 <= exhausted <= seconds * 1000 + 5000
    print('Budget suspended fixture at', exhausted, 'ms; limit', seconds * 1000, 'ms')

    adb('install', '-r', str(args.apks / 'fixture-2.apk'))
    time.sleep(2)
    assert suspended()
    assert snapshot()[1] == exhausted
    print('APK update preserved usage and suspension')

    adb('shell', 'appops', 'set', '--uid', FW, 'GET_USAGE_STATS', 'ignore')
    time.sleep(3)
    data, revoked = snapshot()
    assert revoked == exhausted and data['gaps'] > 0 and data['uncertain_through_day']
    assert suspended()
    adb('shell', 'appops', 'set', '--uid', FW, 'GET_USAGE_STATS', 'allow')
    print('Revocation reported a gap, retained totals and retained the exhausted-app block')

    if args.reboot:
        old_boot = adb('shell', 'cat', '/proc/sys/kernel/random/boot_id').strip()
        adb('reboot')
        for _ in range(90):
            time.sleep(2)
            boot = adb('shell', 'cat', '/proc/sys/kernel/random/boot_id', check=False).strip()
            if boot and boot != old_boot and adb('shell', 'getprop', 'sys.boot_completed', check=False).strip() == '1':
                break
        else:
            raise RuntimeError('Reboot timed out')
        adb('shell', 'input', 'swipe', '500', '1800', '500', '500', '300')
        for _ in range(30):
            time.sleep(2)
            if snapshot()[0]['seq'] > data['seq'] and suspended():
                break
        else:
            raise RuntimeError('Collector boot recovery failed')
        assert snapshot()[1] >= exhausted
        print('Reboot retained usage and block and restarted the collector')
finally:
    adb('shell', 'appops', 'set', '--uid', FW, 'GET_USAGE_STATS', 'allow', check=False)
    control(False)
    time.sleep(2)
    adb('uninstall', PKG, check=False)
