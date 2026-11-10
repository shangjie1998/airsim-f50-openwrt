---
name: audio-extension-per-kernel-modules-diagnose
description: 语音插件在换内核后能跑但无声、服务不自启时，判定是音频模块未针对该内核编译，而不是内核版本本身不兼容
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [openwrt, 内核切换, 音频模块, 预检, 电话语音, 诊断]
---
## 适用场景
设备换了内核后，通话插件进程能手动拉起、界面和设备名正常，但语音服务不自启、通话没声音；或需要判断"别人某个内核能跑"是否适用于自己这台。

## 关键做法
按证据链三层确认，不要只看 `uname -r` 大版本：
1. 读音频预检脚本里的硬性条件（如 `[ "$(uname -r)" = <完整 release 串> ] || fail`），直接手动执行预检，看重试错串。
2. 解开随插件的音频扩展包，看模块目录名：`tar tzf <ext>.tar.gz | grep -o 'modules/[^/]*/' | sort -u`。只有某一内核版本的目录，说明模块只为那一版编过。
3. 在当前内核下数模块与声卡：`ls /lib/modules/$(uname -r) | grep -c '^audio_'`、`grep -c agdsp`、`cat /proc/asound/cards`。语音专有模块为 0、声卡只剩 Loopback，即硬件声卡从未注册。
对"别人能跑"的对照，用采集/播放节点与产品名区分机型；`hw:0,x` 与 `hw:1,x`、不同 product 基本不是同一硬件路线。

## 踩过的坑
不要把"能装的包、能起的进程、正确的设备名"当成语音可用；预检失败会让 procd 停在 `active with no instances`，而手动起 daemon 恰恰掩盖了这个状态。
不要以为换更大的内核版本号就能通；音频模块是按 release 串与内核配置编译的，缺模块时只能回旧内核，或用音频源码按目标内核重编整组模块并同步改预检的期望值（需对应 SDK 与内核头文件）。

## 可复用的产物或模板
```sh
tar tzf /usr/lib/airsim/deps/*.tar.gz | grep -o 'modules/[^/]*/' | sort -u
ls /lib/modules/$(uname -r) | grep -c '^audio_'
cat /proc/asound/cards
/usr/lib/airsim/audio-preflight; echo "rc=$?"
```