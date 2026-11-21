---
name: relay-cloudflare-worker-1027-daily-quota-exhausted
description: 配对/命令通道卡住，设备拉命令返回 429 且 body 为 error code: 1027 时，先判 Cloudflare Worker 免费档日额度耗尽被边缘停用，别再往本地密钥鉴权或设备网络上找
metadata:
    type: work_experience
    source: unknown
    created_at: "2026-11-21"
    tags:
        - 云端中继
        - relay
        - 429
        - 1027
        - cloudflare
        - worker
        - 命令通道
        - 配对卡住
        - 静默差异
---
## 适用场景
手机 App 配对或下发命令卡住；设备 agent `/api/push/status` 里 `command_connected=false`、`last_error="Relay HTTP 429"`。表象和早期「本地密钥写入/鉴权失败」很像，容易重复走鉴权排查弯路。

## 结论
`error code: 1027` 是 **Cloudflare Workers 免费档每日 10 万请求超限**的专属错误码（fail-closed 模式下边缘直接返回 1027 错误页，请求不进 Worker）。中继后端是一个 Cloudflare Worker，当日额度打满后整条路由被临时停用，与设备密钥、HMAC、出口 IP、蜂窝驻网都无关。UTC 午夜（北京时间次日 08:00）额度自动重置，设备侧对 429 已有退避，恢复后会自动重连。

## 判据（按顺序交叉验证）
1. 设备上用 `push.json` 里的 `device_id / device_secret / android_agent_id` 手动 `POST <relay>/v2/commands/pull`，看状态码与响应头：
   - `Server: cloudflare`、带 `CF-RAY`；
   - `Server-Timing: cfEdge;dur=...,cfOrigin;dur=0` —— `cfOrigin dur=0` 说明请求根本没到源站，是边缘自拦；
   - body 纯文本 `error code: 1027`，且通常不带 `Retry-After`。
2. 扫一遍全站路径：`GET /`、`GET /v2/health`、命令端点、随便一条不存在的设备路径，**全部 429 1027**，不分方法、不带凭据也一样。这一步用来排除单设备被限/凭据无效/agent_id 不匹配。
3. 换一个独立出口（另一台设备或 Mac）打同一个 relay 首页，仍 1027 → 服务端全局停用，不是本机问题。
4. 1027 对照 Cloudflare Workers Limits 文档：Daily requests 超限返回 Error 1027；免费档每日 100,000，UTC 午夜重置。

## 本次排除项（避免重复排查）
- 本地 HMAC/密钥写入：网关诊断 `control_auth_rejected=0`、`control_authenticated` 累计计数、`paired_clients=1`、key 已落 `vowlan-clients.json`、`paired_this_boot=True`，说明本地鉴权全过，#39 那类密钥 bug 不是这次根因。
- 设备出口 IP 限流 / 单设备封禁：全站路径与跨设备复现结果一致，排除。
- 凭据或 agent_id 不匹配：无凭据的公开路径同样 1027，排除。
- 蜂窝未驻网：reg_status 已注册、信号可用，与命令通道无关。

## 处置
1. 等重置：UTC 午夜（北京 08:00）后 Worker 恢复；判据是 `curl https://<relay>/` 返回 200，设备 8575 `/api/push/status` 的 `command_connected=true`，再重新配对/下发命令验证。
2. 若调试期反复把日额度打满：登 Cloudflare 账号看该 Worker 今日 requests 用量与 1027 计数；要么升级 Workers Paid（每日请求数放开，subrequest 另计），要么降低设备端命令轮询/WSS 重连频次，避免退避重试本身继续吃额度。
3. 设备侧本次不改配对/鉴权代码。可选加固（未做，P2）：识别到 1027/无 `Retry-After` 的 429 时拉长退避，别激进重试加剧额度消耗。

## 踩过的坑
- 把「配对卡住」直接对上 #39 的密钥问题：两次表象相似但根因不同，这次是云端额度。先看 429 响应头和错误码再定方向。
- 只打命令端点就下结论：必须对比公开路径，才能区分 Worker 全局停用与单设备/凭据被拒。
- 1027 不是源站业务码：本地 Go 源码和业务码表里 grep 不到是正常的，它来自 Cloudflare 边缘。

## 可复用的产物或模板
设备端手动复现脚本：读 `/mnt/mu300-disk/extra/airsim/data/push.json` 取 `relay_url/device_id/device_secret/android_agent_id`，用 python3 urllib 打 `/v2/commands/pull`，打印状态码、`Retry-After`、body 前 200 字符；脚本与输出不得回显 `device_secret/voip_token/alert_token/push_to_start_token` 全量值，`device_id` 只留前缀。
