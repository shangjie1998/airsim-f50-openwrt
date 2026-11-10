---
name: mu300-voice-choppy-exclude-local-chain
description: 设备端双向有声但用户报断续时，用逐帧采样判断本地采集-播放链是否干净，从而把排查转向传输或射频侧
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-07
 tags: [openwrt, mu300, alsa, pcm, 通话质量, 断续, 诊断]
---
## 适用场景
在 OpenWrt/Linux 上把 modem 语音 PCM 经 DSP + ALSA 桥接到网络（如 AirSIM mu300 插件）后，双向都有声音，但用户反馈断续、卡顿或时好时坏，需要先判断是不是设备本地音频链的问题。

## 关键做法
1. 读设备状态接口（如 `http://127.0.0.1:<管理端口>/status`）拿到 `audio` 段：`downlink_frames`、`received_uplink_frames`、`uplink_frames`、`dropped_uplink_frames`、`silence_frames`，以及 `gateway.events` 事件计数。
2. 通话进行中按秒级周期把上述字段连同 `call_active` 和 DSP 状态落盘，形成时序，而不是只看通话结束后的累计值。
3. 用帧节奏做判据：PCM 帧多为 20 ms 一帧，正常应约 40 帧/秒稳定递增；若帧计数按时增长、`dropped_*` 在通话中不再增长、静音占比处于个位数百分比、且日志里没有 xrun/underrun 或音频进程异常退出，则本地采集-播放链和 DSP 供给是干净的，断续不在设备侧，应转查 WiFi 链路（信道、带宽、干扰）与蜂窝侧语音质量。

## 踩过的坑
容易一上来就改 ALSA 缓冲、period size 或采样率。帧节奏正常、丢帧不增的情况下改本地参数没有任何收益，只会把真因（传输或射频侧的瞬时抖动）掩盖掉。先做排除，再决定动哪一侧。

## 可复用的产物或模板
采样脚本：/Users/shangjie/Doubao/chats/2026-11-07/new-chat-5/修复脚本/通话实时监控.sh