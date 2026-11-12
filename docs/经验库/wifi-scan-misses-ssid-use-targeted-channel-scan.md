---
name: wifi-scan-misses-ssid-use-targeted-channel-scan
description: 单射频设备扫描能返回结果、但目标 SSID 时有时无（漏抓）时，改用限定支持信道的定向扫描提高命中率
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-12
 tags: [openwrt, wifi, 扫描, iw, iwinfo, 信道, 单射频, 漏抓, 嵌入式]
---
## 适用场景
设备扫描 Wi-Fi 有返回、列表里也有几条，但某个已知就在附近的 SSID 反复不出现或时有时无；尤其是单射频机型（只支持固定信道段，如 5G 36/40/44/48）。

## 关键做法
1. 先证伪"频段不支持"：全量扫一次并把每条结果连频点打出来（iwinfo 输出按 `ESSID` 行 + `Mode: ... Frequency: ... Channel:` 行配对），确认目标 SSID 的实际频率与信道。若它落在设备支持的信道内，问题就是漏抓而不是能力不足。
2. 改用定向信道扫描做主路径：`iw dev <sta_if> scan freq 5180 5200 5220 5240`（5G 信道→频率换算 = 5000 + 信道号 × 5）。只扫本机真正能用的几个信道，不必跨 2.4G 走十几个信道。
3. 解析 `iw` 输出：按 `BSS` 行开块，块内取 `freq:`、`signal:`、`SSID:`；有 `RSN:`/`WPA:` 行即判加密，否则开放。
4. 保留 iwinfo 全信道 + 按信道过滤作为兜底，因为有的固件 `iw scan` 回不了 netlink 事件，只在兜底失败时才报"扫描没结果"。

## 踩过的坑
- 全信道扫一圈（2.4G + 5G 十几条）单轮经常漏抓，页面上只剩两三条，用户会以为搜不到自家网络；同一台设备连扫两次，第二次才抓到目标，说明这是概率性漏抓而不是配置错。
- `iw scan` 报 `Resource busy (-16)` 或 `Netlink error while awaiting scan results: No event received`，多半是射频睡着（`iwinfo <if> info` 里 `Tx-Power: 0 dBm`），先 `wifi up` 顶起来再扫。
- AP 模式下的接口不响应扫描，必须临时建一个 managed 接口（`iw phy phy0 interface add scan0 type managed`），扫完删掉；别拿 AP 类型的接口直接扫。
- 按信道过滤会丢掉 2.4G 的结果，这是设计如此，但要在页面上写清楚"另有 N 个在不支持的信道，本机连不上"，否则用户误判成搜索坏了。

## 可复用的产物或模板
```sh
# 5G 定向扫描（信道→频率 = 5000 + 信道号*5）
iw dev wlan0 scan freq 5180 5200 5220 5240 2>/dev/null | awk '
  /^BSS/      { if (s != "") printf "%s|%s|%s|5 GHz|%s|%s\n", b, s, c, g, e
                b = substr($0, 5, 17); s=""; c=""; g=""; e="NONE" }
  /freq:/     { c = int($2 / 5 - 1000) }
  /signal:/   { g = $2 " dBm" }
  /SSID:/     { s = $0; sub(/^[ \t]*SSID: /, "", s) }
  /RSN:|WPA:/ { e = "psk2" }
  END         { if (s != "") printf "%s|%s|%s|5 GHz|%s|%s\n", b, s, c, g, e }'
```