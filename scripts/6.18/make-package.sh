#!/bin/sh
# 打 F50 的覆盖式整包：dist/airsim-f50-<VERSION>.tar.gz
#
# 用法：
#   sh scripts/6.18/make-package.sh          # 用 out/f50 里编好的二进制
#   sh scripts/6.18/make-package.sh --build  # 先重编再打包
#
# 包结构（和 0.1.0.6 一致，其余机器上的安装脚本只认这个形状）：
#   airsim-f50-<VER>/
#     install.sh  uninstall.sh  VERSION  MANIFEST
#     airsim-agent  airsim-mu300d
#     audio/airsim-audio-retry.init
#     files/...            <- overlay/f50/files 整棵树（rpcd 后端、页面、菜单、ACL、init.d、uci）
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
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

# overlay/f50/files 整棵树照抄，权限按「有 shebang 就 755」处理
rsync -a "$HERE/overlay/f50/files/" "$STAGE/files/"
find "$STAGE/files" -type f | while read -r f; do
	head -c 2 "$f" | grep -q '^#!' && chmod 755 "$f" || chmod 644 "$f"
done

[ -f "$HERE/scripts/6.18/airsim-audio-retry.init" ] && cp -f "$HERE/scripts/6.18/airsim-audio-retry.init" "$STAGE/audio/"

cp -f "$HERE/scripts/6.18/pkg/install.sh" "$HERE/scripts/6.18/pkg/uninstall.sh" "$STAGE/"
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
	printf 'kernel_required=6.18.60（5.4 线已冻结，本包不适用）\n'
	printf 'audio_modules=/mnt/mu300-disk/extra/airsim-audio/modules/6.18.60/\n'
	printf 'scope=脚本+页面+二进制，不改内核、不改启动镜像\n'
	printf 'dual_sim=双卡逻辑全留，按 sms_sims 数组长度>1 自动显示；F50 单卡自动隐藏\n'
	printf 'install_note=覆盖 /usr/lib/airsim、/usr/libexec/rpcd、/www/luci-static/resources/view/airsim、/etc，装前备份到 /usr/lib/airsim/backup-<VER>\n'
} > "$STAGE/MANIFEST"

tar czf "$OUT" -C "$HERE/dist/stage" "$NAME"
rm -rf "$HERE/dist/stage"
cd "$HERE/dist" && shasum -a 256 "$NAME.tar.gz" | awk '{print $1"  "$2}' | tee "$NAME.tar.gz.sha256"
echo "== 打好 =="
ls -l "$OUT"
