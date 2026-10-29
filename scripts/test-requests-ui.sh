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
sleep 2
capture() {
    sleep 2
    # UiAutomator can return a null root while a preview is still starting: retry.
    for _ in 1 2 3 4 5 6; do
        "${ADB[@]}" shell rm -f /sdcard/fw-requests-ui.xml
        "${ADB[@]}" shell uiautomator dump /sdcard/fw-requests-ui.xml >/dev/null 2>&1 || true
        "${ADB[@]}" exec-out cat /sdcard/fw-requests-ui.xml > "$OUT/$1.xml" 2>/dev/null || true
        grep -q '<hierarchy' "$OUT/$1.xml" && break
        sleep 2
    done
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
nodes=[n for n in E.parse(sys.argv[1]).iter('node') if sys.argv[2] in (n.get('text'), n.get('content-desc'))]
assert nodes,sys.argv[2]
node=next((n for n in nodes if n.get('clickable')=='true'), nodes[0])
x1,y1,x2,y2=map(int,re.findall(r'\d+',node.get('bounds')))
print((x1+x2)//2,(y1+y2)//2)
PY
)
    "${ADB[@]}" shell input tap "$x" "$y"
}
check_nav() {
    python3 "$ROOT/test/android/check_navigation.py" "$OUT/$1.xml" --selected "$2" --density "$DENSITY" --labels "${@:3}"
}
check_picker() {
    python3 - "$OUT/$1.xml" "$DENSITY" "$2" <<'PY'
import math,re,sys,xml.etree.ElementTree as E
root=E.parse(sys.argv[1]).getroot()
parents={child:parent for parent in root.iter() for child in parent}
for name in ('Alex','Sam'):
    matches=[n for n in root.iter('node') if n.get('text')==name]
    assert matches, f'Missing child option: {name}'
    target=matches[-1]
    # Android exposes the already-selected radio option as checked, not clickable.
    while target.get('clickable')!='true' and target.get('checked')!='true' and target in parents:
        target=parents[target]
    assert target.get('clickable')=='true' or target.get('checked')=='true', name
    x1,y1,x2,y2=map(int,re.findall(r'\d+',target.get('bounds')))
    minimum=math.ceil(48*int(sys.argv[2])/160)
    assert x2-x1>=minimum and y2-y1>=minimum, f'Small child option: {name}'
    selected=target.get('selected')=='true' or target.get('checked')=='true'
    assert selected==(name==sys.argv[3]), f'Wrong child selection: {name}'
print('PASS accessible child options:', sys.argv[3])
PY
}
reveal() {
    for _ in 1 2 3 4 5 6; do
        capture "$1"
        grep -Fq "text=\"$2\"" "$OUT/$1.xml" && return 0
        "${ADB[@]}" shell input swipe 400 1500 400 700 350
    done
    contains "$1" "$2"
}
render ParentExpiredPreview parent-expired
contains parent-expired 'No requests waiting'
if grep -Fq 'text="Approve"' "$OUT/parent-expired.xml"; then echo 'Expired request remained actionable'; exit 1; fi
render ParentAwaitingPreview parent-awaiting
contains parent-awaiting 'Waiting for phone approval'
if grep -Fq 'No children assigned' "$OUT/parent-awaiting.xml"; then echo 'Pending enrollment reported as an empty scope'; exit 1; fi
render ParentRequestsPreview parent-light
check_nav parent-light Requests 'Screen time' Controls Requests
contains parent-light Alex
if grep -Fq 'Set up device owner' "$OUT/parent-light.xml"; then echo 'Parent-only view requires Device Owner'; exit 1; fi
tap parent-light 'Screen time'
capture parent-screen-time
check_nav parent-screen-time 'Screen time' 'Screen time' Controls Requests
contains parent-screen-time '1 h 22 min'
contains parent-screen-time 'Reported for 2027-01-15'
contains parent-screen-time Video
if grep -Fq 'text="Approve"' "$OUT/parent-screen-time.xml"; then echo 'Approvals leaked into screen time'; exit 1; fi
tap parent-screen-time 'Choose a child'
capture child-picker
check_picker child-picker Alex
tap child-picker Sam
capture parent-sam
contains parent-sam Sam
contains parent-sam 'No usage summary is available'
if grep -Fq '1 h 22 min' "$OUT/parent-sam.xml"; then echo 'Sibling usage leaked into selected child'; exit 1; fi
tap parent-sam Controls
capture parent-settings
contains parent-settings Sam
check_nav parent-settings Controls 'Screen time' Controls Requests
contains parent-settings 'Managed on the router'
tap parent-settings Requests
capture parent-combined-inbox
contains parent-combined-inbox 'Alex · Sam'
tap parent-combined-inbox 'Screen time'
capture parent-selection-retained
contains parent-selection-retained 'No usage summary is available'
if grep -Fq '1 h 22 min' "$OUT/parent-selection-retained.xml"; then echo 'Child selection lost between pages'; exit 1; fi
tap parent-selection-retained Requests
reveal parent-time-request Approve
tap parent-time-request Approve
capture parent-approve-dialog
contains parent-approve-dialog 'Approve extra time'
contains parent-approve-dialog 'Choose 1–30 minutes.'
contains parent-approve-dialog '30 min · asked'
contains parent-approve-dialog 'two hours from approval'
if grep -Fq '1 h · asked' "$OUT/parent-approve-dialog.xml"; then echo 'Approval exceeded requested minutes'; exit 1; fi
tap parent-approve-dialog Cancel
reveal parent-after-dismiss Decline
tap parent-after-dismiss Decline
capture parent-decline-dialog
contains parent-decline-dialog 'Decline this request?'
render ManagedParentPreview managed-parent
tap managed-parent Family
capture managed-family
check_nav managed-family 'Screen time' 'Screen time' Controls Requests
contains managed-family 'My phone'
tap managed-family 'Choose a child'
capture managed-picker
tap managed-picker Sam
capture managed-sam
tap managed-sam 'My phone'
capture managed-own
contains managed-own Overview
tap managed-own Family
capture managed-sam-restored
contains managed-sam-restored 'No usage summary is available'
if grep -Fq '1 h 22 min' "$OUT/managed-sam-restored.xml"; then echo 'Child selection lost across modes'; exit 1; fi
render ParentScopeChangePreview parent-scope
tap parent-scope 'Choose a child'
capture scope-picker
tap scope-picker Sam
capture scope-sam
tap scope-sam 'Sync now'
capture scope-revoked
contains scope-revoked '1 h 22 min'
tap scope-revoked 'Choose a child'
capture scope-current-picker
if grep -Fq 'text="Sam"' "$OUT/scope-current-picker.xml"; then echo 'Revoked child remains selectable'; exit 1; fi
"${ADB[@]}" shell input keyevent KEYCODE_BACK
render ChildRequestPreview child-pending
contains child-pending 'Your request'
contains child-pending 'Waiting'
contains child-pending 'You can have one request open at a time.'
render ChildAskPreview child-today
check_nav child-today Today Today History Settings
tap child-today 'Ask for more time'
capture child-ask-what
contains child-ask-what 'What do you need more time for?'
tap child-ask-what Video
tap child-ask-what Next
capture child-ask-how
contains child-ask-how 'How much more?'
contains child-ask-how 'Add a note (optional)'
tap child-ask-how 'Other amount'
capture child-ask-custom
contains child-ask-custom 'Choose 1–240 minutes.'
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
check_nav parent-dark Requests 'Screen time' Controls Requests
reveal parent-dark-request Approve
contains parent-dark-request Approve
render ChildExtraDarkPreview child-extra-dark
contains child-extra-dark 'About 20 extra minutes left'
"${ADB[@]}" shell cmd locale set-app-locales "$PKG" --locales da-DK >/dev/null
"${ADB[@]}" shell settings put system font_scale 2.0 >/dev/null
sleep 2
render ParentRequestsLargePreview parent-danish-large
contains parent-danish-large Godkendelser
check_nav parent-danish-large Godkendelser Skærmtid Kontrol Godkendelser
# Match the actual action, not the page heading "Godkendelser".
reveal parent-danish-large-scrolled Godkend
tap parent-danish-large-scrolled Godkend
capture parent-danish-approval
contains parent-danish-approval '30 min. · ønsket'
reveal parent-danish-approval-bottom Godkend
contains parent-danish-approval-bottom 'to timer fra godkendelsen'
reveal parent-danish-cancel Annullér
tap parent-danish-cancel Annullér
capture parent-danish-dismissed
tap parent-danish-dismissed Skærmtid
capture parent-danish-selector
check_nav parent-danish-selector Skærmtid Skærmtid Kontrol Godkendelser
tap parent-danish-selector 'Vælg et barn'
capture danish-child-picker
check_picker danish-child-picker Alex
tap danish-child-picker Sam
capture danish-sam
contains danish-sam 'Der er ingen tilgængelig forbrugsoversigt'
printf 'P8 UI checks passed; fictional-fixture captures: %s\n' "$OUT"
