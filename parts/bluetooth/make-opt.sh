#!/bin/sh
# 打「蓝牙可选件」包：dist/airsim-f50-bt-opt-<ver>-<kver>.tar.gz
#
# 可选件单独一条版本线（bt-opt），跟主包版本无关；包名里带内核版本，
# 因为驱动模块跟内核必须严格对上，别的设备/别的内核不能混用。
#
# 用法：sh parts/bluetooth/make-opt.sh [版本号，默认读 parts/bluetooth/BT-OPT-VERSION]
set -eu

REPO_DIR=$(cd "$(dirname "$0")/../.." && pwd)
OWN="$REPO_DIR/parts/bluetooth"
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

# 固定所有文件时间戳再用 gzip -n 压缩：同样的内容每次打出来 sha 一样，
# 这样 rpcd/安装器里钉的 sha256 才是稳定的（否则一重打就对不上）。
find "$BUILD" -exec touch -t 202601010000 {} + 2>/dev/null || true
tar cf - -C "$BUILD" airsim-f50-bt-opt | gzip -n -9 > "$OUT"
echo "== 打好: $OUT"
echo "== 大小: $(du -h "$OUT" | cut -f1)  条目: $(tar tzf "$OUT" | wc -l | tr -d ' ')"
SUM=$(shasum -a 256 "$OUT" | cut -d' ' -f1)
echo "== sha256: $SUM"

# 守卫：rpcd 与安装器里钉的 tag/文件名必须跟刚打出来的这个包一致。
# 完整性不钉 tarball 的 sha（tar 每次打的 sha 会变），改由安装器逐文件核对包内
# manifest.sha256 —— 那个只跟内容有关。
R="$REPO_DIR/overlay/f50/files/usr/libexec/rpcd/airsim-bt"
I="$REPO_DIR/overlay/f50/files/usr/libexec/airsim-btopt-install"
bad=0
grep -q "bt-opt-v$VER" "$R" || { echo "!! rpcd 里的 tag 不是 bt-opt-v$VER" >&2; bad=1; }
# 文件名现在按内核现拼（airsim-f50-bt-opt-$BT_OPT_VER-k$kver.tar.gz），不再有字面全名可对，
# 只校"前缀对不对"和"版本号跟本次 VER 一致"。
grep -q 'airsim-f50-bt-opt-' "$R" || { echo "!! rpcd 里没有 airsim-f50-bt-opt- 前缀" >&2; bad=1; }
grep -q "BT_OPT_VER=$VER\b" "$R" || { echo "!! rpcd 里的 BT_OPT_VER 不是 $VER" >&2; bad=1; }
grep -q "bt-opt-v$VER" "$I" || { echo "!! 安装器里的 tag 不是 bt-opt-v$VER" >&2; bad=1; }
grep -q "airsim-f50-bt-opt-$VER-k" "$I" || { echo "!! 安装器里的文件名模板不是 $VER" >&2; bad=1; }
[ "$bad" = 0 ] || { echo "== 守卫拦住：先把 rpcd / 安装器的 pin 改到这次的包里再发" >&2; exit 2; }
echo "== 守卫通过：rpcd 与安装器的 pin 与本次包一致"
