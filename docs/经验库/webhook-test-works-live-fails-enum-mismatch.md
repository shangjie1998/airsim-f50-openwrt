---
name: webhook-test-works-live-fails-enum-mismatch
description: 自建轮询脚本对接本地 API 时，测试按钮能发通知但真事件一条不发，如何定位是上游状态枚举名与消费端分支不匹配
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [轮询脚本, 状态机, 枚举不匹配, 静默失败, webhook, 真机验证]
---
## 适用场景
自己写的轮询/转发脚本消费某个本地服务的状态接口，页面上的“测试”按钮能成功投递，但真实事件（来电、告警等）一条也不发时。

## 关键做法
先别怀疑网络、推送渠道或开关配置。用每秒一次的探针把上游原始响应落到文件，逐字比对上游状态字段的取值与脚本 `case` 里的标签。两边命名不一致时，让消费端同时接受两种命名（如 `incoming|ringing`、`active|answered`），而不是只改一边。改完必须用真实事件触发，看投递历史的落行确认。

## 踩过的坑
测试按钮走的是独立假事件路径，它能发成功恰好掩盖真实路径全漏；日志里只有 `forwarded test ...`、没有真实号码行，就是信号，修复前历史里从未出现过真实事件名。另外，挂断瞬间上游 `active` 已变空，模板里的方向字段会渲染成空白，需把方向随状态一起持久化到状态文件。

## 可复用的产物或模板
```sh
# 每秒采样上游状态，只打印变化点
i=0; : > /tmp/probe.log
while [ "$i" -lt 150 ]; do
	printf '%s|ACT=%s\n' "$(date '+%H:%M:%S')" \
		"$(wget -q -T 2 -O- "$URL" | sed -n 's/^.*"active":\(.*\),"history":\[.*$/\1/p')" >> /tmp/probe.log
	i=$((i+1)); sleep 1
done
awk -F'|' 'BEGIN{p=""} {if($2!=p){print; p=$2}}' /tmp/probe.log
```