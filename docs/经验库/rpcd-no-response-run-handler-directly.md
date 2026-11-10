---
name: rpcd-no-response-run-handler-directly
description: ubus 调自建 rpcd 对象只报 "(No response)"、看不到真实错误时，如何手工执行处理器脚本取回 stderr 定位失败原因
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-07
 tags: [openwrt, rpcd, ubus, luci, 静默失败, 诊断]
---
## 适用场景
在 OpenWrt 上自建 `/usr/libexec/rpcd/<对象>` 供 LuCI 页面调用，`ubus list` 能看到对象已注册，但 `ubus call` 或页面调用只返回 `Command failed: ubus call <对象> <方法> (No response)`，没有任何错误正文。

## 关键做法
先绕过 ubus，按 rpcd 调用的真实协议手工跑一遍处理器：`printf '{}' | /usr/libexec/rpcd/<对象> call <方法>`（需要参数时按同样 JSON 体传入），再单独 `sh -x` 或直接执行处理器最终调用的那个后端脚本。这样 stderr 会当场打印出来（例如脚本 `source` 的公共库不存在），失败原因立刻可见，不必在页面上反复点。
对分发到外部脚本的 rpcd 对象，还要确认该外部脚本依赖的共享库/二进制在裁剪过的镜像里确实存在，缺库时处理器会整体退出，ubus 侧只会体现为无响应。

## 踩过的坑
`ubus call` 返回 No response 只说明处理器没吐出合法 JSON 就退出了，页面上几乎无法判断是参数、权限还是缺文件；靠页面日志排查会白费时间，直接手跑处理器就能一句话看到缺哪个文件。

## 可复用的产物或模板
printf '{}' | /usr/libexec/rpcd/<object> call <method>
/usr/libexec/rpcd/<object> list