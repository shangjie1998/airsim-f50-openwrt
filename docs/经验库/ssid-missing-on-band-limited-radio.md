---
name: ssid-missing-on-band-limited-radio
description: 扫描有返回结果但某个已知 SSID 不出现时，判定它是落在设备射频不支持的频段/信道而被过滤，而不是扫描功能坏了
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-12
 tags: [openwrt, wifi, 扫描, iwinfo, 频段, 信道过滤, 单射频, 静默失败]
---
## 适用场景
自研路由器/网关的网页里“某个已知 Wi-Fi 搜不到”，但扫描接口本身有正常返回、列表里也有其它热点时，先判断是过滤设计还是扫描故障，别直接去改扫描代码。

## 关键做法
抓一次全量扫描（绕过页面/接口的信道白名单），看目标 SSID 实际出现在哪些频点与信道：`iwinfo <if> scan` 输出里每条 `ESSID` 与 `Frequency ... Channel` 成对读；`iw dev <if> scan` 在部分厂商驱动上只回空结果，优先用 iwinfo。再对照本机射频能力（uci 的 band / channels / country / htmode）和过滤逻辑的信道白名单。若目标 SSID 只落在设备不支持的频段或信道，就是按能力过滤，不是故障；接口返回里的 filtered 计数可与全量结果对上，作为交叉验证。最后对照 AP 侧 2.4G/5G 配置：某频段“有配置”不等于真的广播了同名 SSID，优先让用户在该 SSID 设置处确认或显式改名后再复扫。

## 踩过的坑
“搜不到”很容易被当成扫描 bug 去改代码，实际是单射频设备只支持部分信道、代码故意不列连不上的热点。排查时连续高频扫描会把这类厂商驱动打进忙状态，随后 `iwinfo` 只报 `Netlink error while awaiting scan results: No event received`，看起来像彻底坏了；拉开间隔、必要时把无线栈重启一次再扫，别把驱动忙归因成功能坏。

## 可复用的产物或模板
全量对照：`iwinfo wlan0 scan | grep -iE 'ESSID|Frequency'`；能力核对：`uci show wireless.radio0 | grep -E 'band|channel|country|htmode'`