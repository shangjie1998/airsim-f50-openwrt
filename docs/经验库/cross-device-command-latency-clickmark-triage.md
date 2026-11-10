---
name: cross-device-command-latency-clickmark-triage
description: 跨设备 App 远程命令（发短信、拨号等）用户反馈"有延迟"时，用点击时刻标记对齐设备日志，把延迟拆到具体环节并排除无关通道噪声
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-07
 tags: [延迟归因, logcat, 时间线对齐, 跨设备, 短信, 诊断]
---
## 适用场景
手机 A 的中控 App 把操作（发短信、拨号、开热点等）发给手机 B 执行，用户感觉"慢"，需要判断慢在 App 链路、权限通道还是运营商/远端。
## 关键做法
1. 先在执行端起全量抓取：`adb logcat -v time > x.logcat`，并 `adb logcat -c` 清旧缓冲，记录后台 pid。
2. 让用户只操作一次，并请他在按下按钮后立刻在聊天里打一个标记（如"1"）。用标记消息的时间反推点击时刻，与日志逐行对齐。
3. 在日志里定位三个锚点：请求落地（控制转发/端点选择日志）、交给系统执行（如 `ImsSmsImpl sendSms`）、系统回执（`onSendSmsResult`）。三段相减即得各段耗时。
4. 结论按段下：请求落地→交系统是 App 内部开销，交系统→回执是基带/运营商开销，点击→落地是网络往返开销。只有网络往返和 App 内部两步能改。
## 踩过的坑
执行端日志里常有另一条通道的退避噪声（形如 `standalone_command_retry reason=network tier=N delay_ms≈60000`，逐级涨到几十秒）。它是云端/长轮询通道的失败重试，与被测的局域网直连命令无关，别当成本次延迟主因；先用锚点时间戳确认被测请求走的是哪条路径再下结论。
## 可复用的产物或模板
（无）