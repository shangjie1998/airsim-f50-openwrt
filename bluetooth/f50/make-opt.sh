#!/bin/sh
# 打「蓝牙可选件」包：dist/airsim-f50-bt-opt-<ver>-<kver>.tar.gz
#
# 可选件单独一条版本线（bt-opt），跟主包版本无关；包名里带内核版本，
# 因为驱动模块跟内核必须严格对上，别的设备/别的内核不能混用。
#
# 用法：sh bluetooth/f50/make-opt.sh [版本号，默认读 bluetooth/f50/BT-OPT-VERSION]
set -eu

REPO_DIR=$(cd "$(dirname "$0")/../.." && pwd)
OWN="$REPO_DIR/bluetooth/f50"
STAGE_ROOT="$OWN/payload/root"
KVER=${BT_OPT_KVER:-6.18.60}
VER=${1:-$(cat "$OWN/BT-OPT-VERSION" 2>/dev/null || echo 1.0.0)}

[ -d "$STAGE_ROOT" ] || { echo "先跑 fetch-payload.sh 拉载荷" >&2; exit 1; }

OUT="$REPO_DIR/dist/airsim-f50-bt-opt-${VER}-k${KVER}.tar.gz"
BUILD=$(mktemp -d)
trap 'rm -rf "$BUILD"' EXIT

PKG="$BUILD/airsim-f50-bt-opt"
mkdir -p "$PKG"
cp -R "$STAGE_ROOT" "$PKG/root"
install -m 0755 "$OWN/install-bt.sh" "$PKG/install-bt.sh"
echo "$KVER" > "$PKG/KERNEL"
printf '%s\n' "$VER" > "$PKG/BT-OPT-VERSION"

# 清单：每行 sha256 + 路径，安装端可选核对
( cd "$PKG" && find root -type f | sort | while read -r f; do
	printf '%s  %s\n' "$(sha256sum "$f" | cut -d' ' -f1)" "$f"
done ) > "$PKG/manifest.sha256"

# 包本身的大小/条目（写进包内 README，方便对着 Release 说明核）
SIZE=$(du -sk "$PKG" | awk '{print $1}')
cat > "$PKG/README.txt" <<EOF
AirSIM for F50 · 蓝牙可选件 bt-opt ${VER}（内核 ${KVER}）

这是可选件，不是主包。主包不带蓝牙驱动与 bluez，机器上没装过的话菜单里
不会有蓝牙页；装了才会出现。

装法（设备上，联网）：
    wget -O /tmp/btopt.tar.gz <Release 附件地址>
    tar xzf /tmp/btopt.tar.gz -C /tmp && sh /tmp/airsim-f50-bt-opt/install-bt.sh

装法（离线，先在电脑上下好再传上去）：同上，只是第一步换成 scp。

包内：root/ 下按设备绝对路径摆放，install-bt.sh 负责自检与铺文件。
本包只对内核 $KVER 有效，驱动模块跟内核必须严丝合缝。
约 ${SIZE}KB（未压缩）。

本包不做的事：
  * 「已配对设备」只读，不在网页里发起配对/删除（留给下一版）
  * 不提供音频协议栈（A2DP/HFP）：不能拿耳机放歌
EOF

tar czf "$OUT" -C "$BUILD" airsim-f50-bt-opt
echo "== 打好: $OUT"
echo "== 大小: $(du -h "$OUT" | cut -f1)  条目: $(tar tzf "$OUT" | wc -l | tr -d ' ')"
echo "== sha256: $(shasum -a 256 "$OUT" | cut -d' ' -f1)"
