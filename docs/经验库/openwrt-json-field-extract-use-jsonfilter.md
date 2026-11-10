---
name: openwrt-json-field-extract-use-jsonfilter
description: 在 OpenWrt/busybox 设备上用 shell 从 ubus 等服务的 JSON 里取单个字段时，grep -o 与 sed 会静默取空，应改用 jsonfilter 按路径取值
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [openwrt, busybox, json, jsonfilter, ubus, 静默失败, shell]
---
## 适用场景
在 OpenWrt 设备上用 `sh` 从 `ubus call ...` 或其它服务返回的 JSON 里抠单个字段（信号值、PLMN、状态码），供上报、推送或页面展示使用。

## 关键做法
取字段统一用 `jsonfilter -e '@.a.b'`，它按路径解析、只返回裸值：
`_rsrp=$(printf '%s' "$_sig_json" | jsonfilter -e '@.sig.rsrp')`
同一含义有多处候选时连调多次、前一个为空再试下一个（`@.sig.rsrp` → `@.nr.rsrp` → `@.lte.rsrp`）。判断解析是否真取到，就先在设备上手工跑这一条看输出；若输出只有键名没有值，说明解析手段在当前环境不可靠，换工具而不是在调用方加判断。

## 踩过的坑
- busybox 的 `grep -o '"rsrp":[-0-9.]*'` 只吐出 `"rsrp":`，值全丢，不报错、退出码正常；下游拿到空串后 `printf '%.1f'` 直接报 `invalid number`。
- JSON 压在一行时，`sed -n 's/.*"plmn":"[0-9]*".*/\1/p'` 这类写法容易抓偏或抓到同名的另一处键（同一份 JSON 里 5G / LTE 段各有一份 `rsrp`）。
- 两种失败都是静默的：脚本跑完、文件照常生成，只是值为空，最后表现成页面或推送写"未知"。容易误判成设备读数能力不足，实际是解析没取到。

## 可复用的产物或模板
```sh
val=$(ubus call <obj> <method> | jsonfilter -e '@.path.to.field')
v=$(printf '%s' "$json" | jsonfilter -e '@.sig.rsrp')
[ -n "$v" ] || v=$(printf '%s' "$json" | jsonfilter -e '@.nr.rsrp')
```