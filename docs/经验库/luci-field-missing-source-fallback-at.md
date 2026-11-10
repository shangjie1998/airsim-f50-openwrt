---
name: luci-field-missing-source-fallback-at
description: 页面或推送显示"未挂载/未知"但设备硬件状态正常时，判断是后端取值路径在本机不存在（sysfs 节点缺失、外部工具未安装），并改用 AT/CLI 兜底
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [luci, rpcd, openwrt, sysfs, mmcli, at命令, 静默失败, 跨端移植]
---
## 适用场景
把另一台设备的页面与后端整页搬过来后，字段显示"未挂载""未知""—"；但用 `hciconfig`、进程列表、`ls /sys/class/...` 看到硬件与内核态其实一切正常。

## 关键做法
按「先核硬件、再查取值路径、最后换源」三步走：
1. 先用设备上的底层工具确认真实状态（如 `hciconfig hci0` 看 `UP RUNNING`、`pgrep -f bluetoothd`），确认不是硬件没起。
2. 再回头看后端脚本该字段是从哪取的，逐个核对那条路径在本机是否存在：`ls /sys/class/bluetooth/hci0/` 这类 sysfs 属性在部分内核上根本没有对应节点。
3. 路径不存在就换源，两类兜底：
   - sysfs 缺失 → 从 CLI 输出里抓（`hciconfig hci0 | sed -n 's/.*BD Address: *(...).*/\1/p'`）。
   - 依赖的外部工具缺失（判断 `command -v mmcli` 为空）→ 退回 AT：运营商读 `AT+COPS?`，取引号里的 MCC+MNC 做映射；信号读 `AT+CSQ`，`rssi=99/255` 一律记"未知"，不拿公式硬算一个假值。

## 踩过的坑
- 页面判据常写成 `attached && address` 两者同时成立才显示正常态。后端少了 `address` 就直接显示"未挂载"，对外表现为硬件故障，极易误判。
- 搬过来的后端如果用 `mmcli`（ModemManager）取运营商/信号，目标设备没装 ModemManager 时会静默返回空字符串，正文里全是"未知"，而脚本另有一套字段看着是对的。
- `AT+CSQ` 的 `99` 是"读不到"的哨兵值，按 `-113+2*rssi` 换算会得到 `-113dBm` 这种看似合理的假数据；这类"有值但没测到"的字段要如实留空。
- 设备能被搜到、能被配对，不代表页面显示正确，两者取值路径不同：前者看内核实态，后者看后端字段。

## 可复用的产物或模板
```sh
# 硬件真状态
hciconfig hci0 | sed -n '2,6p'; pgrep -f '[b]luetoothd' >/dev/null && echo bluetoothd=up
ls /sys/class/bluetooth/hci0/          # 没有 address 就走下面这条
hciconfig hci0 | sed -n 's/.*BD Address: *\([0-9A-Fa-f:]*\).*/\1/p' | head -1

# 无 mmcli 时的运营商/信号兜底
mu300-at -t 3 'AT+COPS?'   # +COPS: 0,2,"46011",11
mu300-at -t 3 'AT+CSQ'     # rssi=99 => 未知
```