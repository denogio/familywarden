"""Fast tests of the Mast shell UI selectors; no adb, SDK or emulator required."""
from pathlib import Path
import json
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
