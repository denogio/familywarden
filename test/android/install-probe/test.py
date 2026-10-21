#!/usr/bin/env python3
"""Bounded emulator experiment, not a zero-race security proof. Never wipes accounts.
Build APKs with build.sh, then: python3 test.py /tmp/fw-install-probe [--reboot]
Requires the FamilyWarden debug Device Owner with PROBE_PACKAGE_WATCH support.
"""
import argparse
import concurrent.futures
from pathlib import Path
import subprocess
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('apks', type=Path)
parser.add_argument('--serial', default='emulator-5556')
parser.add_argument('--reboot', action='store_true')
args = parser.parse_args()
assert args.serial.startswith('emulator-'), 'Only disposable emulators are supported'
FW = 'io.github.denogio.familywarden'
PKG = FW + '.fixture'


def adb(*argv, check=True):
    return subprocess.run(['adb', '-s', args.serial, *argv], text=True,
                          capture_output=True, check=check, timeout=45).stdout


def probe(enabled):
    adb('shell', 'am', 'broadcast', '-n', FW + '/.DebugProbeReceiver', '-a',
        FW + '.PROBE_PACKAGE_WATCH', '--ez', 'enabled', str(enabled).lower())


def logs():
    return adb('logcat', '-d', '-s', 'FwPackageProbe:I', 'FwInstallFixture:I', '*:S').splitlines()


def install(version, *flags):
    assert 'Success' in adb('install', '--no-streaming', *flags, str(args.apks / f'fixture-{version}.apk'))


def suspended():
    return 'suspended=true' in adb('shell', 'dumpsys', 'package', PKG)


assert FW in adb('shell', 'dpm', 'list-owners'), 'Expected FamilyWarden Device Owner'
paused_pid = None
try:
    probe(True)
    time.sleep(2)
    start = len(logs())
    for _ in range(5):
        adb('uninstall', PKG, check=False)
        monitor = (f'while ! pm path {PKG} 2>/dev/null | grep -q package:; do sleep 0.01; done; '
                   f'am start -n {PKG}/.ProbeActivity >/dev/null 2>&1')
        with concurrent.futures.ThreadPoolExecutor() as pool:
            launch = pool.submit(adb, 'shell', monitor)
            install(1)
            launch.result(timeout=45)
        time.sleep(2)
        assert suspended(), 'Dynamic receiver did not suspend the fixture'
    install(2, '-r')
    time.sleep(2)
    ordinary = logs()[start:]
    assert any('PACKAGE_REPLACED' in line for line in ordinary)
    print('Five install/launch races; ordinary successful starts:', sum('CREATED' in line for line in ordinary))
    print('\n'.join(ordinary))  # Only synthetic package logs, no account data.

    adb('uninstall', PKG)
    time.sleep(1)
    paused_pid = adb('shell', 'pidof', FW).strip()
    assert paused_pid.isdigit()
    start = len(logs())
    adb('shell', 'run-as', FW, 'kill', '-STOP', paused_pid)
    install(1)
    adb('shell', 'am', 'start', '-W', '-n', PKG + '/.ProbeActivity')
    time.sleep(1)
    assert any('CREATED' in line for line in logs()[start:]), 'Expected execution during controlled watcher pause'
    adb('shell', 'run-as', FW, 'kill', '-CONT', paused_pid)
    paused_pid = None
    time.sleep(3)
    assert suspended(), 'Recovery did not suspend the fixture'
    print('Controlled process pause exposes a launch window; suspension recovered afterwards')

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
            if 'watching' in '\n'.join(logs()) and suspended():
                break
            time.sleep(2)
        else:
            raise RuntimeError('Boot reconciliation failed')
        print('Watcher and suspension recovered after reboot')
finally:
    if paused_pid:
        adb('shell', 'run-as', FW, 'kill', '-CONT', paused_pid, check=False)
    probe(False)
    adb('uninstall', PKG, check=False)
