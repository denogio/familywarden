"""Fast tests of Mast selectors, mode routing and reboot progress; no emulator required."""
from pathlib import Path
import json
import os
import re
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[2] / 'scripts/test-mast-emulator.sh'


class MastEmulatorTapTests(unittest.TestCase):
    def tap(self, function, xml, text='Remove'):
        # Exercise the actual shell function, including failure propagation before adb.
        body = re.search(r'^' + function + r'\(\) \{\n.*?^\}',
                         SCRIPT.read_text(), re.M | re.S).group()
        return subprocess.run(
            ['bash', '-ec', '''
fail() { echo "FAIL: $*" >&2; }
fake_adb() { echo "ADB: $*"; }
sleep() { :; }
ADB=fake_adb
UI=$1
''' + body + '\n' + function + ' "$2"', 'test', xml, text],
            capture_output=True, text=True)

    def test_tap_text_missing_node_fails_without_traceback_or_adb(self):
        result = self.tap('tap_text', '<hierarchy/>')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('no node for Remove', result.stderr)
        self.assertIn('cannot tap Remove', result.stderr)
        self.assertNotIn('Traceback', result.stderr)
        self.assertNotIn('ADB:', result.stdout)

    def test_tap_switch_missing_node_fails_without_traceback_or_adb(self):
        result = self.tap('tap_switch', '<hierarchy><node checkable="false"/></hierarchy>')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('no checkable node for switch', result.stderr)
        self.assertIn('cannot tap switch', result.stderr)
        self.assertNotIn('Traceback', result.stderr)
        self.assertNotIn('ADB:', result.stdout)

    def test_tap_text_prefers_clickable_exact_match(self):
        result = self.tap('tap_text', '''<hierarchy>
<node text="Remove later" clickable="true" bounds="[0,0][2,2]"/>
<node text="Remove" bounds="[0,0][4,4]"/>
<node content-desc="Remove" clickable="true" bounds="[10,20][30,40]"/>
</hierarchy>''')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('ADB: shell input tap 20 30', result.stdout)

    def test_tap_text_falls_back_to_nonclickable_label(self):
        result = self.tap('tap_text', '<hierarchy><node text="Remove" bounds="[2,4][6,8]"/></hierarchy>')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('ADB: shell input tap 4 6', result.stdout)

    def test_tap_switch_uses_first_checkable_node(self):
        result = self.tap('tap_switch', '''<hierarchy>
<node checkable="false" bounds="[0,0][2,2]"/>
<node checkable="true" bounds="[10,20][30,40]"/>
</hierarchy>''')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('ADB: shell input tap 20 30', result.stdout)


