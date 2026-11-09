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
	# 4) 设显示名：F50-<BD Address 后 6 位>。控制器地址每台机器唯一且重启不变，
	#    取 6 位够分辨又不会太长（手机蓝牙列表里 F50-142211 这种）。
	#    读不到地址时退回固定名 F50，不让名字变空。
	if [ -d /sys/class/bluetooth/hci0 ]; then
		_bd=$(hciconfig hci0 2>/dev/null | awk '/BD Address/ {print $3; exit}')
		_tag=$(printf '%s' "$_bd" | tr -d ':' | tr 'a-f' 'A-F')
		_suf=$(printf '%s' "$_tag" | awk 'length($0)>=6 {print substr($0,length($0)-5)}')
		_btname="F50${_suf:+-$_suf}"
		hciconfig hci0 name "$_btname" >/dev/null 2>&1
		echo "sprd-bt: 显示名 $_btname"
	fi
) &

# 5) 常驻 H4 附着进程（内核已内置 CONFIG_BT_HCIUART=y / H4）
exec /usr/bin/btattach -B /dev/ttyBT0 -S 115200 -P h4
