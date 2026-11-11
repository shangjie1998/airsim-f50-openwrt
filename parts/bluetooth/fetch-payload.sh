#!/bin/sh
# 从真机把「蓝牙可选件」需要的文件拉到本地 payload/root/ 下。
#
# 为什么要从设备拉：bluez / dbus 那套是 apk 包 (bluez-daemon/libs/utils/utils-extra + dbus)，
# 上游 OpenWrt 的包内容随版本变，直接照设备现存文件取，装回去跟当前机器一字不差。
#
# 用法：F50PW=xxx sh parts/bluetooth/fetch-payload.sh [设备IP，默认 192.168.77.1]
set -eu

REPO_DIR=$(cd "$(dirname "$0")/../.." && pwd)
IP=${1:-192.168.77.1}
PW=${F50PW:?需要 F50PW 环境变量（设备 root 密码）}
STAGE="$REPO_DIR/parts/bluetooth/payload/root"
TMP_LOCAL=/tmp/f50-bt-payload.tar

KO_VER=6.18.60

# 设备上要取的文件（按 bluez-daemon/libs/utils/utils-extra + dbus + libdbus 的包清单挑的）
FILES="
lib/modules/$KO_VER/sprdbt_tty.ko
lib/modules/$KO_VER/wcn_bsp.ko
etc/bluetooth/input.conf
etc/bluetooth/main.conf
etc/bluetooth/network.conf
etc/dbus-1/system.d/bluetooth.conf
etc/capabilities/dbus.json
etc/init.d/bluetoothd
etc/init.d/dbus
usr/bin/bluetoothctl
usr/bin/bluetoothd
usr/bin/obexd
usr/bin/btattach
usr/bin/hciconfig
usr/bin/hcitool
usr/bin/btmgmt
usr/bin/hcidump
usr/bin/rfcomm
usr/bin/sdptool
usr/bin/gatttool
usr/bin/dbus-daemon
usr/bin/dbus-uuidgen
usr/bin/dbus-cleanup-sockets
usr/bin/dbus-launch
usr/bin/dbus-launch.real
usr/lib/dbus-daemon-launch-helper
usr/lib/libbluetooth.so.3
usr/lib/libbluetooth.so.3.19.15
usr/lib/libdbus-1.so.3
usr/lib/libdbus-1.so.3.38.3
usr/share/dbus-1/system.conf
usr/share/dbus-1/session.conf
"

echo "== [1/3] 设备上打包载荷（${IP}）"
export F50PW
export PATH="$PATH:/opt/homebrew/bin"
LIST=$(echo "$FILES" | tr '\n' ' ')
expect /tmp/f50ssh.exp "cd / && tar cf /tmp/bt-payload.tar $LIST 2>/dev/null; echo 打包条目: \$(tar tf /tmp/bt-payload.tar | wc -l)" 2>&1 | grep -vE 'Warning|password|spawn' | tail -2

echo "== [2/3] 取回本地"
# 取回用反向 scp（正向那个脚本只会本机→设备）
GET=/tmp/f50get.exp
if [ ! -f "$GET" ]; then
cat > "$GET" <<'EOF'
#!/usr/bin/expect -f
set timeout 300
set src [lindex $argv 0]
set dst [lindex $argv 1]
spawn scp -O -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o PubkeyAuthentication=no root@192.168.77.1:$src $dst
expect {
  "*assword:" { send "$env(F50PW)\r"; exp_continue }
  "*(yes/no)?" { send "yes\r"; exp_continue }
  eof
}
EOF
chmod +x "$GET"
fi
expect "$GET" /tmp/bt-payload.tar "$TMP_LOCAL" 2>&1 | grep -vE 'Warning|password|spawn' | tail -1

echo "== [3/3] 解到 payload/root/"
mkdir -p "$STAGE"
tar xf "$TMP_LOCAL" -C "$STAGE"

# 我们自己的脚本/页面/接口从仓库拷进去（不从设备拿，避免设备上是旧版）
OWN="$REPO_DIR/parts/bluetooth"
OVL="$REPO_DIR/overlay/f50/files"
# macOS 的 install 没有 -D，就 mkdir -p 再 cp 再 chmod
put() { mkdir -p "$(dirname "$2")"; cp -f "$1" "$2"; chmod "$3" "$2"; }
put "$OWN/sprd-bt-attach.sh"  "$STAGE/usr/sbin/sprd-bt-attach.sh"  0755
put "$OWN/bt-switch.sh"       "$STAGE/usr/sbin/f50-bt-switch.sh"   0755
put "$OWN/sprdbluetooth.init" "$STAGE/etc/init.d/sprdbluetooth"    0755
put "$OVL/etc/init.d/airsim-bt-boot" "$STAGE/etc/init.d/airsim-bt-boot" 0755
put "$OVL/usr/libexec/rpcd/airsim-bt" "$STAGE/usr/libexec/rpcd/airsim-bt" 0755
put "$OVL/usr/share/rpcd/acl.d/luci-app-airsim-bt.json" "$STAGE/usr/share/rpcd/acl.d/luci-app-airsim-bt.json" 0644
put "$OVL/www/luci-static/resources/view/airsim/bluetooth.js" "$STAGE/www/luci-static/resources/view/airsim/bluetooth.js" 0644

echo "== 载荷文件数: $(find "$STAGE" -type f | wc -l | tr -d ' ')，大小: $(du -sh "$STAGE" | cut -f1)"
echo "== 完"
