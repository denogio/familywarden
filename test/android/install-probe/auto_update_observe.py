#!/usr/bin/env python3
"""Bounded observation, not a deterministic auto-update test. Does not change preferences.
No Play update is tapped/queued. An unchanged version is INCONCLUSIVE, never proof that
updates cannot work. Emulator only; restores native policy and wakes the display.
"""
import argparse
import re
import subprocess
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--serial', default='emulator-5556')
parser.add_argument('--seconds', type=int, default=600)
args = parser.parse_args()
assert args.serial.startswith('emulator-') and 30 <= args.seconds <= 1800
FW = 'io.github.denogio.familywarden'
PACKAGES = ['com.google.android.calendar', 'com.google.android.apps.messaging']


def adb(*argv, check=True):
    return subprocess.run(['adb', '-s', args.serial, *argv], capture_output=True,
                          text=True, check=check, timeout=45).stdout


def probe(enabled):
    adb('shell', 'am', 'broadcast', '-n', FW + '/.DebugProbeReceiver', '-a', FW + '.PROBE_LOCK_TASK',
        '--ez', 'enabled', str(enabled).lower())


def mode():
    return re.search(r'mLockTaskModeState=(\w+)', adb('shell', 'dumpsys', 'activity')).group(1)


def versions():
    return {p: re.search(r'versionCode=(\d+)', adb('shell', 'dumpsys', 'package', p)).group(1)
            for p in PACKAGES}


assert FW in adb('shell', 'dpm', 'list-owners') and mode() == 'NONE'
try:
    adb('shell', 'input', 'keyevent', 'KEYCODE_WAKEUP')
    adb('shell', 'wm', 'dismiss-keyguard')
    probe(True)
    time.sleep(2)
    assert mode() == 'LOCKED'
    before = versions()
    print('Baseline package versions:', before, flush=True)
    battery = adb('shell', 'dumpsys', 'battery')
    print('AC powered:', 'AC powered: true' in battery, flush=True)
    adb('shell', 'input', 'keyevent', 'KEYCODE_SLEEP')
    time.sleep(2)
    power = re.search(r'mWakefulness=(\w+)', adb('shell', 'dumpsys', 'power'))
    print('Wakefulness:', power.group(1) if power else 'unknown', flush=True)
    started = time.monotonic()
    changes = {}
    while time.monotonic() - started < args.seconds:
        time.sleep(min(30, max(0, args.seconds - (time.monotonic() - started))))
        assert mode() == 'LOCKED'
        current = versions()
        changes = {p: (before[p], current[p]) for p in PACKAGES if current[p] != before[p]}
        if changes:
            print('Observed background package update(s):', changes, flush=True)
            break
    duration = round(time.monotonic() - started)
    if not changes:
        print(f'INCONCLUSIVE: no watched package version changed in {duration} s; auto-update preference/scheduling not established', flush=True)
    else:
        print('Update observed after', duration, 's with no foreground Play update request', flush=True)
finally:
    adb('shell', 'input', 'keyevent', 'KEYCODE_WAKEUP')
    adb('shell', 'wm', 'dismiss-keyguard')
    probe(False)
    for _ in range(20):
        if mode() == 'NONE':
            break
        time.sleep(0.5)
    assert mode() == 'NONE', 'Native observation policy was not exited'
    adb('shell', 'input', 'keyevent', 'KEYCODE_HOME')
    print('Native policy restored; display awake', flush=True)
