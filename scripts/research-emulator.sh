#!/bin/sh
# Persistent, memory-capped Android 17 Play emulator for P2 research (emulator-5556).
#
#   scripts/research-emulator.sh start|stop|status
#
# The AVD (including any signed-in TEST account) lives in the docker volume
# fw-p2-avd and survives stop/start: never pass -wipe-data here.
# The emulator raises guest RAM to at least 4096 MB for the android-37 Play
# image ("Increasing RAM size to 4096MB"), whatever -memory or the screen size
# say; 2 GB guests with a 3 GB cap were OOM-killed right after boot. The
# container is hard-capped (no extra swap) so a runaway emulator is OOM-killed
# inside its own cgroup instead of exhausting the host.
# Delete everything with: docker rm -f fw-p2; docker volume rm fw-p2-avd
set -eu

NAME=fw-p2
VOLUME=fw-p2-avd
GUEST_MB=${GUEST_MB:-4096}    # minimum the emulator accepts for this image
CAP=${CAP:-5g}                # guest RAM + emulator/GPU overhead; no extra swap
SDK_IMAGE=ghcr.io/cirruslabs/android-sdk:35

case "${1:-status}" in
start)
    docker volume inspect "$VOLUME" >/dev/null 2>&1 || { echo "missing volume $VOLUME (create the AVD first)"; exit 1; }
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    docker run -d --name "$NAME" --network host --device /dev/kvm \
        --memory "$CAP" --memory-swap "$CAP" \
        -v mast-sdk:/opt/extra -v "$VOLUME:/root/.android/avd" \
        -v "$HOME/.android/adbkey.pub:/host-adbkey.pub:ro" -v "$HOME/.android/adbkey:/host-adbkey:ro" \
        "$SDK_IMAGE" sh -c "
export ANDROID_HOME=/opt/extra ANDROID_SDK_ROOT=/opt/extra
cp /host-adbkey.pub ~/.android/adbkey.pub; cp /host-adbkey ~/.android/adbkey
rm -f ~/.android/avd/p2.avd/*.lock
sed -i 's/^hw.ramSize=.*/hw.ramSize=$GUEST_MB/' ~/.android/avd/p2.avd/config.ini
exec /opt/extra/emulator/emulator -avd p2 -no-window -no-audio -no-snapshot -no-boot-anim \
    -gpu swangle_indirect -accel on -memory $GUEST_MB -cores 2 -port 5556
" >/dev/null
    for _ in $(seq 1 120); do
        [ "$(adb -s emulator-5556 shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = 1 ] && break
        sleep 3
    done
    "$0" status
    ;;
stop)
    # Graceful: let Android flush userdata before the container goes away.
    adb -s emulator-5556 emu kill >/dev/null 2>&1 || true
    for _ in $(seq 1 30); do
        [ "$(docker inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null)" = true ] || break
        sleep 1
    done
    docker rm -f "$NAME" >/dev/null 2>&1 || true
    echo stopped
    ;;
status)
    docker stats --no-stream --format '{{.Name}}: {{.MemUsage}} ({{.MemPerc}})' "$NAME" 2>/dev/null || echo "$NAME not running"
    adb -s emulator-5556 shell getprop sys.boot_completed 2>/dev/null | tr -d '\r' | sed 's/^1$/booted/'
    ;;
*) echo "usage: $0 start|stop|status"; exit 2 ;;
esac
