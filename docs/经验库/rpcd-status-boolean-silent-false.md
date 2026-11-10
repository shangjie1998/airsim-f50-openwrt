---
name: rpcd-status-boolean-silent-false
description: rpcd/ubus 状态接口里布尔字段恒为 false 时，怎么定位是 jshn 类型转换还是进程检测命令本身没匹配上
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [openwrt, rpcd, ubus, jshn, shell, 静默失败, busybox]
---
## 适用场景
给 OpenWrt 写 rpcd 后端，接口返回了正确的 JSON 结构，但其中某个 `service_running`、`enabled` 之类的布尔字段一直是 `false`，脚本里看着逻辑没错。

## 关键做法
按两层排查，从输出侧往命令侧走：
1. 先怀疑 jshn 的类型转换：`json_add_boolean` 只认 `1`/`0`，传 `"true"`/`"false"` 字符串会被渲染成 `false`。把赋值改成 `if 条件; then _v=1; else _v=0; fi` 再 `json_add_boolean 字段 "$_v"`。
2. 再怀疑命令本身被引号污染：`json_add_boolean x "$(pgrep -f 'pattern' >/dev/null && echo true)"` 这种写法里，外层双引号内的单引号是**字面量**，不会经由 shell 去掉，`pgrep` 收到的是带引号的模式，永远匹配不上。改成把判断提到语句前：`if pgrep -f 'pattern' >/dev/null 2>&1; then _v=1; else _v=0; fi`。
3. 定位时用 `sh -x 脚本 call 方法` 看实际展开后的参数，再单独跑最小复现（只做 `json_init`+赋值+`json_dump`）区分是哪一层坏掉。

## 踩过的坑
- 用 `ps | grep` 手工验证进程明明在，接口却报 `false`，容易误判成“服务没起来”而去反复重启服务，实际是字段渲染问题。
- 修完第一层（引号）仍为 `false` 时容易以为没生效，其实是第二层（jshn 布尔类型）还在；两层都要改。
- 改 rpcd 脚本后 `/etc/init.d/rpcd restart` 会踢掉 LuCI 会话，页面卡“加载视图中…”并刷出 `"%s/%s.js%s".format is not a function`，这是会话失效的连带现象，补一次 `/etc/init.d/uhttpd restart` 并开新标签页重登再验，不要当成页面代码的问题去改。

## 可复用的产物或模板
```sh
# 正确写法
if pgrep -f 'airsim/call-forward' >/dev/null 2>&1; then _run=1; else _run=0; fi
json_add_boolean service_running "$_run"
# 定位
sh -x /usr/libexec/rpcd/<对象> call <方法>
```