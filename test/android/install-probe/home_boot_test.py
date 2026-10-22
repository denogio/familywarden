#!/usr/bin/env python3
"""Reversible Home/if_whitelisted boot research on a disposable emulator only.
python3 home_boot_test.py /tmp/fw-install-probe [--with-pin]
Temporarily changes default Home. Optional synthetic PIN is set ONLY after verifying the
emulator currently has no credential; it is cleared afterwards. Never removes accounts.
No call intents or telephone numbers are sent.
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
parser.add_argument('--with-pin', action='store_true')
parser.add_argument('--expect-preunlock-lock', action='store_true')
args = parser.parse_args()
assert args.serial.startswith('emulator-')
FW = 'io.github.denogio.familywarden'
PKG = FW + '.fixture'
HOME = FW + '/.LockTaskHomeProbe'
PIN = '764281'  # Public synthetic fixture credential, never a person's phone/account PIN.


def adb(*argv, check=True):
    return subprocess.run(['adb', '-s', args.serial, *argv], capture_output=True,
                          text=True, check=check, timeout=45).stdout


def home():
    text = adb('shell', 'cmd', 'package', 'resolve-activity', '--brief', '-a',
               'android.intent.action.MAIN', '-c', 'android.intent.category.HOME')
    return next(line for line in text.splitlines() if '/' in line)


def inactive():
    for path in ('shared_prefs/lock-task-probe.xml', '/data/user_de/0/' + FW + '/shared_prefs/lock-task-probe.xml'):
        text = adb('shell', 'run-as', FW, 'cat', path, check=False)
        if text.strip():
            root = E.fromstring(text)
            for name in ('active', 'home_boot'):
                flag = root.find("./boolean[@name='" + name + "']")
                if flag is not None and flag.get('value') == 'true':
                    return False
    return True


def set_home(component):
    assert 'Success' in adb('shell', 'cmd', 'package', 'set-home-activity', component)


def probe(enabled):
    adb('shell', 'am', 'broadcast', '-n', FW + '/.DebugProbeReceiver', '-a', FW + '.PROBE_LOCK_TASK',
        '--ez', 'enabled', str(enabled).lower())


def mode():
    return re.search(r'mLockTaskModeState=(\w+)', adb('shell', 'dumpsys', 'activity')).group(1)


def state():
    return adb('shell', 'am', 'get-started-user-state', '0').strip()


def logs():
    return adb('logcat', '-d', '-s', 'FwInstallFixture:I', '*:S').splitlines()


def unlock():
    if state() == 'RUNNING_UNLOCKED':
        return
    adb('shell', 'input', 'keyevent', 'KEYCODE_WAKEUP')
    adb('shell', 'input', 'swipe', '500', '1800', '500', '500', '300')
    time.sleep(1)
    if args.with_pin:
        adb('shell', 'input', 'text', PIN)
        adb('shell', 'input', 'keyevent', 'KEYCODE_ENTER')
    for _ in range(30):
        if state() == 'RUNNING_UNLOCKED':
            return
        time.sleep(1)
    raise RuntimeError('Could not unlock test emulator')


def reboot():
    old_boot = adb('shell', 'cat', '/proc/sys/kernel/random/boot_id').strip()
    adb('reboot')
    for _ in range(90):
        time.sleep(2)
        boot = adb('shell', 'cat', '/proc/sys/kernel/random/boot_id', check=False).strip()
        if boot and boot != old_boot and adb('shell', 'getprop', 'sys.boot_completed', check=False).strip() == '1':
            return
    raise RuntimeError('Reboot timed out')


assert mode() == 'NONE'
assert inactive(), 'Previous experiment snapshot must be restored first'
original_home = home()
assert FW not in original_home, 'Do not overwrite an existing FamilyWarden Home setup'
assert FW in adb('shell', 'dpm', 'list-owners')
pin_set = False
try:
    if args.with_pin:
        assert 'verified successfully' in adb('shell', "locksettings verify --old ''"), 'Existing credential must not be changed'
        assert 'Pin set' in adb('shell', 'locksettings', 'set-pin', PIN)
        pin_set = True
    # Disabled in the manifest so ordinary debug installs never offer a second Home.
    adb('shell', 'pm', 'enable', HOME)
    adb('install', '-r', str(args.apks / 'fixture-1.apk'))
    baseline = len(logs())
    adb('shell', 'am', 'start', '-W', '-n', PKG + '/.ProbeActivity')
    time.sleep(1)
    assert any('CREATED' in line for line in logs()[baseline:]), 'Fixture must launch before the experiment'
    set_home(HOME)
    probe(True)
    time.sleep(3)
    assert mode() == 'LOCKED'
    assert home() == HOME
    print('Synthetic Home installed as default; native lock task active')
    reboot()
    print('After boot, before scripted unlock: user=' + state(), 'mode=' + mode())
    if args.with_pin:
        assert state() == 'RUNNING_LOCKED', 'PIN test did not exercise credential-locked boot'
    if args.expect_preunlock_lock:
        assert args.with_pin and mode() == 'LOCKED', 'Native barrier was not active before first unlock'
    start = len(logs())
    monitor = (f'while [ "$(am get-started-user-state 0)" != RUNNING_UNLOCKED ]; do sleep 0.01; done; '
               f'am start -n {PKG}/.ProbeActivity >/dev/null 2>&1')
    with concurrent.futures.ThreadPoolExecutor() as pool:
        launch = pool.submit(adb, 'shell', monitor)
        unlock()
        launch.result(timeout=45)
    time.sleep(3)
    first_starts = sum('CREATED' in l for l in logs()[start:])
    print('First-unlock race successful fixture starts:', first_starts)
    if args.expect_preunlock_lock:
        assert first_starts == 0
    for _ in range(30):
        if mode() == 'LOCKED':
            break
        time.sleep(1)
    else:
        raise RuntimeError('Home did not automatically enter lock task after boot')
    assert 'suspended=false' in adb('shell', 'dumpsys', 'package', PKG)
    print('Native Home lock-task active after reboot/unlock (pre-unlock result reported separately)')
    start = len(logs())
    adb('shell', 'am', 'start', '-W', '-n', PKG + '/.ProbeActivity')
    time.sleep(1)
    assert not any('CREATED' in line for line in logs()[start:])
    print('Steady-state unapproved launch blocked after boot')
finally:
    unlock()
    # Prevent if_whitelisted Home from immediately re-entering while we stop its root.
    set_home(original_home)
    probe(False)
    for _ in range(20):
        if mode() == 'NONE':
            break
        time.sleep(0.5)
    if mode() != 'NONE':
        print('WARNING: cleanup needs a non-wiping recovery reboot with the original Home')
        reboot()
        unlock()
        probe(False)
        time.sleep(2)
    # A stopped root is not sufficient proof that the saved policy was restored.
    if not inactive():
        probe(False)
        time.sleep(1)
    assert mode() == 'NONE' and inactive()
    assert home() == original_home
    adb('shell', 'pm', 'disable', HOME, check=False)
    if pin_set:
        assert 'cleared' in adb('shell', 'locksettings', 'clear', '--old', PIN).lower()
        adb('shell', 'wm', 'dismiss-keyguard')
    adb('uninstall', PKG, check=False)
    print('Original Home and lock-task policy restored; fixture/PIN removed')
