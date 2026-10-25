#!/bin/bash
# P8 rendering/interaction checks using DEBUG-only previews and fictional families.
# Reuse a running disposable emulator; this script never provisions Device Owner.
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
SERIAL=${1:-emulator-5554}
[[ "$SERIAL" == emulator-* ]] || { echo 'Only emulator serials are permitted'; exit 1; }
OUT=${OUT:-$(mktemp -d /tmp/fw-requests-ui-XXXXXX)}
mkdir -p "$OUT"
ADB=(adb -s "$SERIAL")
PKG=io.github.denogio.familywarden
"${ADB[@]}" install -r "$ROOT/android/app/build/outputs/apk/debug/app-debug.apk" >/dev/null
FONT=$("${ADB[@]}" shell settings get system font_scale | tr -d '\r')
NIGHT=$("${ADB[@]}" shell cmd uimode night | tr -d '\r' | awk '{print $NF}')
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
sleep 2
capture() {
    sleep 2
    "${ADB[@]}" shell rm -f /sdcard/fw-requests-ui.xml
    "${ADB[@]}" shell uiautomator dump /sdcard/fw-requests-ui.xml >/dev/null
    "${ADB[@]}" exec-out cat /sdcard/fw-requests-ui.xml > "$OUT/$1.xml"
    "${ADB[@]}" exec-out screencap -p > "$OUT/$1.png"
}
render() {
    "${ADB[@]}" shell am start -f 0x10008000 -n "$PKG/androidx.compose.ui.tooling.PreviewActivity" \
        --es composable "io.github.denogio.familywarden.ui.RequestPreviewsKt.$1" >/dev/null
    capture "$2"
}
contains() { grep -Fq "$2" "$OUT/$1.xml" || { echo "Missing '$2' in $1"; exit 1; }; }
tap() {
    read -r x y < <(python3 - "$OUT/$1.xml" "$2" <<'PY'
import re,sys,xml.etree.ElementTree as E
nodes=[n for n in E.parse(sys.argv[1]).iter('node') if n.get('text')==sys.argv[2]]
assert nodes,sys.argv[2]
x1,y1,x2,y2=map(int,re.findall(r'\d+',nodes[0].get('bounds')))
print((x1+x2)//2,(y1+y2)//2)
PY
)
    "${ADB[@]}" shell input tap "$x" "$y"
}
render ParentRequestsPreview parent-light
contains parent-light Alex
contains parent-light Approve
contains parent-light Decline
if grep -Fq 'Set up device owner' "$OUT/parent-light.xml"; then echo 'Parent-only view requires Device Owner'; exit 1; fi
tap parent-light Approve
capture parent-approve-dialog
contains parent-approve-dialog 'Approve extra time'
contains parent-approve-dialog 'Choose 1–30 minutes.'
tap parent-approve-dialog Cancel
capture parent-after-dismiss
tap parent-after-dismiss Decline
capture parent-decline-dialog
contains parent-decline-dialog 'Decline this request?'
render ChildRequestPreview child-pending
contains child-pending 'Waiting for a parent'
render ChildRetryPreview child-retry
contains child-retry 'Retry action'
tap child-retry 'Dismiss local retry'
capture child-dismiss-dialog
contains child-dismiss-dialog 'It does not cancel a request'
render ChildExtraPreview child-extra-light
contains child-extra-light 'A little more time'
contains child-extra-light 'About 20 extra minutes left'
"${ADB[@]}" shell cmd uimode night yes >/dev/null
sleep 2
render ParentRequestsDarkPreview parent-dark
contains parent-dark Approve
render ChildExtraDarkPreview child-extra-dark
contains child-extra-dark 'About 20 extra minutes left'
"${ADB[@]}" shell cmd locale set-app-locales "$PKG" --locales da-DK >/dev/null
"${ADB[@]}" shell settings put system font_scale 2.0 >/dev/null
sleep 2
render ParentRequestsLargePreview parent-danish-large
contains parent-danish-large Anmodninger
for _ in 1 2 3 4; do
    capture parent-danish-large-scrolled
    grep -Fq Godkend "$OUT/parent-danish-large-scrolled.xml" && break
    "${ADB[@]}" shell input swipe 400 1500 400 700 350
done
contains parent-danish-large-scrolled Godkend
printf 'P8 UI checks passed; fictional-fixture captures: %s\n' "$OUT"
