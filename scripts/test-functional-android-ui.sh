#!/bin/bash
# Fictional DEBUG previews only: real native history UI, voyage states and role isolation.
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
SERIAL=${1:-emulator-5558}
[[ "$SERIAL" == emulator-* ]] || exit 2
OUT=${OUT:-$(mktemp -d /tmp/fw-functional-ui-XXXXXX)}
mkdir -p "$OUT"
ADB=(adb -s "$SERIAL")
PKG=io.github.denogio.familywarden
"${ADB[@]}" install -r "$ROOT/android/app/build/outputs/apk/debug/app-debug.apk" >/dev/null
FONT=$("${ADB[@]}" shell settings get system font_scale | tr -d '\r')
NIGHT=$("${ADB[@]}" shell cmd uimode night | tr -d '\r' | awk '{print $NF}')
LOCALE=$("${ADB[@]}" shell cmd locale get-app-locales "$PKG" | tr -d '\r' | grep -o '\[.*\]' | tr -d '[]')
DENSITY=$("${ADB[@]}" shell wm density | tr -d '\r' | awk '/density:/{v=$NF} END{print v}')
cleanup() {
 "${ADB[@]}" shell settings put system font_scale "$FONT" >/dev/null
 "${ADB[@]}" shell cmd uimode night "$NIGHT" >/dev/null
 "${ADB[@]}" shell cmd locale set-app-locales "$PKG" --locales "$LOCALE" >/dev/null
 "${ADB[@]}" shell input keyevent KEYCODE_HOME >/dev/null
}
trap cleanup EXIT
capture() {
 sleep 2
 for _ in 1 2 3 4 5 6; do
  "${ADB[@]}" shell rm -f /sdcard/fw-functional.xml
  "${ADB[@]}" shell uiautomator dump /sdcard/fw-functional.xml >/dev/null 2>&1 || true
  "${ADB[@]}" exec-out cat /sdcard/fw-functional.xml > "$OUT/$1.xml" 2>/dev/null || true
  grep -q '<hierarchy' "$OUT/$1.xml" && break
  sleep 2
 done
 "${ADB[@]}" exec-out screencap -p > "$OUT/$1.png"
}
render() {
 "${ADB[@]}" shell am start -f 0x10008000 -n "$PKG/androidx.compose.ui.tooling.PreviewActivity" \
  --es composable "io.github.denogio.familywarden.ui.FunctionalPreviewsKt.$1" >/dev/null
 capture "$2"
}
contains() { grep -Fq "$2" "$OUT/$1.xml" || { echo "Missing '$2' in $1"; exit 1; }; }
absent() { if grep -Fq "$2" "$OUT/$1.xml"; then echo "Unexpected '$2' in $1"; exit 1; fi; }
tap() {
 read -r x y < <(python3 - "$OUT/$1.xml" "$2" <<'PY'
import re,sys,xml.etree.ElementTree as E
nodes=[n for n in E.parse(sys.argv[1]).iter('node') if sys.argv[2] in (n.get('text'),n.get('content-desc'))]
assert nodes,sys.argv[2]
n=next((n for n in nodes if n.get('clickable')=='true'),nodes[0])
a,b,c,d=map(int,re.findall(r'\d+',n.get('bounds')));print((a+c)//2,(b+d)//2)
PY
 )
 "${ADB[@]}" shell input tap "$x" "$y"
}
reveal() {
 for _ in 1 2 3 4 5 6 7; do
  capture "$1"
  grep -Fq "$2" "$OUT/$1.xml" && return
  "${ADB[@]}" shell input swipe 400 1500 400 650 350
 done
 contains "$1" "$2"
}
"${ADB[@]}" shell input keyevent KEYCODE_WAKEUP
"${ADB[@]}" shell wm dismiss-keyguard
"${ADB[@]}" shell settings put system font_scale 1.0
"${ADB[@]}" shell cmd uimode night no >/dev/null
"${ADB[@]}" shell cmd locale set-app-locales "$PKG" --locales en-GB >/dev/null
sleep 3
render ParentHistoryPreview parent
tap parent History
capture history-today
contains history-today 'RECORDED SCREEN TIME'
contains history-today 'Alex video'
contains history-today 'Report received:'
tap history-today 'Previous day'
capture history-zero
contains history-zero 'Reported: no counted use'
absent history-zero 'Alex video'
tap history-zero 'Previous day'
capture history-missing
contains history-missing 'No report'
absent history-missing 'Alex video'
tap history-missing 'Previous day'
capture history-error
contains history-error 'History could not be loaded'
absent history-error '0 min'
tap history-error 'Previous day'
capture history-incomplete
contains history-incomplete 'Some time may be missing'
"${ADB[@]}" shell input keyevent KEYCODE_BACK
capture history-back
contains history-back 'Screen time'
render HistoryScopePreview scope
tap scope 'Choose a child'
capture picker
tap picker Sam
capture sam
tap sam History
capture sam-history
contains sam-history 'Sam video'
tap sam-history 'Sync now'
capture revoked
contains revoked 'Alex video'
absent revoked 'Sam video'
render VoyageBoundPreview voyage-start
tap voyage-start Ulysses
capture voyage-bound
contains voyage-bound 'Bound by an ongoing pact'
contains voyage-bound 'Household waiting period: 1 h 30 min'
contains voyage-bound 'Hotspot is allowed'
reveal voyage-port 'PORT CALL'
reveal voyage-router 'Open router administration'
tap voyage-router Family
capture family
absent family 'Bound by an ongoing pact'
absent family 'THE SAVED COURSE'
tap family 'My phone'
capture own-return
contains own-return 'Ulysses'
render VoyageCalmPreview calm-start
tap calm-start Ulysses
capture calm
contains calm 'The sea is calm'
contains calm 'Open router administration'
render VoyageReleasePreview release-start
tap release-start Ulysses
capture release
contains release 'Expected release:'
contains release 'Household waiting period: 1 h 30 min'
render VoyagePendingPreview voyage-pending
contains voyage-pending 'No usable device policy'
absent voyage-pending 'The sea is calm'
absent voyage-pending 'Hotspot is allowed'
absent voyage-pending 'content-desc="Ulysses"'
render VoyageNoCaptainPreview no-captain-start
tap no-captain-start Ulysses
capture no-captain
contains no-captain 'No captain aboard'
absent no-captain 'The sea is calm'
render VoyageMidnightPreview midnight-start
tap midnight-start Ulysses
reveal midnight 'Sunday · 23:00–24:00'
render VoyageUnknownPreview unknown-start
tap unknown-start Ulysses
capture offline
contains offline 'last verified voyage'
reveal unknown-details 'has not supplied the waiting period'
absent unknown-details '24 hours'
"${ADB[@]}" shell cmd uimode night yes >/dev/null
for preview in VoyageBoundPreview VoyageCalmPreview VoyageReleasePreview VoyageUnknownPreview; do
 render "$preview" "dark-$preview-root"
 tap "dark-$preview-root" Ulysses
 capture "dark-$preview"
done
render ParentHistoryPreview dark-parent
tap dark-parent Controls
capture controls
tap controls 'App status and remaining work'
capture capabilities
contains capabilities 'SUPPORTED IN THIS APP'
reveal capability-missing 'NOT YET AVAILABLE IN THIS APP'
contains capability-missing 'Native parent editors'
"${ADB[@]}" shell input keyevent KEYCODE_BACK
"${ADB[@]}" shell cmd locale set-app-locales "$PKG" --locales da-DK >/dev/null
"${ADB[@]}" shell settings put system font_scale 2.0
# Start a fresh preview process after locale/font recreation has settled.
"${ADB[@]}" shell am force-stop "$PKG"
sleep 6
render VoyageBoundPreview da-start
tap da-start Ulysses
capture da-voyage
if ! grep -Fq 'Husstandens ventetid' "$OUT/da-voyage.xml"; then
 tap da-voyage Ulysses
 capture da-voyage
fi
contains da-voyage 'Husstandens ventetid'
python3 "$ROOT/test/android/check_navigation.py" "$OUT/da-voyage.xml" --selected Ulysses --density "$DENSITY" --labels Oversigt DNS Ulysses Indstillinger
reveal da-now 'LIGE NU'
render ParentHistoryPreview da-parent
tap da-parent Historik
capture da-history
contains da-history 'Alex video'
python3 "$ROOT/test/android/check_navigation.py" "$OUT/da-history.xml" --selected Skærmtid --density "$DENSITY" --labels Skærmtid Kontrol Godkendelser
tap da-history 'Forrige dag'
capture da-zero
contains da-zero 'intet talt forbrug'
tap da-zero Kontrol
reveal da-controls 'Appstatus og resterende arbejde'
tap da-controls 'Appstatus og resterende arbejde'
capture da-capabilities
contains da-capabilities 'Appstatus og resterende arbejde'
reveal da-missing 'ENDNU IKKE TILGÆNGELIGT I APPEN'
echo "Functional Android UI checks passed: $OUT"
