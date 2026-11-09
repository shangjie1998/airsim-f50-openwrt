#!/bin/sh
# 合成 src/ + overlay/f50/ 并交叉编出两个 arm64 二进制。
# 用法: sh overlay/f50/build.sh [输出目录，默认 ./out/f50]
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO=$(CDPATH= cd -- "$HERE/../.." && pwd)
OUT=${1:-$REPO/out/f50}
BUILD=$REPO/out/build-f50
SDK=${GO_SDK:-$HOME/Documents/doubaokaifagongju/go/go-sdk/go}

[ -x "$SDK/bin/go" ] || { echo "找不到 Go SDK: $SDK（用 GO_SDK=... 指定）" >&2; exit 1; }
VER=$(cat "$REPO/VERSION")
AGENTVER=$(cat "$REPO/AGENT-VERSION" 2>/dev/null || echo "0.4.7-mu300.$VER")

rm -rf "$BUILD" && mkdir -p "$BUILD"
rsync -a "$REPO/src/" "$BUILD/"
python3 "$HERE/apply.py" "$BUILD"

export GOROOT="$SDK"; export PATH="$SDK/bin:$PATH"; export GOFLAGS=-mod=vendor
export CGO_ENABLED=0 GOOS=linux GOARCH=arm64

mkdir -p "$OUT"
cd "$BUILD/AirSIM/module/module-agent"
go build -buildvcs=false -trimpath -ldflags="-s -w -X main.agentVersion=$AGENTVER" -o "$OUT/airsim-agent" .

cd "$BUILD/plugin"
go build -buildvcs=false -trimpath -ldflags="-s -w -X main.version=$(cat "$BUILD/plugin/VERSION")" -o "$OUT/airsim-mu300d" ./cmd/airsim-mu300d

echo "== 编好 =="
ls -l "$OUT"
file "$OUT/airsim-mu300d" | cut -c1-60
