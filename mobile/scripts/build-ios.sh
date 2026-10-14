#!/bin/sh
set -eu
mobile_dir=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$mobile_dir"
if [ "$(uname -s)" != Darwin ]; then
    printf 'iOS bindings require macOS with Xcode and its iOS SDKs.\n' >&2
    exit 1
fi
xcrun --sdk iphoneos --show-sdk-path >/dev/null
xcrun --sdk iphonesimulator --show-sdk-path >/dev/null
mkdir -p build/tools
GOBIN="$mobile_dir/build/tools" go tool gomobile init
PATH="$mobile_dir/build/tools:$PATH" go tool gomobile bind \
    -target=ios,iossimulator -prefix=PKUDisk -trimpath \
    -o "$mobile_dir/build/PKUDiskGUI.xcframework" ./bridge
