#!/bin/sh
# 在设备上安装「蓝牙可选件」。
#
# 用法（设备上）：
#   sh install-bt.sh /path/to/payload-dir          # 目录形态（调试用）
#   tar xzf airsim-f50-bt-opt-*.tar.gz -C /tmp/btopt && sh /tmp/btopt/install-bt.sh
#
# 做的事：自检 → 铺文件 → depmod → 开服务开机自启 → 起蓝牙 → 刷 rpcd/uhttpd
# 不碰：/etc/config/f50-bt（开关状态）、/var/lib/bluetooth（已配对记录）
#
# 退出码：0 成功；1 条件不符；2 铺文件失败
set -eu

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT="$HERE/root"
FORCE=0
[ "${1:-}" = "--force" ] && FORCE=1

say() { echo "[btopt] $*"; }
die() { echo "[btopt] 不符合要求：$*" >&2; exit 1; }

REQ_KVER=$(cat "$HERE/KERNEL" 2>/dev/null || echo 6.18.60)

say "== 1/6 自检"
[ -d /sys/class ] || die "没有 /sys/class"
if ! command -v btattach >/dev/null 2>&1 || [ ! -f /usr/bin/bluetoothd ]; then
	: # 没装过，正常
fi

KVER=$(uname -r)
if [ "$KVER" != "$REQ_KVER" ]; then
	die "内核是 ${KVER}，这个包是给 $REQ_KVER 打的。驱动模块跟内核版本必须对得上，装上去也加载不了。请装对应内核版本的可选件。"
fi

# 控制器硬件/驱动节点：有的机器 DT 里没有这颗 WCN，装了也没用
if [ ! -d /sys/class/bluetooth ] && [ ! -e /dev/ttyBT0 ]; then
	die "本机没有蓝牙控制器节点（/sys/class/bluetooth 与 /dev/ttyBT0 都没有），这颗芯片不存在或驱动不在这份固件里。"
fi

NEED_KB=$(du -sk "$ROOT" | awk '{print $1}')
AVAIL_KB=$(df -k / | awk 'NR==2{print $4}')
[ "$AVAIL_KB" -gt $((NEED_KB + 4096)) ] || die "空间不够：需要约 $((NEED_KB / 1024 + 4))MB，只剩 $((AVAIL_KB / 1024))MB。"

say "== 2/6 铺文件（$(find "$ROOT" -type f | wc -l | tr -d ' ') 个）"
cd "$ROOT"
find . -type f | while read -r f; do
	dst="/${f#./}"
	mkdir -p "$(dirname "$dst")"
	cp -f "$f" "$dst"
done
# 软链：libbluetooth / libdbus 要有 .so.3 名字
cd /
[ -e /usr/lib/libbluetooth.so.3 ] || ln -sf libbluetooth.so.3.19.15 /usr/lib/libbluetooth.so.3
[ -e /usr/lib/libdbus-1.so.3 ] || ln -sf libdbus-1.so.3.38.3 /usr/lib/libdbus-1.so.3

say "== 3/6 驱动模块"
depmod -a "$REQ_KVER" 2>/dev/null || depmod -a 2>/dev/null || true
modprobe wcn_bsp 2>/dev/null || insmod /lib/modules/"$REQ_KVER"/wcn_bsp.ko 2>/dev/null || true
modprobe sprdbt_tty 2>/dev/null || insmod /lib/modules/"$REQ_KVER"/sprdbt_tty.ko || die "sprdbt_tty 加载失败"
[ -e /dev/ttyBT0 ] || say "注意：/dev/ttyBT0 还没出现，attach 时再试"

say "== 4/6 main.conf 规则（超时置 0 / 自动开启）"
M=/etc/bluetooth/main.conf
if [ -f "$M" ]; then
	sed -i 's/^#AutoEnable=true/AutoEnable=true/' "$M" 2>/dev/null || true
	sed -i 's/^AutoEnable=false/AutoEnable=true/' "$M" 2>/dev/null || true
	sed -i 's/^DiscoverableTimeout = .*/DiscoverableTimeout = 0/' "$M" 2>/dev/null || true
	sed -i 's/^#DiscoverableTimeout = .*/DiscoverableTimeout = 0/' "$M" 2>/dev/null || true
	sed -i 's/^PairableTimeout = .*/PairableTimeout = 0/' "$M" 2>/dev/null || true
	sed -i 's/^#PairableTimeout = .*/PairableTimeout = 0/' "$M" 2>/dev/null || true
fi

say "== 5/6 开服务 + 开机自启"
chmod 0755 /etc/init.d/bluetoothd /etc/init.d/dbus /etc/init.d/sprdbluetooth /etc/init.d/airsim-bt-boot 2>/dev/null || true
chmod 0755 /usr/sbin/sprd-bt-attach.sh /usr/sbin/f50-bt-switch.sh 2>/dev/null || true
# 注意：/usr/libexec/rpcd/airsim-bt、bluetooth.js、luci-app-airsim-bt.json 都属于主包，
# 可选件不再带、也不再 chmod。以前带了一份，装一次就把主包装的盖成旧版
# （页面点按钮没反应、acl 被改成 600 页面直接 403），这就是 v1.0.3 重打的原因。
/etc/init.d/dbus enable >/dev/null 2>&1 || true
/etc/init.d/bluetoothd enable >/dev/null 2>&1 || true
/etc/init.d/sprdbluetooth enable >/dev/null 2>&1 || true
/etc/init.d/airsim-bt-boot enable >/dev/null 2>&1 || true
/etc/init.d/dbus restart >/dev/null 2>&1 || /etc/init.d/dbus start >/dev/null 2>&1 || true
/etc/init.d/sprdbluetooth restart >/dev/null 2>&1 || /etc/init.d/sprdbluetooth start >/dev/null 2>&1 || true
/etc/init.d/bluetoothd restart >/dev/null 2>&1 || /etc/init.d/bluetoothd start >/dev/null 2>&1 || true

say "== 6/6 起蓝牙（按开关状态）"
SW=/usr/sbin/f50-bt-switch.sh
if [ -x "$SW" ]; then
	"$SW" on >/dev/null 2>&1 || true
else
	sh /usr/sbin/sprd-bt-attach.sh >/dev/null 2>&1 || true
fi

# 重装时控制器可能已经挂着，开关脚本会认为"已开"而跳过复位，
# 所以这里显式把可见性/可配对再推一遍（跟 airsim-bt-boot 的 visible() 同一套动作）
if hciconfig hci0 >/dev/null 2>&1; then
	hciconfig hci0 piscan >/dev/null 2>&1 || true
	printf 'pairable on\ndiscoverable on\nquit\n' | bluetoothctl >/dev/null 2>&1 || true
fi

# rpcd 认新接口、LuCI 认新 acl / 菜单
/etc/init.d/rpcd restart >/dev/null 2>&1 || true
/etc/init.d/uhttpd restart >/dev/null 2>&1 || true
sleep 3

say "== 结果"
if [ -d /sys/class/bluetooth/hci0 ]; then
	say "已装好：hci0 在位（$(hciconfig hci0 2>/dev/null | sed -n 's/.*BD Address: \(\S*\).*/\1/p')）"
else
	say "文件已装好，但 hci0 还没起来。去蓝牙页把开关打开看看；还不行看 logread | grep -iE 'btattach|sprdbt|bluetoothd'"
fi
say "完"
