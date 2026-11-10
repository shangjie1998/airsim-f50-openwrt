---
name: device-display-name-rename-preserve-identifiers
description: 给自建设备网关改对外显示名（mDNS/配对/页面）时，哪些字段该换、哪些标识必须原样保留，以及怎样验证没打断客户端识别
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [mdns, 零配置发现, 设备改名, 客户端识别, openwrt, 网关]
---
## 适用场景
要把自建服务端设备的对外品牌名换掉（如 MU300→VIPME-M8），且已有客户端 App 通过局域网发现/配对连接这台设备时。

## 关键做法
只替换供人读的字段：mDNS 实例名、TXT 里的 device_name/model、health 与注册接口的 device_name/model、product 显示串、Web 面板文案。
必须原样保留：客户端用来判别设备类型的 agent_kind、模块导入路径、二进制文件名、随包工具脚本名、升级检查用的仓库地址。这些一旦改了，要么客户端不认设备，要么升级链断。
改完用真实发现路径验证：`dns-sd -B _服务类型_` 看到新实例名，`dns-sd -L <实例> 服务类型` 确认 TXT 里 device_name 已是新名、agent_kind 仍是旧值，再查 /status 的 product 与版本。

## 踩过的坑
已配对过的客户端把 device_name 缓存在本地配对记录里，服务端改名后老客户端仍显示旧名，必须删设备重新配对才会刷新；改名后也要确认原来共存的其他对象（如短信代理）仍在，别只check 主服务。

## 可复用的产物或模板
（无）