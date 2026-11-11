#!/bin/sh
# 合成 src/ + overlay/f50/ 并交叉编出两个 arm64 二进制。
# 版本号取仓库根的 VERSION（0.1.0.N-r23-6.18），不再用上游 src/plugin/VERSION，
# 页面「当前安装」与 Release tag 才对得上。
# 用法: sh overlay/f50/build.sh [输出目录，默认 ./out/f50]
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO=$(CDPATH= cd -- "$HERE/../.." && pwd)
OUT=${1:-$REPO/out/f50}
# 输出目录必须是绝对路径：后面会 cd 进源码树再 go build，
# 相对路径会跟着 cwd 跑，产物落到 out/build-f50/plugin/out/f50 里，校验步骤找不到。
case "$OUT" in /*) ;; *) OUT=$(pwd)/$OUT ;; esac
BUILD=$REPO/out/build-f50
SDK=${GO_SDK:-$HOME/Documents/doubaokaifagongju/go/go-sdk/go}

[ -x "$SDK/bin/go" ] || { echo "找不到 Go SDK: $SDK（用 GO_SDK=... 指定）" >&2; exit 1; }
VER=$(cat "$REPO/VERSION")
AGENTVER=$(cat "$REPO/AGENT-VERSION" 2>/dev/null || echo "0.4.7-mu300.$VER")

rm -rf "$BUILD" && mkdir -p "$BUILD"
rsync -a "$REPO/src/" "$BUILD/"
# 补丁体检：能不能干净打上、上游是不是已经自己做了。打不上由 apply.py 硬失败，
# 这里提前把表打出来，方便一眼看到哪个补丁要人修。
python3 "$REPO/build/tools/check-patches.py" || echo "(补丁体检有提示，见上；不影响继续构建)" >&2
python3 "$HERE/apply.py" "$BUILD"

export GOROOT="$SDK"; export PATH="$SDK/bin:$PATH"; export GOFLAGS=-mod=vendor
export CGO_ENABLED=0 GOOS=linux GOARCH=arm64

mkdir -p "$OUT"
cd "$BUILD/AirSIM/module/module-agent"
go build -buildvcs=false -trimpath -ldflags="-s -w -X main.agentVersion=$AGENTVER" -o "$OUT/airsim-agent" .

cd "$BUILD/plugin"
go build -buildvcs=false -trimpath -ldflags="-s -w -X main.version=$(cat "$REPO/VERSION")" -o "$OUT/airsim-mu300d" ./cmd/airsim-mu300d

echo "== 编好 =="
for f in airsim-mu300d airsim-agent; do
	[ -x "$OUT/$f" ] || { echo "缺产物: $OUT/$f" >&2; exit 1; }
done
ls -l "$OUT"
file "$OUT/airsim-mu300d" | cut -c1-60
