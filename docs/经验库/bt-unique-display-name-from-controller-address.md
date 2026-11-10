---
name: bt-unique-display-name-from-controller-address
description: 给嵌入式 OpenWrt 设备设一个短、唯一、重启不重复的蓝牙显示名时，用控制器地址后缀在 attach 脚本里现算，并留一个静态兜底名
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [openwrt, 蓝牙, bluez, hciconfig, 唯一标识, procd, 嵌入式]
---
## 适用场景
设备自带蓝牙、需要对外广播一个能区分多台同型机的名字（手机蓝牙列表里可读），且不想引入随机值或超长标识。

## 关键做法
取控制器 BD Address 的末 6 位作为后缀：`hciconfig hci0` 输出里地址，去冒号、转大写，得到 `F50-XXXX` 这种 11 字符的名字；长度够短、在手机端不截断，且同一控制器的地址重启不变。
写入时机放在 attach 脚本里「hci0 上电成功之后」，由 procd 常驻拉起，随开机重设；同时在 `/etc/bluetooth/main.conf` 的 `Name` 写一个静态兜底名（如 `F50`），避免取地址失败时名字回落成厂商默认值。取不到地址时让后缀为空，名字退化为兜底名，不产生 `F50-` 这种半截名。
不要用房内后端服务生成的「设备标识」：蓝牙 attach 通常早于该服务就绪，取值会为空。

## 踩过的坑
`hciconfig` 输出形如 `BD Address: <蓝牙地址>  ACL MTU: 1021:8  SCO MTU: 240:3`，地址是**第 3 列**；按 `awk '{print $4}'` 取到的是 `ACL`，最终名字变成 `F50-`，必须用 `$3`。
本机验证到的是手工执行同款命令后名字正确；脚本在整机重启时自动重设这条路径尚未实测，依赖它之前先在真机上重启复核一次。

## 可复用的产物或模板
```sh
_bd=$(hciconfig hci0 2>/dev/null | awk '/BD Address/ {print $3; exit}')
_tag=$(printf '%s' "$_bd" | tr -d ':' | tr 'a-f' 'A-F')
_suf=$(printf '%s' "$_tag" | awk 'length($0)>=6 {print substr($0,length($0)-5)}')
hciconfig hci0 name "F50${_suf:+-$_suf}"
```