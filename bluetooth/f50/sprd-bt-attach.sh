#!/bin/sh
# 展锐 WCN 蓝牙（sprdbt_tty + H4 over /dev/ttyBT0）
# 由 procd 拉起，btattach 常驻；退出后由 procd 自动重启

# 1) 加载 tty 驱动，等待设备节点出现
modprobe sprdbt_tty 2>/dev/null
i=0
while [ ! -e /dev/ttyBT0 ] && [ "$i" -lt 30 ]; do
	sleep 1
	i=$((i + 1))
done

if [ ! -e /dev/ttyBT0 ]; then
	echo "sprd-bt: /dev/ttyBT0 未出现，放弃"
	exit 1
fi

# 2) 解除所有 bluetooth 类 rfkill 软屏蔽
unblock_bt() {
	for f in /sys/class/rfkill/rfkill*; do
		[ "$(cat "$f/type" 2>/dev/null)" = "bluetooth" ] && echo 0 > "$f/soft" 2>/dev/null
	done
}
unblock_bt

# 3) 后台等待 hci0 注册完成后再解除它的 rfkill 并上电
(
	i=0
	while [ "$i" -lt 60 ]; do
		unblock_bt
		if [ -d /sys/class/bluetooth/hci0 ]; then
			hciconfig hci0 up >/dev/null 2>&1 && break
		fi
		sleep 1
		i=$((i + 1))
	done
) &

# 4) 常驻 H4 附着进程（内核已内置 CONFIG_BT_HCIUART=y / H4）
exec /usr/bin/btattach -B /dev/ttyBT0 -S 115200 -P h4
