---
name: bluez-display-name-survives-service-restart
description: 在 OpenWrt/BlueZ 设备上设了蓝牙显示名，服务一重启名字就退回默认时，怎么让名字持久、并可靠读出当前名字
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [openwrt, 蓝牙, bluez, main.conf, hciconfig, bluetoothctl, 静默失败]
---
## 适用场景
在嵌入式 OpenWrt（BlueZ + bluetoothd）设备上给蓝牙控制器设自定义显示名，且此后有可能重启 bluetoothd（删配对、重装组件、手动重启服务）时。

## 关键做法
1. 设名不能只写控制器：hciconfig hci0 name 只改当前控制器状态，bluetoothd 一重启就按 /etc/bluetooth/main.conf 的 Name 覆盖回去。应在同一处（如 attach 脚本）既下发控制器名，也把 main.conf 的 `[General]` 段 `Name = ...` 改成同一份名字。
2. 改完 main.conf 后要重启 bluetoothd 才生效；重启会清掉 PSCAN/ISCAN，紧接着必须补 `hciconfig hci0 piscan` 和 `bluetoothctl` 的 `pairable on/discoverable on`。
3. 读名字优先 bluetoothctl show 的 `Name:`/`Alias:`（走 bluetoothd，手机端看到的就是它）；hciconfig -a 读本地名在刚重启后可能 HCI 超时返回空，只能作快路径，取空时用它兜底。

## 踩过的坑
只写 hciconfig hci0 name、main.conf 仍是默认 Name：删配对触发 bluetoothd 重启后，页面和手机端名字都退回默认名，而删除、可见性本身正常，属于静默回退。另外 hciconfig -a 读名会报 `Can't read local name on hci0: Operation timed out (110)`，此时若只依赖它，页面显示名为空。

## 可复用的产物或模板
conf=/etc/bluetooth/main.conf
if grep -qE '^[[:space:]]*Name[[:space:]]*=' "$conf"; then
	sed -i "s|^[[:space:]]*Name[[:space:]]*=.*|Name = $btname|" "$conf"
else
	printf '[General]\nName = %s\n' "$btname" >> "$conf"
fi
名读取：bluetoothctl show 2>/dev/null | sed -n 's/^[[:space:]]*Name:[[:space:]]*//p' | head -n1