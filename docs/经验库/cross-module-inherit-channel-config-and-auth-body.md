---
name: cross-module-inherit-channel-config-and-auth-body
description: 页面功能「跟随另一个模块的推送配置」时地址/密钥取空、或 HTTP 200 却业务失败，如何找到真实配置源并正确继承
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [openwrt, luci, rpcd, 配置源, 继承, 鉴权字段, 静默失败, uci, jsonfilter]
---
## 适用场景
某个功能复用另一模块（短信转发、推送通道等）的渠道参数，页面提供「跟随 XX」选项，但测试发送报目标地址无效，或 HTTP 200 却回业务错误（如 token 为空）。

## 关键做法
先确认被跟随模块的真实配置源，不要默认它在 uci：`uci -q get <段>.<键>` 返回空既可能是没配，也可能是配置根本不在 uci。用已知道的某条配置值反查实际文件（如 `grep -rl <令牌片段> /etc`），再用 `jsonfilter -i <文件> -e '$.键'` 按路径取值。
读取做成回落链：先用旧的/兼容来源，取到非空就返回；为空再读真实来源；两边键名常不一致（`url` 对 `webhook_url`），在映射表里对齐。
「跟随」要连鉴权载荷一起继承：密钥/token/topic 写在正文模板里的渠道，只继承地址和方法必然失败，必须把正文一并继承；仅当用户在本页确实改过正文时才用他自己的模板。
模板变量要按语义逐一映射（短信侧 `{from}/{text}/{sim}` 对通话侧的事件、号码、卡槽），否则变量名会原样漏在推送内容里。
成功判定看服务端业务码（响应体 code/errcode），不能只看 HTTP 状态码。

## 踩过的坑
只读一个本机不存在的 uci 段 → 取到空地址，后端落成「目标地址无效」，看起来像用户没填，实际是配置源不对。
只修地址不修正文 → 服务端返回 HTTP 200 但响应是 `{"code":999,"data":"token不能为空"}`，页面显示「服务响应无法确认成功」，很容易误判成网络问题。
修复后必须用真实接口回读一次（例如调用该模块的测试方法看 result/http_code），而不是看脚本退出码。

## 可复用的产物或模板
```sh
cfg_get() {  # 先兼容来源（uci），缺省读厂商 JSON
  v=$(uci -q get "e5-notify.forward.$1" 2>/dev/null)
  [ -n "$v" ] && { printf '%s' "$v"; return; }
  [ -f "$JSON" ] || return 0
  jsonfilter -i "$JSON" -e "\$.webhook_$1" 2>/dev/null
}
```