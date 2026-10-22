#!/usr/bin/env python3
"""Emulator-only lock-task research; no first-code-execution/background guarantee.
python3 lock_task_test.py /tmp/fw-install-probe [--play-update com.google.android.youtube] [--reboot]
The optional update test needs English Play UI, a signed-in test account and an available
update. It queues a real Play update BEFORE entering lock task, then checks completion
while Play's activities are not allowlisted. This does not prove periodic auto-update.
"""
import argparse
import concurrent.futures
from pathlib import Path
import re
import subprocess
import time
import xml.etree.ElementTree as E

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('apks', type=Path)
parser.add_argument('--serial', default='emulator-5556')
parser.add_argument('--play-update', choices=['com.google.android.youtube', 'com.google.android.gm', 'com.google.android.apps.docs', 'com.google.android.apps.maps'])
parser.add_argument('--reboot', action='store_true')
args = parser.parse_args()
assert args.serial.startswith('emulator-')
FW = 'io.github.denogio.familywarden'
PKG = FW + '.fixture'


def adb(*argv, check=True):
    return subprocess.run(['adb', '-s', args.serial, *argv], capture_output=True,
                          text=True, check=check, timeout=45).stdout


def probe(enabled):
    adb('shell', 'am', 'broadcast', '-n', FW + '/.DebugProbeReceiver', '-a', FW + '.PROBE_LOCK_TASK',
        '--ez', 'enabled', str(enabled).lower())


def mode():
    match = re.search(r'mLockTaskModeState=(\w+)', adb('shell', 'dumpsys', 'activity'))
    return match.group(1) if match else 'unknown'


def fixture_logs():
    return adb('logcat', '-d', '-s', 'FwInstallFixture:I', '*:S').splitlines()


def version(pkg):
    m = re.search(r'versionCode=(\d+)', adb('shell', 'dumpsys', 'package', pkg))
    return m.group(1) if m else None


