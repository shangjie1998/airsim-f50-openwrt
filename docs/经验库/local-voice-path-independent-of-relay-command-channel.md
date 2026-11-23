---
name: local-voice-path-independent-of-relay-command-channel
description: 云模式下实时语音媒体走本地直连，不经过云端命令通道；relay 不可用期间本地呼出仍可双向打通。App 卡在「等待网关身份验证」时先按本地直连链路逐项验绿，再看云端快照，别把命令通道故障当成通话链路故障
metadata:
 type: work_experience
 source: harness
 created_at: "2026-11-23"
 tags: [云模式, relay, 命令通道, 本地直连, 配对, 身份验证, 鉴权计数, 并发竞态, hmac]
---
## 适用场景
设备以 cloud 模式接入，App 配对或建链时卡在「等待网关身份验证」；同时设备 `/api/push/status` 的 `command_connected=false`。容易把两类故障混在一起：一类是**本地直连链路**（网关鉴权、配对落库、health、媒体握手、声卡/DSP），一类是**云端命令通道**（relay 心跳/注册/命令拉取/快照回传）。两者解耦，要分别定性。

## 关键做法
1. 先把两条链路分开判：
   - **本地直连链路**：配对密钥是否落 `vowlan-clients.json`、在册钥是否在、`control_authenticated` 累计计数是否增长、`control_auth_rejected` 是否为 0、`/api/health` 各字段是否绿、`agent_id` 与在册记录是否同源。
   - **云端命令通道**：`/api/push/status` 的 `command_connected`、`last_error`，直接打 relay 各业务端点看状态码与错误码（1027/429 的定性见 `relay-cloudflare-worker-1027-daily-quota-exhausted`）。
2. 本地呼出（dial → PCM connection → authenticated → ready → 双向音频帧 → hangup）在 relay 不可用期间仍可双向打通：命令通道只负责云端下发与配对快照回传，**不承载实时语音媒体**。验证以一通真机拨测的音频帧计数（downlink/uplink/received_uplink）和 DSP `aud_send_cmd` 无 failed command 为准，不以 `command_connected=true` 为准。
3. claim/配对响应的 result 直接取 agent `/api/push/register` 响应体，`command_connected` 等云端快照字段原样回传给 App；App 看到的云状态差，先回读这个快照源，别在前端文案上猜。
4. 拨号瞬间若出现并发 `/api/calls/status` 的 `hmac_mismatch` / 单次 `control_auth_rejected`：属于拨号并发竞态，请求交错导致单次签名校验失败，随后自愈、主链路照常建链。看累计计数是否只跳 1 次且后续 `control_authenticated` 正常增长，**不要据此判定密钥错配或 #39 那类落库 bug**。

## 踩过的坑
- 把「App 卡身份验证」整体归因到本地密钥/鉴权：本地链路逐项验绿后，卡点往往在云端 relay；先分层再定方向，能省掉重复的鉴权排查弯路。
- 用 `command_connected=false` 推断「通话不通」：命令通道断不影响已建立的本地媒体通路，两码事。
- 看到一条并发 401 就重配对/重刷密钥：先看是否竞态自愈、后续鉴权计数是否恢复；贸然改钥会把本来正常的在册配对打乱。
- 只看 App 页面提示不回读设备端计数与快照：页面是结果呈现，定性要落到网关计数、health 字段、register 响应体这三处原始证据。

## 可复用的产物或模板
设备端分层体检：`wget -qO- http://127.0.0.1:7581/status`、`http://127.0.0.1:8575/api/health`、`/api/push/status`、`/api/debug`（agent 环形内存日志）、`ubus call airsim-pair pair_clients`、`netstat -ltnp/-tnp`。取证记录里 `device_secret/voip_token/alert_token/push_to_start_token` 全量值脱敏，`device_id` 只留前缀。
