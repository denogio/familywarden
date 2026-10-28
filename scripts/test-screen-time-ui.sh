#!/bin/bash
# Focused rendering check using Compose's DEBUG-only PreviewActivity and fictional fixtures.
# No pairing, policy writes, Device Owner changes, or physical-device targets.
# Start scripts/research-emulator.sh first. Full enforcement tests remain test-mast-emulator.sh.
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
SERIAL=${1:-emulator-5556}
[[ "$SERIAL" == emulator-* ]] || { echo 'Only emulator serials are permitted'; exit 1; }
OUT=${OUT:-$(mktemp -d /tmp/fw-screen-time-XXXXXX)}
mkdir -p "$OUT"
ADB=(adb -s "$SERIAL")
PKG=io.github.denogio.familywarden
"${ADB[@]}" install -r "$ROOT/android/app/build/outputs/apk/debug/app-debug.apk" >/dev/null
FONT=$("${ADB[@]}" shell settings get system font_scale | tr -d '\r')
NIGHT=$("${ADB[@]}" shell cmd uimode night | tr -d '\r' | awk '{print $NF}')
DENSITY=$("${ADB[@]}" shell wm density | tr -d '\r' | awk '/density:/{v=$NF} END{print v}')
LOCALE=$("${ADB[@]}" shell cmd locale get-app-locales "$PKG" | tr -d '\r' | grep -o '\[.*\]' | tr -d '[]')
cleanup() {
    "${ADB[@]}" shell settings put system font_scale "$FONT" >/dev/null
    "${ADB[@]}" shell cmd uimode night "$NIGHT" >/dev/null
    "${ADB[@]}" shell cmd locale set-app-locales "$PKG" --locales "$LOCALE" >/dev/null
    "${ADB[@]}" shell input keyevent KEYCODE_HOME >/dev/null
}
trap cleanup EXIT
"${ADB[@]}" shell input keyevent KEYCODE_WAKEUP
"${ADB[@]}" shell wm dismiss-keyguard
"${ADB[@]}" shell settings put system font_scale 1.0
"${ADB[@]}" shell cmd uimode night no >/dev/null
"${ADB[@]}" shell cmd locale set-app-locales "$PKG" --locales en-GB >/dev/null
sleep 3 # Let Android finish the locale/configuration restart before opening a preview.
capture() {
    sleep 3
    # UiAutomator can return a null root while a preview is still starting: retry.
    for _ in 1 2 3 4 5 6; do
        "${ADB[@]}" shell rm -f /sdcard/fw-screen-time.xml
        "${ADB[@]}" shell uiautomator dump /sdcard/fw-screen-time.xml >/dev/null 2>&1 || true
        "${ADB[@]}" exec-out cat /sdcard/fw-screen-time.xml > "$OUT/$1.xml" 2>/dev/null || true
        grep -q '<hierarchy' "$OUT/$1.xml" && break
        sleep 2
    done
    "${ADB[@]}" exec-out screencap -p > "$OUT/$1.png"
}
render() {
    "${ADB[@]}" shell am start -f 0x10008000 -n "$PKG/androidx.compose.ui.tooling.PreviewActivity" \
        --es composable "io.github.denogio.familywarden.ui.FamilyWardenPreviewsKt.$1" >/dev/null
    capture "$2"
}
contains() { grep -Fq "$2" "$OUT/$1.xml" || { echo "Missing '$2' in $1"; exit 1; }; }
tap() {
    read -r x y < <(python3 - "$OUT/$1.xml" "$2" <<'PY'
import re, sys, xml.etree.ElementTree as ET
nodes = [n for n in ET.parse(sys.argv[1]).iter('node') if sys.argv[2] in (n.get('text'), n.get('content-desc'))]
if not nodes:
    raise SystemExit('Label not found')
node = next((n for n in nodes if n.get('clickable') == 'true'), nodes[0])
x1, y1, x2, y2 = map(int, re.findall(r'\d+', node.get('bounds')))
print((x1+x2)//2, (y1+y2)//2)
PY
)
    "${ADB[@]}" shell input tap "$x" "$y"
}
check_nav() {
    python3 "$ROOT/test/android/check_navigation.py" "$OUT/$1.xml" --selected "$2" --density "$DENSITY" --labels "${@:3}"
}
render ChildTodayPreview child-light
check_nav child-light Today Today History Settings
contains child-light '38 min'
contains child-light 'School'
tap child-light History
capture child-history
check_nav child-history History Today History Settings
contains child-history 'Recorded screen time on this phone'
contains child-history '82 min'
tap child-history Settings
capture child-settings
check_nav child-settings Settings Today History Settings
contains child-settings 'Control address'
render NearLimitPreview near-limit
contains near-limit '5 min'
contains near-limit 'ALMOST TIME'
render DowntimePreview downtime
contains downtime 'Downtime until'
contains downtime 'Calls and the apps below still work.'
render ChildUnknownPreview unknown
contains unknown 'Not counted right now'
contains unknown 'Allow usage access'
if grep -Fq '38 min' "$OUT/unknown.xml"; then echo 'Unknown usage invented remaining time'; exit 1; fi
"${ADB[@]}" shell cmd uimode night yes >/dev/null
sleep 3
render TimesUpPreview times-up-dark
contains times-up-dark 'Time’s up for today'
contains times-up-dark 'Phone, emergency calls and alarm clock'
render AdultTodayPreview adult-dark
check_nav adult-dark Today Today DNS Ulysses Settings
contains adult-dark 'Ulysses'
"${ADB[@]}" shell cmd locale set-app-locales "$PKG" --locales da-DK >/dev/null
"${ADB[@]}" shell settings put system font_scale 2.0
sleep 3
render ChildLargePreview danish-large
check_nav danish-large 'I dag' 'I dag' Historik Indstillinger
contains danish-large 'TID TILBAGE I DAG'
contains danish-large '38 min.'
contains danish-large 'I dag'
for _ in 1 2 3 4; do "${ADB[@]}" shell input swipe 300 1100 300 400 400; done
capture danish-large-scrolled
contains danish-large-scrolled 'ALTID OK'
echo "Screen-time rendering checks passed. Fictional-fixture screenshots: $OUT"
