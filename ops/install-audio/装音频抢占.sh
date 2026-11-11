#!/bin/sh
# Install the audio-page claim retry as its own rc script on the F50 (6.18 line).
# Usage: sh 装音频抢占.sh [/path/to/airsim-audio-retry.init]
set -e
SRC="${1:-./airsim-audio-retry.init}"
DST=/etc/init.d/airsim-audio-retry
[ -f "$SRC" ] || { echo "没有找到 $SRC"; exit 1; }
cp "$SRC" "$DST"
chmod 755 "$DST"
"$DST" enable
echo "已安装 $DST 并在 S11 启用"
echo "试跑一次：$DST start"
echo "日志：/tmp/airsim-audio-claim.log"
