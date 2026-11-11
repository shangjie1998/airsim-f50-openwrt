#!/bin/sh
# AirSIM for F50 —— 覆盖式更新包安装脚本
#
# 本包只做覆盖更新：先备份当前二进制与页面，再铺 overlay/f50 那棵树。
# 不写内核、不改 boot、不动分区。
#
# 内核策略（2026-11 起）：主包不绑内核。
#   两个二进制是 Go 交叉编出来的静态件，与内核版本无关；页面、rpcd、init.d 也都是
#   纯 shell/lua，换内核不影响。唯一跟内核有关的是语音驱动模块，那玩意儿已经拆成
#   独立配件包（按内核分别出），装不装、装哪个版本，由配件包自己判断。
#   所以这里不再拒绝非 6.18 内核，改成「认一认、说清楚、继续装」。
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
VER=$(cat "$HERE/VERSION")
BK=/usr/lib/airsim/backup-$VER
AUD=/mnt/mu300-disk/extra/airsim-audio/modules/$(uname -r)

echo "== AirSIM for F50 $VER =="

# --- 内核识别（不再拦人，只交代清楚）---
# 已知三档：5.4.254（历史线，语音靠厂商镜像自带的驱动，实测可用）、6.18.60、7.3.1。
# 别的内核：照装，只是提醒「没在真机验过」，出问题看自检与日志。
K=$(uname -r)
AUD_VENDOR=0
case "$(uname -m)" in
aarch64) : ;;
*) echo "  当前架构 $(uname -m)，本包只对 aarch64 有效。" >&2; exit 3 ;;
esac
case "$K" in
5.4.*)
	echo "  内核 ${K}：主体可用；语音用这台机器自带的厂商驱动（厂商 5.4 镜像里就是全的）。"
	echo "  真机实测（5.4.254）：/dev/snd 在、两张声卡在、audio_reserve missing=0，不用配件包；"
	echo "  6.18/7.3 那套配件包跟它不通用，别往上装。"
	AUD_SUPPORTED=1
	AUD_VENDOR=1
	;;
6.18.*)
	echo "  内核 ${K}：主体可用，语音取决于音频配件包（audio-opt 1.0.3 支持 6.18.60）"
	AUD_SUPPORTED=1
	;;
7.3.*)
	echo "  内核 ${K}：主体可用，语音取决于音频配件包（audio-opt 1.0.3 支持 7.3.1）"
	AUD_SUPPORTED=1
	;;
*)
	echo "  内核 ${K}：不认识这一版，按通用规则照装（主体与内核无关）。" >&2
	echo "  没在真机验过，装完请跑一遍自检；语音要另外确认有没有对应内核的配件包。" >&2
	AUD_SUPPORTED=1
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
if [ -d "$AUD" ]; then
	echo "  语音模块目录已在：$AUD"
elif [ "$AUD_VENDOR" = 1 ]; then
	echo "  提示：$AUD 不在 —— 5.4 这套语音驱动是厂商镜像自带的，配件包不覆盖它，" >&2
	echo "  要语音得从厂商 5.4 镜像里把 modules 目录补回来（或者切回 6.18/7.3 用配件包）。" >&2
elif [ "$AUD_SUPPORTED" = 1 ]; then
	echo "  提示：$AUD 不存在 —— 语音现在不可用，去维护页「可选配件」装音频配件包（按当前内核出包）" >&2
else
	: # 这个内核本来就没有语音，上面已经交代过了，不重复
fi

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

# --- 配置保护：包里的 /etc/config/* 只当“没有配置时的默认值”，
# 升级场景一律以安装前的备份为准，否则 enabled、relay 地址这些会被默认值盖掉 ---
if [ "$FRESH" = 0 ] && [ -d "$BK/config" ]; then
	for f in "$BK"/config/*; do
		[ -f "$f" ] && cp -p "$f" "/etc/config/$(basename "$f")"
	done
fi

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

# --- 联网（上网通道）能力落地 ---
# 两个脚本随包铺到 /usr/bin/airsim-netmode 与 /usr/libexec/airsim-net-probe。
# 这一段只做三件事：给权限、留一份机器现状快照、处理与作者旧脚本的命名冲突。
# 幂等：文件不在就跳过，跑第二遍不会把东西改坏。
chmod 755 /usr/bin/airsim-netmode /usr/libexec/airsim-net-probe 2>/dev/null || true
mkdir -p /usr/lib/airsim
if [ -x /usr/libexec/airsim-net-probe ]; then
	/usr/libexec/airsim-net-probe -p > "/usr/lib/airsim/netprobe-$(date +%Y%m%d).txt" 2>/dev/null || true
	echo "  已存一份联网能力快照：/usr/lib/airsim/netprobe-$(date +%Y%m%d).txt"
fi
# 作者原版 /usr/bin/netmode 与我们的 airsim-netmode 并存，会出现「谁改的 uci 说不清」。
# 安装时只把它收起来、不删：改名成 netmode.vendor-bak，随时能改回去。
if [ -f /usr/bin/netmode ]; then
	mv -f /usr/bin/netmode /usr/bin/netmode.vendor-bak 2>/dev/null || true
	echo "  已收起作者原版 /usr/bin/netmode → /usr/bin/netmode.vendor-bak（避免两套脚本争状态）"
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
