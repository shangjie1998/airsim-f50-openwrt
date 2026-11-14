#!/usr/bin/env sh
# 回滚到安装本包之前的状态
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
VER=$(cat "$HERE/VERSION")
BK=/usr/lib/airsim/backup-$VER
[ -d "$BK" ] || { echo "没有备份 $BK" >&2; exit 1; }

/etc/init.d/airsim-call-forward stop >/dev/null 2>&1 || true
/etc/init.d/airsim stop >/dev/null 2>&1 || true

for f in airsim-mu300d airsim-agent VERSION; do
	[ -f "$BK/$f" ] && cp -p "$BK/$f" "/usr/lib/airsim/$f" || true
done
rm -f /usr/lib/airsim/MANIFEST

# 页面：备份里有的还原，本包新加的直接删掉
for f in "$BK"/view/*; do
	[ -f "$f" ] || continue
	b=$(basename "$f")
	case "$b" in
	*.json) cp -p "$f" "/usr/share/luci/menu.d/$b" 2>/dev/null || true ;;
	*)      cp -p "$f" "/www/luci-static/resources/view/airsim/$b" || true ;;
	esac
done
for f in /www/luci-static/resources/view/airsim/call.js \
         /www/luci-static/resources/view/airsim/maintenance.js \
         /www/luci-static/resources/view/airsim/settings.js \
         /www/luci-static/resources/view/airsim/connect.js \
         /www/luci-static/resources/view/airsim/ui.css \
         /www/luci-static/resources/view/airsim/ui.js; do
	[ -f "$BK/view/$(basename "$f")" ] || rm -f "$f"
done
[ -f "$BK/view/luci-app-airsim.json" ] && cp -p "$BK/view/luci-app-airsim.json" /usr/share/luci/menu.d/

# rpcd 后端：还原旧文件，删掉本包新加的
for f in "$BK"/rpcd/*; do
	[ -f "$f" ] && cp -p "$f" "/usr/libexec/rpcd/$(basename "$f")" || true
done
for f in airsim-backup airsim-bt airsim-pair airsim-sms airsim-call; do
	[ -f "$BK/rpcd/$f" ] || rm -f "/usr/libexec/rpcd/$f"
done

# ACL：备份里有就还原，新加的删掉
for f in "$BK"/acl/*; do
	[ -f "$f" ] && cp -p "$f" "/usr/share/rpcd/acl.d/$(basename "$f")" || true
done
for f in luci-app-airsim-backup.json luci-app-airsim-bt.json luci-app-airsim-pair.json \
         luci-app-airsim-sms.json luci-app-airsim-call.json luci-app-airsim-f50-extra.json; do
	[ -f "$BK/acl/$f" ] || rm -f "/usr/share/rpcd/acl.d/$f"
done

# 来电转发：撤自启、撤脚本、撤 uci
/etc/init.d/airsim-call-forward disable >/dev/null 2>&1 || true
rm -f /etc/init.d/airsim-call-forward /usr/lib/airsim/call-forward
[ -f "$BK/config/airsim-call-forward" ] && cp -p "$BK/config/airsim-call-forward" /etc/config/airsim-call-forward || rm -f /etc/config/airsim-call-forward

# 本包新加的抢占重试脚本一并撤掉
/etc/init.d/airsim-audio-retry disable >/dev/null 2>&1 || true
rm -f /etc/init.d/airsim-audio-retry /tmp/airsim-audio-claim.log

# 厂商 CLI 兼容层：把原版放回去，别让卸载留下我们自己的 shim
#   /usr/bin/mu300-at          —— 固件里原本是 symlink（指向 /opt/mu300/bin/mu300-at）
#   /opt/mu300/bin/mu300-sms   —— 固件里是实体脚本，装包时备份成 .fw-real
#   /opt/mu300/lib/run-dir.sh  —— 固件里本来没有，是我们补的
if [ -f "$BK/mu300-at.symlink" ]; then
	ln -sf "$(cat "$BK/mu300-at.symlink")" /usr/bin/mu300-at 2>/dev/null || true
elif [ -f "$BK/mu300-at" ]; then
	cp -p "$BK/mu300-at" /usr/bin/mu300-at 2>/dev/null || true
else
	rm -f /usr/bin/mu300-at
fi
if [ -f /opt/mu300/bin/mu300-sms.fw-real ]; then
	mv -f /opt/mu300/bin/mu300-sms.fw-real /opt/mu300/bin/mu300-sms 2>/dev/null || true
else
	rm -f /opt/mu300/bin/mu300-sms
fi
if [ -f /opt/mu300/lib/run-dir.sh.fw-real ]; then
	mv -f /opt/mu300/lib/run-dir.sh.fw-real /opt/mu300/lib/run-dir.sh 2>/dev/null || true
else
	rm -f /opt/mu300/lib/run-dir.sh
fi

/etc/init.d/rpcd restart >/dev/null 2>&1 || true
/etc/init.d/uhttpd restart >/dev/null 2>&1 || true
/etc/init.d/airsim start >/dev/null 2>&1 || true
echo "已回滚到装 $VER 之前的状态（备份仍在 $BK）"
