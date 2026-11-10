#!/bin/sh
# AirSIM for F50 —— 6.18 线覆盖式更新包安装脚本
#
# 本包只做覆盖更新：先备份当前二进制与页面，再铺 overlay/f50 那棵树。
# 不写内核、不改 boot、不动分区。
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

# 刷机后 /usr/lib/airsim 是空的，这时候按首次安装走：不备份、直接铺。
# 有旧插件时才是覆盖更新，先备份再铺。
FRESH=0
if [ ! -f /usr/lib/airsim/airsim-mu300d ]; then
	FRESH=1
	mkdir -p /usr/lib/airsim
	echo "  未检测到已装插件（刷机后？）：按首次安装处理，不备份"
fi
[ -d "$AUD" ] || echo "  注意：$AUD 不存在，语音会不可用（先装 6.18 音频模块）" >&2

# --- 备份：二进制、页面、菜单/ACL、rpcd 后端、init.d 与 uci ---
if [ "$FRESH" = 0 ]; then
mkdir -p "$BK/view" "$BK/rpcd" "$BK/acl" "$BK/initd" "$BK/config"
for f in /usr/lib/airsim/airsim-mu300d /usr/lib/airsim/airsim-agent /usr/lib/airsim/VERSION; do
	[ -f "$f" ] && cp -p "$f" "$BK/" || true
