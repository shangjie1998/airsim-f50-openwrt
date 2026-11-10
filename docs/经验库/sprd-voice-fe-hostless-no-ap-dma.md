---
name: sprd-voice-fe-hostless-no-ap-dma
description: 展锐 sprdphone 声卡上旁路读写通话音频时，别只盯主 voice FE，改找同卡的语音采集/注入 FE 并打开对应开关，即可拿到带 DMA 的真数据
metadata:
    type: work_experience
    source: harness
    created_at: "2026-11-07"
    tags:
        - alsa
        - sprd
        - unisoc
        - voice
        - 通话音频
        - mcdt
        - dma
        - 诊断
---
## 适用场景
展锐（Unisoc/Sprd）平台、声卡名为 `sprdphone-*` 的设备上，想在用户态直接读写通话双向音频（旁路或接管），而主语音节点 `FE_ST_VOICE` 打不开、或打开后读写立刻 EINVAL 时。

## 关键做法
1. 不要只盯主 voice FE。`FE_DAI_ID_VOICE`（`FE_ST_VOICE`）常常只是点灯/路由手柄，驱动里可能被写进 `default: do not use dma` 分支。
2. `cat /proc/asound/pcm` 枚举同卡全部 FE，找 `FE_ST_VOICE_C`（语音下行采集）与 `FE_ST_VOICE_PCM_P`（语音上行注入）——这两者走 MCDT DMA，才有 AP 数据通路。可在内核源码 `sprd-fe-dai.c` 的 `mcdt_dma_config_init` 里核对 `FE_DAI_ID_VOICE_CAPTURE` / `FE_DAI_ID_VOICE_PCM_P` 是否挂了 MCDT 通道。
3. 先开路由开关，否则节点 open 后读写仍 EINVAL：`S_VOICE_CAP_C SWITCH`（下行送 AP）、`S_VOICE_PCM_P SWITCH`（AP 混入上行）。
4. 通话 active 时用 S16_LE、1ch、8000Hz、period 640、buffer 1280 读写这两个节点。
5. 判定成功：arecord 返回 0 且 wav 里确有大片非零样本（看 peak 与非零计数），aplay 返回 0 且不报 `pcm_write` 错。仅 open 成功不算成功。
6. 用完把两个开关关回 off，避免 DSP 残留多余路由；不必停掉原生语音服务，可与其并行。

## 踩过的坑
- 由"主 FE 无 AP DMA"推出"用户态永远拿不到通话音频、只能改内核"，是错的；同卡另有带 DMA 的语音 FE，先把节点枚举清楚再谈改内核。
- `FE_ST_DUMP` 在部分机器上 open 报 `no backend DAIs enabled`，不能当采集端点用。
- `dd` 读 `/dev/audio_pipe_voice` 得 EINVAL：该 misc 设备是 SIPC 命令通道，read 的 count 必须恰好等于 `struct aud_smsg` 大小，不是音频流。
- 在主 FE 上改 period/buffer/启动时机、找 sysfs usecase 节点，都是白耗。

## 可复用的产物或模板
```sh
amixer -c 0 cset "name=S_VOICE_CAP_C SWITCH" on
arecord -D hw:0,9 -f S16_LE -r 8000 -c 1 --period-size=640 --buffer-size=1280 -d 3 /tmp/voice.wav
amixer -c 0 cset "name=S_VOICE_PCM_P SWITCH" on
aplay  -D hw:0,18 -f S16_LE -r 8000 -c 1 --period-size=640 --buffer-size=1280 /tmp/uplink.raw
```
## 补充：双向同时跑时的时序硬约束（2026-11 实测）

- 语音采集 FE 与语音播放 FE **可以同时开**，但顺序是硬性的：先开播放（aplay 起稳），再开采集。
- 两个 mixer 开关一起 flip、或先开采集再开播放，都会让 aplay 约 0.5~1 秒后以 `pcm_write: I/O error` 退出，采集同时变静音；单独跑任一路都正常，极易误判成"硬件不支持全双工"。
- 可用顺序：开播放开关 → 等 700ms → 起 aplay → 开采集开关 → 等 1.5s → 起 arecord。
- 子进程一律挂 stderr 缓冲并在错误里回传，否则 EPIPE/EIO 只能看到"broken pipe"，定位不到 ALSA 真正报错。
