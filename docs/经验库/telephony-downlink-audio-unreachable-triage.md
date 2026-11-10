---
name: telephony-downlink-audio-unreachable-triage
description: 通话能正常建立却抓不到对端语音（AudioRecord 读出全零）时，如何区分是采集源选错，还是该设备压根没有音频硬件落点
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-07
 tags: [android, telecom, audiorecord, voice-downlink, audio-hal, tinymix, oem-rom]
---
## 适用场景
在安卓设备上通过 Telecom 打通了通话信令（拨打/接听/挂断正常），但音频桥采集到的通话下行恒为静音、字节数在涨而峰值全是 0 时使用。典型对象是随身 WiFi、模块化 CPE、无屏无喇叭的定制设备。

## 关键做法
先证伪"设备本身没有音频落点"，再谈换采集源，顺序别反：
1. `dumpsys audio` 看 `Active communication device` 与输出设备清单。若只有形如 `earpiece:<设备名>`、`speaker` 且地址/name 为空、profiles 也为空的影子设备，没有任何真实声卡条目，基本可判无音频硬件。
2. 查原生音频节点：`/dev/snd` 是否只有 timer、`/proc/asound/pcm` 是否为空、`tinymix` 是否能打开 mixer。三者都不成立（打不开 mixer、无 PCM 节点）说明改 ALSA 路由这条路不存在，不要再花时间试 mixer。
3. 核对厂商自定义音频通道属性（如展锐的 `ro.vendor.ag.pcm`、`ro.hardware.audio.primary`），确认其指向的 `/dev/*` 节点是否真的存在。
4. 采集源要逐一实跑而非看能否启动：`VOICE_DOWNLINK(3)` 与 `VOICE_CALL(4)` 都要真正跑一段通话并读 `pcm_down_peak`；"能 startRecording 且字节在涨"不代表有内容，全零会以 `pcm_capture=all_zero` 形式暴露。
5. 若以上全部指向无硬件，则结论是音频天花板，不应再改 App 侧选源逻辑；把采集源还原成上游语义，保持信令可用即可。

## 踩过的坑
仅凭采集能启动、字节数持续增长就判定采集正常，会误把全零数据当成"有声音"；必须看峰值。把"换采集源"当成万能修复，在无 codec 的设备上会反复空转——第 1、2 步就能提前否掉。另外 OEM 固件可能把 `audioserver` 默认关闭、再靠 Magisk 模块补驱动后才恢复，看到音频服务在跑也不等于有真实输出通路。

## 可复用的产物或模板
（无）