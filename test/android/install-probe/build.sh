#!/bin/sh
# Run inside ghcr.io/cirruslabs/android-sdk:35 with the repository at /src,
# writable scratch directory at /out, and mast-android-debugkey at /root/.android.
set -eu
sdk=/opt/android-sdk-linux
tools=$sdk/build-tools/35.0.0
android=$sdk/platforms/android-35/android.jar
mkdir -p /out/java /out/dex
javac -source 17 -target 17 -classpath "$android" -d /out/java /src/test/android/install-probe/*.java
"$tools/d8" --lib "$android" --min-api 30 --output /out/dex /out/java/io/github/denogio/familywarden/fixture/*.class
for version in 1 2; do
    "$tools/aapt" package -f -M /src/test/android/install-probe/AndroidManifest.xml -I "$android" --version-code "$version" -F /out/unsigned.apk
    (cd /out/dex && zip -q -j /out/unsigned.apk classes.dex)
    "$tools/zipalign" -f 4 /out/unsigned.apk /out/aligned.apk
    "$tools/apksigner" sign --ks /root/.android/debug.keystore --ks-pass pass:android --key-pass pass:android --out "/out/fixture-$version.apk" /out/aligned.apk
done
