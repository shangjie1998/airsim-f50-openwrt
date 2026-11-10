---
name: unisoc-bt-enable-openwrt
description: 展锐/Unisoc 方案（WCN combo，如 marlin3）的 OpenWrt 设备硬件带蓝牙但系统无任何蓝牙功能时，如何从驱动到 hci0 上电并做成开机自启
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [openwrt, 蓝牙, btattach, h4, bluez, rfkill, sprd, 嵌入式]
---
## 适用场景
OpenWrt 设备（展锐 WCN combo）硬件有蓝牙，但系统里既没有 BlueZ 也没有 hci 设备，需要点亮并能扫描、配对、连接。

## 关键做法
先确认内核已内置 HCI UART：`zcat /proc/config.gz | grep CONFIG_BT_HCIUART`（应为 y 且 H4=y），`cat /proc/tty/ldiscs` 里有 `n_hci`。厂商 tty 驱动（如 `sprdbt_tty.ko`）常在固件 `/lib/modules/$(uname -r)/` 里只是没加载，`modprobe` 后出现 `/dev/ttyBT0`（同时会多出一个 bluetooth 类 rfkill 节点）。用户态用 apk 装 `bluez-libs bluez-utils bluez-utils-extra bluez-daemon dbus`，然后 `btattach -B /dev/ttyBT0 -S 115200 -P h4` 拉出 hci0，把 `/sys/class/rfkill/` 下所有 `type=bluetooth` 的节点 `soft` 写 0，再 `hciconfig hci0 up`，最后起 dbus 与 bluetoothd。自启做成 procd 服务：脚本内 modprobe → 等 `/dev/ttyBT0` → 解 rfkill → 后台等 hci0 出现再 up → `exec btattach` 常驻；START 取 60（dbus）与 62（bluetoothd）之间的 61，bluetoothd 侧在 `main.conf` 开 `AutoEnable=true`。

## 踩过的坑
- 对 tty 手动 `TIOCSETD n_hci` 会成功，但随后 HCIUARTSETPROTO 返回 EPROTONOSUPPORT（协议号逐个试遍均失败），而同一设备 `btattach` 能成功——attach 一律交给 btattach，不要自写 ioctl。
- rfkill 有两个蓝牙节点（驱动自身的 + hci0 的）。只解第一个时 `hciconfig up` 报 `Can't init device hci0: No error information (132)`，解完 hci0 自己的那个才真 RUNNING。
- bluetoothd 早于 dbus 就绪启动会 `Unable to get on D-Bus`；dbus 已在跑但 bluetoothd 先于策略加载启动会 `not allowed to own the service "org.bluez"`，重启 dbus 再起 bluetoothd 即可。
- 固件 `/etc/apk/world` 钉的 kernel 版本可能与实际 `uname -r` 不一致，装 BlueZ 会按依赖拉进一批不匹配的 `kmod-*`（落到别的 `/lib/modules/<ver>/`），运行时无效，别去 modprobe。
- `hcitool lescan` 在部分固件上报 `Set scan parameters failed: I/O error`，改用 `bluetoothctl --timeout N scan le` 可扫到。

## 可复用的产物或模板
```
#!/bin/sh
modprobe sprdbt_tty 2>/dev/null
i=0; while [ ! -e /dev/ttyBT0 ] && [ $i -lt 30 ]; do sleep 1; i=$((i+1)); done
[ -e /dev/ttyBT0 ] || exit 1
unblock(){ for f in /sys/class/rfkill/rfkill*; do
  [ "$(cat $f/type 2>/dev/null)" = bluetooth ] && echo 0 > $f/soft 2>/dev/null; done; }
unblock
( i=0; while [ $i -lt 60 ]; do unblock
    [ -d /sys/class/bluetooth/hci0 ] && hciconfig hci0 up >/dev/null 2>&1 && break
    sleep 1; i=$((i+1)); done ) &
exec /usr/bin/btattach -B /dev/ttyBT0 -S 115200 -P h4
```
procd 侧：`START=61`、`USE_PROCD=1`、`procd_set_param command /usr/sbin/sprd-bt-attach.sh`、`respawn 3600 5 0`。