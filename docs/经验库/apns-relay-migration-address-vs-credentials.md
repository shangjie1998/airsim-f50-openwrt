---
name: apns-relay-migration-address-vs-credentials
description: 判断能否把带 APNs 推送的中继服务迁到自建地址时，先分清哪些链路只认 HTTP、哪些被推送凭据锁死，避免"改完地址以为迁移完成"
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [apns, pushkit, relay, 自建部署, 第三方签名, cloudflare-worker, workerd]
---
## 适用场景
要把一个替客户端中转、且承担 Apple 推送的中继服务（Relay / push worker）换到自建域名或自建机上，而客户端只提供"填服务器地址"的入口、不打算重新签名打包时，需要判断这次迁移实际能覆盖哪些功能。

## 关键做法
拆链路判断，不要整体认定"能迁"：
1. 只走 HTTP/WebSocket 的链路——设备注册、鉴权、心跳、命令队列、通话媒体中继、运维面板——换服务器地址即生效，与签名身份无关。
2. 依赖 APNs 的链路——后台/锁屏来电（PushKit VoIP）、普通通知、Live Activity、Watch 推送——服务端必须有该 App 所属 Apple Team 的 `AuthKey_<KEY_ID>.p8` + Key ID + Team ID 签 ES256 JWT，且推送 topic 由 Bundle ID 派生并逐条校验归属。他人签名构建改不了这一点。
3. 结论落法：只改地址保留他人签名，可用的是一半功能；想让推送也通，必须自己 Team 重签主 App 及子 bundle（如 `.watchkitapp`、`.liveactivity`），并同步改服务端允许的 Bundle ID 与推送凭据。
4. 拿不准时看客户端源码/文档里是否有地址注入项（构建期变量或设置页），以及服务端配置里是否只有 token、Bundle ID 这类不涉及签名的校验项。

## 踩过的坑
- 服务端配置文件对 APNs 字段往往不强制校验，缺了也能起、`/healthz` 也返回 200；健康检查只证明 HTTP 服务活着，不代表推送、心跳或通话通过验收，别用它当迁移成功的凭据。
- 客户端"能填地址"只说明发现端可换，不代表签名身份可换；两者是独立开关。
- 只换签名证书、留着别人的 Bundle ID 一样拿不到对应 App ID 与推送 topic 的权限。

## 可复用的产物或模板
判断清单：HTTP/WS 链路清单 → APNs 链路清单 → 服务端凭据归属（团队 .p8、Key ID、Team ID）→ topic 与 Bundle ID 派生关系 → 子 bundle 与允许列表是否同步。任一项落在他人 Team 手里，就只能走重签路线。