class MastEmulatorModeTests(unittest.TestCase):
    def run_mode(self, mode=None, failed=0):
        source = SCRIPT.read_text()
        selection = source[source.index('MAST_MODE=${'):source.index('REPO=')]
        start = source.index('if [ "$MAST_MODE" = full ]; then')
        end = source.index('echo "=== App limits from the signed v3 policy ==="')
        routing = source[start:end]
        # Stub only scenario bodies, retaining the real mode guards and the focused
        # release commands. No docker/adb/SDK is used by these fast routing tests.
        full_start = routing.index('ui blocked-light')
        full_end = routing.index('else\n# The active pact')
        full_body = routing[full_start:full_end]
        for section in ('Same-key address repair', 'Released on the sail page',
                        'A new pact binds again', 'Router unreachable',
                        'Removal after a fresh signed release'):
            self.assertIn('=== ' + section, full_body)
        routing = routing[:full_start] + 'echo FULL_SCENARIOS\n' + routing[full_end:]
        daily = source[source.index('# Daily total used up'):]
        daily_start = daily.index('OTHER=""')
        daily_end = daily.index('\nfi\n\necho ""')
        daily = daily[:daily_start] + 'echo FULL_DAILY_AND_REMOVAL\n' + daily[daily_end:]
        # All policy and RB1 assertions must remain outside the full-only guards.
        shared = source[end:source.index('# Daily total used up')]
        self.assertIn('=== A reboot mid-day debits 15 seconds', shared)
        self.assertIn('assert debit(after) - debit(before) == 15000', shared)
        self.assertIn('# Sampling runs every 15s', shared)
        self.assertNotIn('MAST_MODE', shared)
        script = selection + '''
FAILED=0
INJECT_FAILURE=$1
ROUTER=disposable-test-router
docker() { echo "RELEASE: $*"; }
broadcast() { echo "SYNC: $*"; }
has() { return 1; }
pass() { :; }
fail() { FAILED=1; }
''' + routing + '''echo SHARED_POLICY_AND_RB1
if [ "$INJECT_FAILURE" = 1 ]; then fail "shared regression"; fi
''' + daily
        env = dict(os.environ)
        env.pop('MAST_MODE', None)
        if mode is not None:
            env['MAST_MODE'] = mode
        return subprocess.run(['bash', '-ec', script, 'test', str(failed)],
                              env=env, capture_output=True, text=True)

    def test_default_and_explicit_full_keep_all_scenarios(self):
        for mode in (None, 'full'):
            with self.subTest(mode=mode):
                result = self.run_mode(mode)
                self.assertEqual(result.returncode, 0, result.stderr)
                for event in ('FULL_SCENARIOS', 'SHARED_POLICY_AND_RB1',
                              'FULL_DAILY_AND_REMOVAL', 'All emulator checks passed!'):
                    self.assertIn(event, result.stdout)
                self.assertNotIn('RELEASE:', result.stdout)

    def test_focused_runs_release_prerequisite_and_scoped_checks_only(self):
        result = self.run_mode('focused')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('RELEASE: exec disposable-test-router', result.stdout)
        self.assertIn('familywarden tick', result.stdout)
        self.assertIn('SYNC:', result.stdout)
        self.assertIn('SHARED_POLICY_AND_RB1', result.stdout)
        self.assertIn('scoped evidence only; NOT the full installation/release gate', result.stdout)
        self.assertNotIn('FULL_SCENARIOS', result.stdout)
        self.assertNotIn('FULL_DAILY_AND_REMOVAL', result.stdout)
        self.assertNotIn('All emulator checks passed!', result.stdout)

    def test_failure_propagates_in_both_modes_without_success(self):
        for mode in ('full', 'focused'):
            with self.subTest(mode=mode):
                result = self.run_mode(mode, failed=1)
                self.assertEqual(result.returncode, 1)
                self.assertIn('FAILED', result.stdout)
                self.assertNotIn('checks passed', result.stdout)

    def test_invalid_mode_fails_before_argument_checks_or_provisioning(self):
        result = subprocess.run(['bash', str(SCRIPT)],
                                env=dict(os.environ, MAST_MODE='invalid'),
                                capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Invalid MAST_MODE', result.stderr)
        self.assertNotIn('router apk dir', result.stderr)
        self.assertNotIn('=== Router', result.stdout)


class MastRebootProgressTests(unittest.TestCase):
    def check_progress(self, **changes):
        before = dict(seq=1, days={'2026-10-30': {'com.example.game': 1000}},
                      gaps=0, reboot_debits={'2026-10-30': {'total': 15000}})
        after = dict(before, seq=2, days={'2026-10-30': {'com.example.game': 2000}})
        after.update(changes)
        source = SCRIPT.read_text()
        block = source[source.index('# Sampling runs every 15s'):source.index('# Daily total used up')]
        with tempfile.TemporaryDirectory() as directory:
            state = Path(directory) / 'reads'
            state.write_text('0\n')
            return subprocess.run(['bash', '-ec', '''
POST_REBOOT=$1; RESULT=$2; STATE=$3
COUNT_TARGET=com.example.game
ADB=fake_adb
sleep() { :; }
pass() { echo PASS; }
fail() { echo FAIL; return 1; }
fake_adb() {
    read -r calls < "$STATE"
    echo "$((calls + 1))" > "$STATE"
    if [ "$calls" -lt 2 ]; then printf '%s' "$POST_REBOOT"; else printf '%s' "$RESULT"; fi
}
''' + block, 'test', json.dumps(before), json.dumps(after), str(state)],
                capture_output=True, text=True)

    def test_waits_past_stale_samples_for_measured_growth(self):
        result = self.check_progress()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('PASS', result.stdout)

    def test_advanced_sequence_without_growth_still_fails(self):
        result = self.check_progress(days={'2026-10-30': {'com.example.game': 1000}})
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Before:', result.stdout)
        self.assertIn('After:', result.stdout)

    def test_growth_with_gap_or_duplicate_debit_still_fails(self):
        for change in [dict(gaps=1), dict(reboot_debits={'2026-10-30': {'total': 30000}})]:
            with self.subTest(change=change):
                self.assertNotEqual(self.check_progress(**change).returncode, 0)


if __name__ == '__main__':
    unittest.main()
