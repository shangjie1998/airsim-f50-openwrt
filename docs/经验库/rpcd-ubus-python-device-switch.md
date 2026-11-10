---
name: rpcd-ubus-python-device-switch
description: 用 rpcd/ubus 包一层 python 脚本给设备做开关（如蓝牙、射频）时，避开 uci 匿名节静默取不到值、ubus 30 秒超时和 sysfs 无状态节点的三个坑
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [openwrt, rpcd, ubus, uci, 静默失败, 超时, sysfs, 嵌入式]
---
## 适用场景
在 OpenWrt 上把设备侧控制（开关某服务、上电某硬件）暴露成 LuCI 能调的 ubus 方法，后端用 python 写，需要一套能同时给多个机型用的实现。

## 关键做法
机型差异全部外置到一份 uci 配置，代码只读配置；后端提供 `get` 返回状态、`set` 执行开关，两者都用同一份状态组装函数，`set` 结束前把状态回读一遍返回。
验收不看脚本能不能跑，看 `ubus call <对象> <方法>` 的返回体：`set` 之后 `attached/up/enabled` 这类字段要真的翻转，再回读一次确认不是一次性的。
## 踩过的坑
1. uci 配置写成匿名节 `config main`，`uci get 包.main.选项` 取不到，后端把“读不到配置”当成“本机不支持”，前端静默整张卡不显示——配置节必须写成具名节 `config main 'main'`。先用 `uci show` 确认节名再联调。
2. ubus 调用有 30 秒上限：后端里串行 `sleep` 等硬件上线（20 s + 10 s）直接把整次开关拖成 `Request timed out`，而设备侧其实已经生效。等待总预算压到 10 秒以内，没就绪就返回“正在初始化”让前端自己再查。
3. `uci set` 必须写全 `包.节.选项=值`，写成 `包.节=值` 会把节名改掉。
4. 别假设 sysfs 有状态节点：某些平台 `/sys/class/bluetooth/hci0/` 下没有 `address`，只能从 `hciconfig` 输出里解析；判断“在不在”用目录存在即可。
## 可复用的产物或模板
```
case "$1" in
	list) cat <<'JSON'
{"<obj>_get":{},"<obj>_set":{"enable":"string"}}
JSON
	;;
	call) exec /usr/bin/python3 /opt/<pkg>/bin/<script>.py "$2" ;;
esac
```
配置节：`config main 'main'` + 全部 option；前端用 `Promise.all` 同时取设备状态与该对象状态。