assert FW in adb('shell', 'dpm', 'list-owners')
assert mode() == 'NONE', 'Do not disturb an existing lock-task session'
pid = None
try:
    adb('shell', 'input', 'keyevent', 'KEYCODE_WAKEUP')
    adb('shell', 'wm', 'dismiss-keyguard')
    if args.play_update:
        old_version = version(args.play_update)
        adb('shell', 'am', 'start', '-W', '-a', 'android.intent.action.VIEW', '-d',
            'market://details?id=' + args.play_update, 'com.android.vending')
        time.sleep(5)
        adb('shell', 'uiautomator', 'dump', '/sdcard/fw-locktask-ui.xml')
        root = E.fromstring(adb('exec-out', 'cat', '/sdcard/fw-locktask-ui.xml'))
        update = next(n for n in root.iter('node') if n.get('text') == 'Update')
        x1, y1, x2, y2 = map(int, re.findall(r'\d+', update.get('bounds')))
        adb('shell', 'input', 'tap', str((x1+x2)//2), str((y1+y2)//2))

    probe(True)
    time.sleep(3)
    assert mode() == 'LOCKED'
    packages = adb('shell', 'dumpsys', 'activity').split('mLockTaskPackages (userId:packages)=', 1)[1].split('\n\n', 1)[0]
    assert 'com.android.vending' not in packages
    assert 'suspended=false' in adb('shell', 'dumpsys', 'package', 'com.android.vending')
    print('Native lock-task mode active; Play installed and unsuspended, but not activity-allowlisted')
    if args.play_update:
        assert version(args.play_update) == old_version, 'Update completed before lock task; inconclusive test'
    start = len(fixture_logs())
    for _ in range(5):
        adb('uninstall', PKG, check=False)
        monitor = (f'while ! pm path {PKG} 2>/dev/null | grep -q package:; do sleep 0.01; done; '
                   f'am start -n {PKG}/.ProbeActivity >/dev/null 2>&1')
        with concurrent.futures.ThreadPoolExecutor() as pool:
            launch = pool.submit(adb, 'shell', monitor)
            assert 'Success' in adb('install', '--no-streaming', str(args.apks / 'fixture-1.apk'))
            launch.result(timeout=45)
        time.sleep(1)
        assert 'suspended=false' in adb('shell', 'dumpsys', 'package', PKG), 'Suspension would invalidate native-block evidence'
    assert not any('CREATED' in l for l in fixture_logs()[start:])
    print('Five first-launch races: zero fixture onCreate calls')

    adb('shell', 'am', 'broadcast', '-n', FW + '/.DebugProbeReceiver', '-a', FW + '.PROBE_LOCK_TASK_IN_TASK')
    time.sleep(2)
    assert not any('CREATED' in l for l in fixture_logs()[start:])
    assert 'attempting non-allowlisted activity' in adb('logcat', '-d', '-s', 'FwLockTaskProbe:I', '*:S')
    print('Non-allowlisted activity launched from inside an allowlisted task was blocked')

    adb('shell', 'am', 'start', '-W', '-a', 'android.intent.action.MAIN', '-c', 'android.intent.category.LAUNCHER', '-p', 'com.google.android.gm')
    for _ in range(20):
        if re.search(r'topResumedActivity=.*com.google.android.gm/', adb('shell', 'dumpsys', 'activity', 'activities')):
            break
        time.sleep(0.5)  # Gmail can redirect into its own first-run welcome activity.
    else:
        raise RuntimeError('Approved Gmail positive control did not reach the foreground')
    print('Approved Gmail positive control launched')
    adb('shell', 'input', 'keyevent', 'KEYCODE_HOME')
    assert mode() == 'LOCKED'
    pid = adb('shell', 'pidof', FW).strip()
    assert pid.isdigit()
    adb('shell', 'run-as', FW, 'kill', '-STOP', pid)
    adb('shell', 'am', 'start', '-W', '-n', PKG + '/.ProbeActivity')
    time.sleep(1)
    assert not any('CREATED' in l for l in fixture_logs()[start:])
    adb('shell', 'run-as', FW, 'kill', '-CONT', pid)
    pid = None
    print('Native activity block also held while FamilyWarden process was paused')

    provider = adb('shell', 'content', 'query', '--uri', 'content://' + PKG + '.provider/test')
    assert 'synthetic' in provider
    assert 'PROVIDER_CREATED' in adb('logcat', '-d', '-s', 'FwProviderFixture:I', '*:S')
    assert not any('CREATED' in l for l in fixture_logs()[start:])
    print('Exported provider code CAN run while activities are blocked; not a blanket code-execution barrier')

    if args.play_update:
        for _ in range(24):
            time.sleep(5)
            if version(args.play_update) != old_version:
                break
        else:
            raise RuntimeError('Queued Play update did not finish in lock task within 120 s')
        assert mode() == 'LOCKED'
        print('Queued Play update completed with Play activities NOT allowlisted:', old_version, '->', version(args.play_update))

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
        time.sleep(10)
        print('Mode after reboot/unlock without automatic re-entry:', mode())
finally:
    if pid:
        adb('shell', 'run-as', FW, 'kill', '-CONT', pid, check=False)
    probe(False)
    for _ in range(20):
        if mode() == 'NONE':
            break
        time.sleep(0.5)
    if mode() != 'NONE':
        # Last-resort emulator-only recovery: remove just our experimental locked root.
        # Never force-stop Android/SystemUI, reset the device, or touch account data.
        section = adb('shell', 'dumpsys', 'activity').split('mLockTaskModeTasks=', 1)[1].split('mLockTaskPackages', 1)[0]
        for line in section.splitlines():
            if re.search(r'A=\d+:' + re.escape(FW) + r'\}', line):
                task = re.search(r' #(\d+) type=', line).group(1)
                adb('shell', 'am', 'stack', 'remove', task)
                print('WARNING: cleanup required adb removal of our experimental task; not production-safe')
                break
        probe(False)
        time.sleep(2)
    adb('uninstall', PKG, check=False)
    assert mode() == 'NONE', 'Experimental mode was not exited'
    print('Original lock-task policy restored; fixture removed')