done
cp -p /www/luci-static/resources/view/airsim/*.js /www/luci-static/resources/view/airsim/*.css "$BK/view/" 2>/dev/null || true
cp -p /usr/libexec/rpcd/airsim* "$BK/rpcd/" 2>/dev/null || true
cp -p /usr/share/rpcd/acl.d/luci-app-airsim*.json "$BK/acl/" 2>/dev/null || true
cp -p /usr/share/luci/menu.d/luci-app-airsim*.json "$BK/view/" 2>/dev/null || true
for f in airsim-call-forward airsim-audio-retry; do
	[ -f "/etc/init.d/$f" ] && cp -p "/etc/init.d/$f" "$BK/initd/" || true
done
for f in /etc/config/airsim /etc/config/airsim-call-forward /etc/config/f50-bt; do
	[ -f "$f" ] && cp -p "$f" "$BK/config/" || true
done
fi

# --- 停服务 ---
/etc/init.d/airsim stop >/dev/null 2>&1 || true
/etc/init.d/airsim-call-forward stop >/dev/null 2>&1 || true

# --- 铺二进制 ---
cp -f "$HERE/airsim-mu300d" "$HERE/airsim-agent" /usr/lib/airsim/
chmod 755 /usr/lib/airsim/airsim-mu300d /usr/lib/airsim/airsim-agent
cp -f "$HERE/VERSION" /usr/lib/airsim/VERSION
cp -f "$HERE/MANIFEST" /usr/lib/airsim/MANIFEST

# --- 铺 overlay 树（files/ 就是设备绝对路径的镜像）---
( cd "$HERE/files" && tar cf - . ) | ( cd / && tar xf - )

# 老菜单文件（已合并成一份）留着会让同一页出现两次，删掉
rm -f /usr/share/luci/menu.d/luci-app-airsim-maintenance.json \
      /usr/share/luci/menu.d/luci-app-airsim-pages.json 2>/dev/null || true

chmod 755 /usr/libexec/rpcd/airsim-backup /usr/libexec/rpcd/airsim-bt \
          /usr/libexec/rpcd/airsim-pair /usr/libexec/rpcd/airsim-sms \
          /usr/libexec/rpcd/airsim-call 2>/dev/null || true
chmod 755 /usr/lib/airsim/call-forward /etc/init.d/airsim-call-forward 2>/dev/null || true

# --- 开机自启 ---
# 蓝牙默认可见：main.conf 里两个超时置 0，否则 180 秒后手机就搜不到了。
# 只在文件存在时改，别的机型没蓝牙配置也不报错。
if [ -f /etc/bluetooth/main.conf ]; then
	sed -i 's/^#DiscoverableTimeout = 0/DiscoverableTimeout = 0/' /etc/bluetooth/main.conf 2>/dev/null || true
	sed -i 's/^#PairableTimeout = 0/PairableTimeout = 0/' /etc/bluetooth/main.conf 2>/dev/null || true

fi
/etc/init.d/airsim-call-forward enable >/dev/null 2>&1 || true
# 短信池路径对齐：厂商池在 /etc/mu300/sms，上游 agent 写死读 /etc/mu300/sms-pool/msg，
# 不接上的话 agent 一直报「native SMS inbox unavailable」，手机端读不到短信。
/etc/init.d/airsim-sms-pool-link enable >/dev/null 2>&1 || true
/etc/init.d/airsim-sms-pool-link start >/dev/null 2>&1 || true

# 蓝牙开机恢复：开关只在 rpcd 里被调用，重启后没人拉起 → hci0 一直不存在、页面显示「未挂载」。
# 这个脚本按 /etc/config/f50-bt 的 enabled 在开机时补这一脚；值为 0 时什么都不做。
/etc/init.d/airsim-bt-boot enable >/dev/null 2>&1 || true

# 蓝牙页只在真装了蓝牙栈的机器上露头：btattach 与 bluetoothd 都在才算有。
# 没装的话把菜单、视图、acl 一起撤掉 —— 页面列表里根本不出现「蓝牙」这一项。
# 菜单项本身依赖 luci-app-airsim-bt 这个 acl：acl 一撤，菜单和 tab 就自动不出现，
# 所以这里只需要撤 acl 和视图，不用去改菜单文件。
if ! command -v btattach >/dev/null 2>&1 || ! command -v bluetoothd >/dev/null 2>&1; then
	rm -f /usr/share/rpcd/acl.d/luci-app-airsim-bt.json
	rm -f /www/luci-static/resources/view/airsim/bluetooth.js
fi

# 音频保留页抢占重试（独立 rc 脚本，不覆盖上游的 airsim-audio-reserve）。
# kano 那套 6.18 内核关了 COMPACTION / CONTIG_ALLOC，audio_reserve 只会「一次性」抢页，
# 抢不全整条音频链就不启动；这个脚本在开机 S11 再轮询几轮。
# 别家的包（内核开了迁移或用 DT 预留该段）不需要它，装了也只是空转。
if [ -f "$HERE/audio/airsim-audio-retry.init" ]; then
	cp -f "$HERE/audio/airsim-audio-retry.init" /etc/init.d/airsim-audio-retry
	chmod 755 /etc/init.d/airsim-audio-retry
	/etc/init.d/airsim-audio-retry enable >/dev/null 2>&1 || true
	echo "  已装 /etc/init.d/airsim-audio-retry（S11 抢占重试，日志 /tmp/airsim-audio-claim.log）"
fi

# rpcd 认新后端要重启；重启会踢掉 LuCI 会话，所以顺带重启 uhttpd
/etc/init.d/rpcd restart >/dev/null 2>&1 || true
/etc/init.d/uhttpd restart >/dev/null 2>&1 || true

# 上游默认是 enabled=0（装完得自己开）。首次安装时直接打开，
# 覆盖更新不动用户已经选好的状态。
if [ "$FRESH" = 1 ] && [ -f /etc/config/airsim ]; then
	uci -q set airsim.main.enabled=1 && uci -q commit airsim || true
fi
/etc/init.d/airsim enable >/dev/null 2>&1 || true

/etc/init.d/airsim start >/dev/null 2>&1 || /etc/init.d/airsim restart >/dev/null 2>&1 || true
[ "$(uci get airsim-call-forward.main.enabled 2>/dev/null || echo 0)" = 1 ] && /etc/init.d/airsim-call-forward start >/dev/null 2>&1 || true

if [ "$FRESH" = 1 ]; then
	echo "  首次安装完成（没有备份可列）"
else
cat <<EOF
  备份在 $BK
  验证：
    /etc/init.d/airsim status                     # running
    cat /sys/module/audio_reserve/parameters/missing   # 必须 0
    ubus call airsim-call call_status             # service_running
    wget -qO- http://127.0.0.1:8575/api/status | grep phone_number
  装完 LuCI 请开新标签页重登（rpcd 重启会把旧会话踢掉）。
EOF
fi
