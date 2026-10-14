#!/bin/sh
set -eu
mobile_dir=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
cd "$mobile_dir"
sdk=$1
ndk_version=$2
output=$3
abis=$4
targets=
for abi in $(printf '%s' "$abis" | tr ',' ' '); do
    case "$abi" in
        arm64-v8a) target=android/arm64 ;;
        x86_64) target=android/amd64 ;;
        *) printf 'Unsupported Android ABI: %s\n' "$abi" >&2; exit 1 ;;
    esac
    targets="${targets:+$targets,}$target"
done
mkdir -p "$mobile_dir/build/tools" "$(dirname "$output")"
GOBIN="$mobile_dir/build/tools" go tool gomobile init
PATH="$mobile_dir/build/tools:$PATH" ANDROID_HOME="$sdk" ANDROID_NDK_HOME="$sdk/ndk/$ndk_version" \
    go tool gomobile bind -target="$targets" -androidapi=29 \
    -javapkg=su.zzi.pkudiskfuse.mobile -trimpath -ldflags='-s -w' \
    -o "$output" ./bridge
