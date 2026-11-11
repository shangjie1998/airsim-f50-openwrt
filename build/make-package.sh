#!/bin/sh
# 打 F50 的覆盖式整包：dist/airsim-f50-<VERSION>.tar.gz
#
# 用法：
#   sh build/make-package.sh          # 用 out/f50 里编好的二进制
#   sh build/make-package.sh --build  # 先重编再打包
#
# 包结构（和 0.1.0.6 一致，其余机器上的安装脚本只认这个形状）：
#   airsim-f50-<VER>/
#     install.sh  uninstall.sh  VERSION  MANIFEST
#     airsim-agent  airsim-mu300d
#     audio/airsim-audio-retry.init
#     files/...            <- overlay/f50/files 整棵树（rpcd 后端、页面、菜单、ACL、init.d、uci）
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VER=$(cat "$HERE/VERSION")
AGENT=$(cat "$HERE/AGENT-VERSION")
NAME=airsim-f50-$VER
STAGE=$HERE/dist/stage/$NAME
OUT=$HERE/dist/$NAME.tar.gz

if [ "${1:-}" = "--build" ]; then
	: "${GOROOT:=$HOME/Documents/doubaokaifagongju/go/go-sdk/go}"
	export GOROOT PATH="$GOROOT/bin:$PATH"
	sh "$HERE/overlay/f50/build.sh" >/dev/null
fi

[ -x "$HERE/out/f50/airsim-agent" ] || { echo "没有 out/f50/airsim-agent，先跑 overlay/f50/build.sh" >&2; exit 1; }

rm -rf "$STAGE" && mkdir -p "$STAGE/audio" "$STAGE/files" "$HERE/dist"

cp -f "$HERE/out/f50/airsim-agent" "$HERE/out/f50/airsim-mu300d" "$STAGE/"
chmod 755 "$STAGE/airsim-agent" "$STAGE/airsim-mu300d"
printf '%s\n' "$VER" > "$STAGE/VERSION"

# 上游基础层（ubus 对象 /usr/libexec/rpcd/airsim、/etc/init.d/airsim、菜单与 ACL 骨架）
# 先铺它，再用 overlay/f50/files 覆盖同名文件（我们的页面、菜单合并版、自定义 rpcd）。
# 刷机后设备上只有内核+LuCI，没这层的话 ubus 里连 airsim 对象都没有。
if [ -d "$HERE/src/plugin/packaging/openwrt/root" ]; then
	rsync -a "$HERE/src/plugin/packaging/openwrt/root/" "$STAGE/files/"
else
	echo "  缺少 src/plugin/packaging/openwrt/root，打出来的是纯覆盖包" >&2
fi

# overlay/f50/files 整棵树照抄，权限按「有 shebang 就 755」处理
rsync -a "$HERE/overlay/f50/files/" "$STAGE/files/"
find "$STAGE/files" -type f | while read -r f; do
	head -c 2 "$f" | grep -q '^#!' && chmod 755 "$f" || chmod 644 "$f"
done

[ -f "$HERE/build/pkg/airsim-audio-retry.init" ] && cp -f "$HERE/build/pkg/airsim-audio-retry.init" "$STAGE/audio/"

cp -f "$HERE/build/pkg/install.sh" "$HERE/build/pkg/uninstall.sh" "$STAGE/"
chmod 755 "$STAGE/install.sh" "$STAGE/uninstall.sh"

{
	printf '%s\n' "$VER"
	printf 'kind=overlay\n'
	printf 'target=F50 (MU300 / UMS9620, OpenWrt 25.12.5 aarch64_generic)\n'
	printf 'built=%s\n' "$(date +%Y-%m-%d)"
	printf 'built_by=本机交叉编 darwin/arm64 -> linux/arm64（未用云主机）\n'
	printf 'upstream=kai-wu-cortex/airsim-mu300 @ v0.1.0-r23 (pre-release)\n'
	printf 'agent_version=%s\n' "$AGENT"
	printf 'rebrand=只改显示文案(AirSIM F50)；agent_kind 仍是 mu300\n'
	printf 'kernel=any (aarch64)；主体不绑内核，Go 静态件 + 纯脚本页面\n'
	printf 'kernel_known=5.4.254（主体可跑，语音用厂商自带驱动，实测可用）/ 6.18.60（语音看配件包）/ 7.3.1（语音看配件包）\n'
	printf 'audio_modules=/mnt/mu300-disk/extra/airsim-audio/modules/<uname -r>/（由音频配件包按内核落地，主包不带模块）\n'
	printf 'scope=脚本+页面+二进制，不改内核、不改启动镜像\n'
	printf 'dual_sim=双卡逻辑全留，按 sms_sims 数组长度>1 自动显示；F50 单卡自动隐藏\n'
	printf 'install_note=覆盖 /usr/lib/airsim、/usr/libexec/rpcd、/www/luci-static/resources/view/airsim、/etc，装前备份到 /usr/lib/airsim/backup-<VER>\n'
} > "$STAGE/MANIFEST"

# macOS 的 bsdtar 默认会把扩展属性打成 ._* 影子文件塞进包里，
# 有些设备的安装逻辑按「顶层只有一个目录」判结构，多出来一条 ._ 文件就误判。
# 关掉元数据与 AppleDouble，保证包内只有我们自己的文件。
COPYFILE_DISABLE=1 tar --no-xattrs --no-mac-metadata -czf "$OUT" -C "$HERE/dist/stage" "$NAME"
rm -rf "$HERE/dist/stage"
cd "$HERE/dist" && shasum -a 256 "$NAME.tar.gz" | awk '{print $1"  "$2}' | tee "$NAME.tar.gz.sha256"
echo "== 打好 =="
ls -l "$OUT"
