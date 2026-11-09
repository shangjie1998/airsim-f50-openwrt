#!/bin/sh
# 展锐 WCN 蓝牙在 OpenWrt 上的一键部署（仅用于干净固件首次安装）
# 用法：把本包全部文件放到设备同一目录，cd 进去后 sh deploy.sh
set -e

echo "[1/7] 安装 BlueZ 用户态"
apk update
apk add bluez-libs bluez-utils bluez-utils-extra bluez-daemon dbus

echo "[2/7] 部署启动脚本"
cp sprd-bt-attach.sh /usr/sbin/sprd-bt-attach.sh
chmod 755 /usr/sbin/sprd-bt-attach.sh
cp sprdbluetooth.init /etc/init.d/sprdbluetooth
chmod 755 /etc/init.d/sprdbluetooth
cp bt-switch.sh /usr/sbin/f50-bt-switch.sh
chmod 755 /usr/sbin/f50-bt-switch.sh

echo "[3/7] 修改 /etc/bluetooth/main.conf"
sed -i 's/^#AutoEnable=true/AutoEnable=true/' /etc/bluetooth/main.conf
sed -i 's/^#Name = BlueZ/Name = mu300/'      /etc/bluetooth/main.conf

echo "[4/7] 启用开机自启"
/etc/init.d/dbus enable
/etc/init.d/sprdbluetooth enable
/etc/init.d/bluetoothd enable

echo "[5/7] 启动服务（顺序不可乱）"
/etc/init.d/dbus restart
/etc/init.d/sprdbluetooth restart
/etc/init.d/bluetoothd restart

echo "[6/7] 自查"
sleep 6
ls /sys/class/bluetooth || true
hciconfig -a || true
ps | grep -E "btattach|bluetoothd" | grep -v grep || true
echo "完成。若 hci0 未出现，看 logread | grep -i sprd"
echo "开关：/usr/sbin/f50-bt-switch.sh on|off|status"
