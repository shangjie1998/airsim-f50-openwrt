#!/bin/sh
# AirSIM for F50 —— 6.18 线覆盖式更新（r23 插件 + 自编音频模块前提检查）
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
VER=$(cat "$HERE/VERSION")
BK=/usr/lib/airsim/backup-$VER
AUD=/mnt/mu300-disk/extra/airsim-audio/modules/$(uname -r)

echo "== AirSIM for F50 $VER =="

K=$(uname -r)
case "$K" in
6.18.*) : ;;
*)
	if [ "${F50_ALLOW_OTHER_KERNEL:-}" = 1 ]; then
		echo "  警告：当前内核 $K，本包按 6.18 线编的（已强制继续）" >&2
	else
		echo "  当前内核 $K，本包只对 6.18 线有效。" >&2
		echo "  确要强装：F50_ALLOW_OTHER_KERNEL=1 sh install.sh" >&2
		exit 3
	fi
	;;
esac

[ -f /usr/lib/airsim/airsim-mu300d ] || { echo "插件未安装，本包只做覆盖更新" >&2; exit 1; }
[ -d "$AUD" ] || echo "  注意：$AUD 不存在，语音会不可用（先装 6.18 音频模块）" >&2

mkdir -p "$BK"
for f in /usr/lib/airsim/airsim-mu300d /usr/lib/airsim/airsim-agent /usr/lib/airsim/VERSION; do
	[ -f "$f" ] && cp -p "$f" "$BK/" || true
done

/etc/init.d/airsim stop >/dev/null 2>&1 || true

cp -f "$HERE/airsim-mu300d" "$HERE/airsim-agent" /usr/lib/airsim/
chmod 755 /usr/lib/airsim/airsim-mu300d /usr/lib/airsim/airsim-agent
[ -f "$HERE/VERSION" ] && cp -f "$HERE/VERSION" /usr/lib/airsim/VERSION

mkdir -p /usr/share/rpcd/acl.d /www/luci-static/resources/view/airsim
cp -f "$HERE/files/usr/share/rpcd/acl.d/luci-app-airsim.json" /usr/share/rpcd/acl.d/
cp -f "$HERE"/files/www/luci-static/resources/view/airsim/*.js /www/luci-static/resources/view/airsim/

/etc/init.d/airsim start >/dev/null 2>&1 || /etc/init.d/airsim restart >/dev/null 2>&1 || true
echo "  备份在 $BK ；验证：wget -qO- http://127.0.0.1:7581/status"
