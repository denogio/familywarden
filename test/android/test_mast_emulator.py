"""Fast tests of the Mast shell UI selectors; no adb, SDK or emulator required."""
from pathlib import Path
import re
import subprocess
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


if __name__ == '__main__':
    unittest.main()
