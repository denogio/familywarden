#!/bin/bash
# Test infrastructure only: wait for the disposable Android guest's control route.
set -euo pipefail
EMU=${1:?emulator container}
IP=${2:?router IPv4 address}
PORT=${3:-7700}
[[ "$IP" =~ ^[0-9.]+$ && "$PORT" =~ ^[0-9]+$ ]] || exit 2
ADB=(docker exec "$EMU" /opt/extra/platform-tools/adb -s emulator-5554)
for _ in $(seq 1 30); do
    # Zero-I/O checks connection establishment, unlike echo|nc which can time out
    # waiting for an HTTP response to an incomplete request and falsely say closed.
    if "${ADB[@]}" shell "nc -z -w 2 $IP $PORT" >/dev/null 2>&1; then
        echo "Android guest can reach router control endpoint"
        exit 0
    fi
    sleep 2
done
echo "Android guest cannot reach router control endpoint $IP:$PORT" >&2
"${ADB[@]}" shell cmd wifi status >&2 || true
"${ADB[@]}" shell ip -4 addr >&2 || true
exit 1
