#!/bin/sh
# F50 蓝牙总开关 —— 给 LuCI 页面 / rpcd / SSH 的稳定口子
#
#   f50-bt-switch.sh on      开：写 enabled=1，按序拉起 dbus → sprdbluetooth → bluetoothd
#   f50-bt-switch.sh off     关：写 enabled=0，停 bluetoothd、sprdbluetooth（hci0 消失）
#   f50-bt-switch.sh status  打印 uci 开关值 + hci0 是否存在
#
# 持久化：状态写在 /etc/config/f50-bt，开机时 /etc/init.d/sprdbluetooth 会读它，
# 值为 0 就直接跳过启动 —— 即「关了以后重启仍然是关」。
# LuCI 侧调用方式（页面 JS 里）：
#   fs.exec('/usr/sbin/f50-bt-switch.sh', ['on' | 'off'])
# 不需要新增 rpcd 白名单。
set -eu

CFG=/etc/config/f50-bt
SVC=sprdbluetooth

ensure_cfg() {
	[ -f "$CFG" ] || cat > "$CFG" <<'CONF'
config f50bt 'settings'
	option enabled '1'
CONF
}

do_on() {
	ensure_cfg
	uci -q set f50-bt.settings.enabled='1'
	uci -q commit f50-bt
	/etc/init.d/dbus start >/dev/null 2>&1 || true
	/etc/init.d/$SVC restart >/dev/null 2>&1 || true
	/etc/init.d/bluetoothd start >/dev/null 2>&1 || true
	sleep 3
	[ -d /sys/class/bluetooth/hci0 ] && echo "bt: on (hci0 已出现)" || echo "bt: on（hci0 未出现，看 logread | grep -i sprd）"
}

do_off() {
	ensure_cfg
	uci -q set f50-bt.settings.enabled='0'
	uci -q commit f50-bt
	/etc/init.d/bluetoothd stop >/dev/null 2>&1 || true
	/etc/init.d/$SVC stop >/dev/null 2>&1 || true
	echo "bt: off（重启后仍为关）"
}

do_status() {
	enabled=$(uci -q get f50-bt.settings.enabled || echo 'unset')
	hci=no
	[ -d /sys/class/bluetooth/hci0 ] && hci=yes
	echo "enabled=$enabled hci0=$hci"
}

case "${1:-status}" in
	on) do_on ;;
	off) do_off ;;
	status) do_status ;;
	*) echo 'Usage: f50-bt-switch.sh on|off|status' >&2; exit 2 ;;
esac
