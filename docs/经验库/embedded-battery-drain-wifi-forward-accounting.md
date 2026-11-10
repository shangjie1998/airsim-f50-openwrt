---
name: embedded-battery-drain-wifi-forward-accounting
description: 嵌入式 4G/WiFi 路由设备待机异常掉电（屏灭、CPU 闲仍 200mA 以上）时，用电池历史曲线加网卡收发对账定位是否为转发流量导致
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [嵌入式, openwrt, 展锐, 功耗诊断, 电池, 流量对账, 真机验证]
---
## 适用场景
嵌入式 OpenWrt/展锐设备掉电异常快，用户描述"几小时就没电"，但屏是灭的、系统看起来空闲。需要判断是本地负载（屏、CPU、后台进程）还是射频/转发在吃电。

## 关键做法
1. 先读设备自带的电池历史，通常是一分钟一条的 CSV（本项目在 `/etc/e5/battery/battery.csv`，字段为 时间,容量%,电流uA,功率,电压,状态,温度）。按小时聚合，定位掉电时段与电流量级，得到"异常时段的稳态电流"这一基准。
2. 排除本地负载：读背光值（`/sys/class/backlight/*/brightness`）与 `/proc/loadavg`。若背光为 0、load 低于 1，本地算力与显示均可排除。
3. 用网卡收发对账：对比 WiFi 出口（`/sys/class/net/wlan0/statistics/tx_bytes`）与蜂窝入口（本项目为 `sipa_eth0` 的 `rx_bytes`）。两者数值接近且量级很大（如五小时 5.6GB），即为 4G→WiFi 转发，双射频满载就是电流来源。
4. 需要定位具体客户端时读 DHCP 租约（`/tmp/dhcp.leases`），拿主机名与 IP。
5. 需要长期观察时在 `/tmp` 挂轻量采样器，每分钟记容量、电流、背光、load 与日志行数，避免依赖环形缓冲日志。

## 踩过的坑
- 电流 sysfs 单次读数抖动极大，连续采样会出现 -9/-30/-68mA 乱跳，必须多次取样取均值，否则误判。
- 内核 logread 环形缓冲只保留约 50 分钟（音频驱动刷屏时更短），出问题几小时后现场早已滚掉，必须靠电池 CSV 或自建采样器，不能指望事后翻日志。
- vnstat 之类工具在本机数据库为空，不能作为流量历史来源。
- 插上 USB 接电脑后 `battery/status` 变为 Charging，`current_now` 变成充电电流，不再是真实放电值，判断必须在纯电池状态下做。
- 屏幕容易被当成头号嫌疑，但实测点屏只增加 10~15mA，与 200mA 以上差距太大，先对账流量更高效。

## 可复用的产物或模板
每分钟采样脚本（放设备 `/tmp` 后台跑）：

```sh
LOG=/tmp/power-watch.log
[ -f "$LOG" ] || echo "time,cap,cur_uA,backlight,load,loglines,vbc_lines,airsim_lines,modem_state" > "$LOG"
while true; do
  t=$(date +%s)
  echo "$t,$(cat /sys/class/power_supply/battery/capacity 2>/dev/null),$(cat /sys/class/power_supply/battery/current_now 2>/dev/null),$(cat /sys/class/backlight/sprd_backlight/brightness 2>/dev/null),$(cut -d' ' -f1 /proc/loadavg),$(logread | wc -l),$(logread | grep -c vbc),$(logread | grep -c airsim),$(cat /sys/class/power_supply/battery/status 2>/dev/null)" >> "$LOG"
  sleep 60
done
```