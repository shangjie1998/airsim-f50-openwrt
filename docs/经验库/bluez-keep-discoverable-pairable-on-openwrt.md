---
name: bluez-keep-discoverable-pairable-on-openwrt
description: 让 OpenWrt/BlueZ 设备默认持续可被发现且可被配对，避免只设控制器扫描后手机搜不到或配不上
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [openwrt, 蓝牙, bluez, hciconfig, bluetoothctl, main.conf, 可被发现, 静默失败]
---
## 适用场景
嵌入式 OpenWrt 设备装好 BlueZ（bluetoothd + btattach 附着 H4 串口），要让手机、耳机默认就能搜到并直接发起配对，且重启后仍保持。

## 关键做法
三处都要设，缺一处就会出现"能搜到但配不上"或"过几分钟就消失"：
1. 控制器层：`hciconfig hci0 piscan`（PSCAN = 可被发现，ISCAN = 可被连接），只 `hciconfig hci0 up` 不够。
2. 服务层：`printf 'pairable on\ndiscoverable on\nquit\n' | bluetoothctl`，把 Pairable/Discoverable 交给 bluetoothd；控制器 PSCAN 不等于 Pairable。
3. 持久化层：`/etc/bluetooth/main.conf` 里 `DiscoverableTimeout = 0`、`PairableTimeout = 0`，否则到 180 秒自动掉出可发现。
开机脚本里拉起控制器成功后立刻做第 1、2 步；手动开关（rpcd）走同一段逻辑，避免只改一处。
核对用读回而不是看配置：`hciconfig hci0` 应显示 `UP RUNNING PSCAN ISCAN`，`bluetoothctl show` 应为 `Discoverable: yes`/`Pairable: yes`，接口据 PSCAN/ISCAN 返回 true。

## 踩过的坑
- `hciconfig hci0` 不带 `-a` 时不打印 `Name:` 段，脚本 sed 取名字会静默取空、JSON 里字段直接缺失且不报错；读名字必须 `hciconfig -a hci0`。
- 只改 main.conf 不够：bluetoothd 重启后控制器 PSCAN/ISCAN 会被清掉，必须在挂载成功后重设。
- 配对设备列表不能只看 `/var/lib/bluetooth/<控制器>/` 下的目录（`cache`、`settings` 也在里面），每台设备要读 `<mac>/info` 并确认含 `[LinkKey]` 才算真配对。

## 可复用的产物或模板
```sh
visible() {
	hciconfig hci0 piscan >/dev/null 2>&1
	if command -v bluetoothctl >/dev/null 2>&1; then
		printf 'pairable on\ndiscoverable on\nquit\n' | bluetoothctl >/dev/null 2>&1
	fi
}
```
```sh
sed -i 's/^#DiscoverableTimeout = 0/DiscoverableTimeout = 0/' /etc/bluetooth/main.conf
sed -i 's/^#PairableTimeout = 0/PairableTimeout = 0/' /etc/bluetooth/main.conf
```