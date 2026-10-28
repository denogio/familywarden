#!/bin/bash
# Render Compose previews (DEBUG-only PreviewActivity, fictional fixtures) on a
# disposable Android 15 emulator and save one screenshot per preview and variant.
# No pairing, router, Device Owner or physical device: for design comparison only.
#
#   ./scripts/capture-android-previews.sh <out-dir> <PreviewFile.Composable> ...
#   e.g. FamilyWardenPreviewsKt.ChildTodayPreview
#
# Variants: light, dark, and Danish at 200 % font (VARIANTS overrides).
set -euo pipefail
REPO=$(cd "$(dirname "$0")/.." && pwd)
OUT=$(mkdir -p "${1:?out dir}" && cd "$1" && pwd); shift
APK=$(realpath "${APK:-$REPO/android/app/build/outputs/apk/debug/app-debug.apk}")
VARIANTS=${VARIANTS:-"light dark da200"}
SDK_IMAGE=ghcr.io/cirruslabs/android-sdk:35
EMU=fw-previews-$$
PKG=io.github.denogio.familywarden
cleanup() { docker rm -f "$EMU" >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker run -d --name "$EMU" --network none --device /dev/kvm --memory=4608m --memory-swap=4608m --cpus=2 \
    -v mast-sdk:/opt/extra -v "$APK:/app.apk:ro" "$SDK_IMAGE" sleep infinity >/dev/null
docker exec "$EMU" sh -c "
    export ANDROID_HOME=/opt/extra ANDROID_SDK_ROOT=/opt/extra
    mkdir -p ~/.android/avd/t.avd
    printf 'avd.ini.encoding=UTF-8\npath=/root/.android/avd/t.avd\ntarget=android-35\n' > ~/.android/avd/t.ini
    printf '%s\n' 'image.sysdir.1=system-images/android-35/google_apis/x86_64/' 'abi.type=x86_64' \
        'hw.cpu.arch=x86_64' 'tag.id=google_apis' 'hw.ramSize=2048' 'disk.dataPartition.size=4G' \
        'hw.lcd.width=1080' 'hw.lcd.height=2400' 'hw.lcd.density=420' > ~/.android/avd/t.avd/config.ini
    (/opt/extra/emulator/emulator -avd t -no-window -no-audio -no-snapshot -no-boot-anim \
        -gpu swangle_indirect -accel on -memory 2048 -feature -ModemSimulator >/tmp/emu.log 2>&1 &)"
ADB="docker exec $EMU /opt/extra/platform-tools/adb"
for _ in $(seq 1 120); do
    [ "$($ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = 1 ] && break
    sleep 3
done
[ "$($ADB shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = 1 ] || { echo "emulator did not boot"; exit 1; }
sleep 10
$ADB install /app.apk >/dev/null
$ADB shell settings put global window_animation_scale 0
$ADB shell settings put global transition_animation_scale 0
$ADB shell input keyevent KEYCODE_WAKEUP
$ADB shell wm dismiss-keyguard
for variant in $VARIANTS; do
    case "$variant" in
        light) night=no; font=1.0; locale=en-GB ;;
        dark) night=yes; font=1.0; locale=en-GB ;;
        da200) night=no; font=2.0; locale=da-DK ;;
        *) echo "unknown variant $variant"; exit 1 ;;
    esac
    $ADB shell cmd uimode night "$night" >/dev/null
    $ADB shell settings put system font_scale "$font"
    $ADB shell cmd locale set-app-locales "$PKG" --locales "$locale" >/dev/null
    sleep 3
    for preview in "$@"; do
        $ADB shell am start -W -f 0x10008000 -n "$PKG/androidx.compose.ui.tooling.PreviewActivity" \
            --es composable "io.github.denogio.familywarden.ui.$preview" >/dev/null
        sleep 4
        $ADB exec-out screencap -p > "$OUT/${preview##*.}-$variant.png"
    done
done
ls "$OUT"
