#!/bin/sh
# 打 6.18 音频模块可选件（audio-opt）：
#   dist/airsim-f50-audio-opt-<ver>-k<kver>.tar.gz
#   dist/airsim-f50-audio-opt-<ver>-k<kver>.tar.gz.sha256
#
# 跟蓝牙 opt 一样是独立版本线、固定 tag 拉取；主包不带这些 .ko。
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO=$(CDPATH= cd -- "$HERE/../.." && pwd)
KVER=6.18.60
VER=$(cat "$HERE/AUDIO-OPT-VERSION")
NAME=airsim-f50-audio-opt-$VER-k$KVER
OUT=$REPO/dist

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
ROOT=$STAGE/$NAME
mkdir -p "$ROOT/modules/$KVER"

cp "$HERE/opt/install.sh" "$ROOT/install.sh"
chmod 755 "$ROOT/install.sh"
echo "$VER" > "$ROOT/VERSION"
cp "$HERE/payload/$KVER"/*.ko "$ROOT/modules/$KVER"/

n=$(ls "$ROOT/modules/$KVER"/*.ko | wc -l | tr -d ' ')
[ "$n" = 25 ] || { echo "payload 里 .ko 数是 $n，不是 25" >&2; exit 1; }

{
	echo "name: airsim-f50-audio-opt"
	echo "version: $VER"
	echo "arch: aarch64"
	echo "kernel: $KVER"
	echo "modules: $n"
} > "$ROOT/MANIFEST"

mkdir -p "$OUT"
tar -C "$STAGE" -czf "$OUT/$NAME.tar.gz" "$NAME"
( cd "$OUT" && sha256sum "$NAME.tar.gz" > "$NAME.tar.gz.sha256" )

echo "== 打好 =="
ls -l "$OUT/$NAME.tar.gz"
cat "$OUT/$NAME.tar.gz.sha256"
