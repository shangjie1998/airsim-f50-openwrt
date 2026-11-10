---
name: ubus-upstream-missing-method-local-state-overlay
description: 上游 ubus 服务没有“标记已读/已处理”这类写状态方法时，在 rpcd 代理层用持久 id 清单合成状态，让页面显示和计数都对
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [openwrt, rpcd, ubus, jshn, 状态合成, 静默失败, 嵌入式]
---
## 适用场景
自建 LuCI 页面通过一层 rpcd 脚本代理上游 ubus 服务（如 mu300dash 之类的厂商 dash 守护进程），页面提供了“标记已读/已处理/归档”这类操作，但上游服务只暴露查询、发送、删除，没有对应的写状态方法，页面点了没反应或报失败。

## 关键做法
先直接调上游对象确认方法存不存在：`ubus call <dash> <method> {}` 返回 `Method not found`（退出码 253）就说明上游没有，不要再在上游找参数。方法缺了就由代理层自己承担状态：

1. 用持久文件存 id 清单（`/etc/<插件名>-state`，一行一个 id），放 `/etc` 而不是 `/tmp`，重启和升级都留着。
2. “全部已读”实现为逐页拉上游列表、把 id 收进清单，返回 `{"ok":1,"marked":N}`，`marked` 只数新增的，重打一次是 0。
3. 查询接口转发后必须**重写记录状态并重算聚合计数**（把 `unread` 改成 `read`、`unread` 数字按改写结果重新累加）；只改列表里单条状态、不重算计数，页面上的“N 条未读”仍然不对。
4. 单条“查看全文”成功后顺手记一条，让按钮文案“查看全文 · 标记已读”名副其实。

确认做对：`ubus call <代理> <查询> '{"page":1}'` 里目标条目 `status` 已变为 `read` 且顶层 `unread` 为 0，页面上计数与条目标签同时更新。

## 踩过的坑
- 上游 `sms_list` 返回的 `status` 永远是 `unread`（存在 SIM 卡上，卡本身不支持已读态），所以显示完全依赖代理层改写；不要以为拉到的是真状态。
- 在 busybox 上 `grep -o '"field":[-0-9.]*'` 会只吐出字段名几个字符、`sed` 因 JSON 一行到底也抓不稳，取数组元素要用 `jsonfilter -e '@.arr[*].id'`，并把每列落到临时文件再按行读，避免 shell 循环里反复解析。
- 这个“已读”只是本机状态，不写回卡；换设备或把卡插到别的机器会恢复未读，要在文档里写明，别当成交付缺陷。

## 可复用的产物或模板
```sh
READ_DB=/etc/airsim-sms-read
msg_is_read() { [ -n "$1" ] && grep -qx -- "$1" "$READ_DB" 2>/dev/null; }
msg_mark_read() { [ -n "$1" ] || return 0; msg_is_read "$1" && return 0; printf '%s\n' "$1" >> "$READ_DB"; }
# 列表：转发后改写 + 重算
_mid=$(sed -n "${i}p" "$_t.1"); _mst=$(sed -n "${i}p" "$_t.2")
[ "$_mst" = unread ] && msg_is_read "$_mid" && _mst=read
[ "$_mst" = unread ] && _unread=$((_unread + 1))
```