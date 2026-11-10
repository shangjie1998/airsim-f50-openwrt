---
name: voice-bridge-hangup-vs-midcall-stream-fault
description: 旁路通话桥在通话结束瞬间报 aplay broken pipe / arecord I/O error 时，如何用探针数据判定是对方挂断的正常收尾还是真正的中途断流故障
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [alsa, 通话音频, 诊断, 挂断, broken-pipe, openwrt, airsim, 误判]
---
## 适用场景
自建旁路通话桥（Go 进程 fork `aplay`/`arecord` 读写展锐 voice FE）出现 `pcm_write: write error` / `pcm_read: read error: I/O error`，用户报“通话中途断链”“N 秒就断”时，先分清是真实流故障还是通话已结束的收尾噪音，再决定要不要改代码。

## 关键做法
用低频采样探针（每 2 秒采一次）同时记录三组量，事后按时间线对齐：
1. `/proc/asound/card0/pcmNNp/sub0/status` 的 `state`、`delay`、`avail`、`hw_ptr/appl_ptr`；
2. 桥的帧计数（downlink/uplink/silence）与 `last_error`；
3. 同一个 `/status` 里的 `call_active`。

判定口径：断流时刻之前若**帧计数持续增长、`delay` 一直在健康区间波动、没有任何 avail 触顶的 xrun 征兆**，且两三秒内 `call_active` 转 false，则这是对方挂断后 DSP 撤 voice path 导致的 EPIPE/EIO 收尾，不是桥的缺陷；内核日志里对应的只有干净正常的 `trigger cmd:0 → hw_free → shutdown → close`，没有驱动侧错误串。反之帧计数提前冻结、`last_error` 先出现而 `call_active` 仍为 true，才是真正的流故障。

## 踩过的坑
把挂断当成“固定 N 秒必断”的定时故障，会顺着用户描述往驱动/DSP 定时收摊上查，白耗时间；两次通话分别断在 50 秒和 98 秒本身就已经否证了“固定超时”的假设。另外 `--fatal-errors` 是为旧 vendor 内核的 DMA 不能重提 xrun 描述符而加的，换到新内核后这一前提不成立，它会让 aplay/arecord 在正常挂断时也直接退场、把 `last_error` 写满，掩盖真实信号。

## 可复用的产物或模板
探针脚本 `/Users/shangjie/Doubao/chats/2026-11-07/new-chat-6/m8-probe-pcm.sh`；对应实测日志 `/Users/shangjie/Doubao/chats/2026-11-07/new-chat-6/m8pcm.log`。