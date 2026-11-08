#!/bin/sh
# AirSIM for F50 — 覆盖式更新（只换显示名，不动插件依赖与音频组件）
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
VER=$(cat "$HERE/VERSION")
BK=/usr/lib/airsim/backup-$VER

echo "== AirSIM for F50 $VER =="

# 这个包只对 5.4 内核有效：音频扩展包里的驱动是按 5.4.254-gb50db5b6224c 编的，
# 6.18.x 上插件能装、能跑，但打电话没有声音。要装就装 5.4，别在 6.18 上瞎折腾。
K=$(uname -r)
case "$K" in
5.4.*) : ;;
*)
	if [ "${F50_ALLOW_OTHER_KERNEL:-}" = 1 ]; then
		echo "  警告：当前内核 $K，本包只支持 5.4.254-gb50db5b6224c，语音会不可用（已强制继续）" >&2
	else
		echo "  当前内核 $K，本包只支持 5.4.254-gb50db5b6224c（语音驱动只编了这个版本）。" >&2
		echo "  先回 5.4：mu300-update rollback-boot  然后重启；确要强装：F50_ALLOW_OTHER_KERNEL=1 sh install.sh" >&2
		exit 3
	fi
	;;
esac

[ -f /usr/lib/airsim/airsim-mu300d ] || { echo "插件未安装，本包只做覆盖更新" >&2; exit 1; }

mkdir -p "$BK"
for f in /usr/lib/airsim/airsim-mu300d /usr/lib/airsim/airsim-agent /usr/lib/airsim/VERSION; do
	[ -f "$f" ] && cp -p "$f" "$BK/" || true
done
for j in overview.js manage.js sms.js; do
	src=/www/luci-static/resources/view/airsim/$j
	[ -f "$src" ] && cp -p "$src" "$BK/$j" || true
done
[ -f /usr/share/rpcd/acl.d/luci-app-airsim.json ] && cp -p /usr/share/rpcd/acl.d/luci-app-airsim.json "$BK/" || true
echo "  备份 -> $BK"

/etc/init.d/airsim stop >/dev/null 2>&1 || true
sleep 1
killall airsim-mu300d >/dev/null 2>&1 || true
sleep 1

cp "$HERE/airsim-mu300d" /usr/lib/airsim/airsim-mu300d
cp "$HERE/airsim-agent"  /usr/lib/airsim/airsim-agent
chmod 755 /usr/lib/airsim/airsim-mu300d /usr/lib/airsim/airsim-agent
printf '0.1.0-r22\n' > /usr/lib/airsim/VERSION
cp "$HERE/MANIFEST" /usr/lib/airsim/MANIFEST
cp "$HERE"/files/www/luci-static/resources/view/airsim/*.js /www/luci-static/resources/view/airsim/
cp "$HERE"/files/usr/share/rpcd/acl.d/luci-app-airsim.json /usr/share/rpcd/acl.d/luci-app-airsim.json
rm -f /tmp/luci-indexcache* /tmp/luci-modulecache/* 2>/dev/null || true

/etc/init.d/airsim start
/etc/init.d/rpcd restart >/dev/null 2>&1 || true
sleep 2

echo "== 校验 =="
echo -n "  进程: "; ps | grep -v grep | grep -q airsim-mu300d && echo 在跑 || echo 没起来
echo -n "  /status product: "; uclient-fetch -qO- http://127.0.0.1:8575/status 2>/dev/null | tr ',' '\n' | sed -n 's/.*"product": *"\([^"]*\)".*/\1/p' | head -1
echo "  MANIFEST: $(head -1 /usr/lib/airsim/MANIFEST)"
echo "  已配对客户端请删除设备后重新配对，才会看到新名字 AirSIM F50"
