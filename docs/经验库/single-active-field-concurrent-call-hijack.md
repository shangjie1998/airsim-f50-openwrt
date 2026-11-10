---
name: single-active-field-concurrent-call-hijack
description: 通话状态对外只暴露一个 active 字段、同时可能有多路通话时，来电把 active 抢走会让挂断按钮打错目标，怎么排序并验证
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [通话状态机, 双卡, 呼叫等待, 定向挂断, 状态优先级, 静默错误]
---
## 适用场景
设备侧通话状态对外只有一路 `active`（外加 calls 数组/多条记录），而基带可能同时存在一路接通、一路来电/保持/等待时。上层 App、网页或推送按 `active` 去挂断、静音、发 DTMF。

## 关键做法
1. 排序规则里，"来电/等待"不能排在"已接通"之前的优先级——否则新来的振铃会把 active 抢走。
2. 单独给 active 槽定优先级：active > incoming/waiting > alerting > dialing > held，让 active 始终指向正在进行的那通；其余各路留在 calls 数组里照样可见。
3. 挂断命令按目标状态分支：一路 active + 一路 waiting 时挂等待方用 `AT+CHLD=0`（只挂等待），不要用 `CHUP` 一锅端；挂 active 方用 `AT+CHLD=1x` 按序号定向。
4. 验证方式：制造"一通接通 + 一通来电等待"，读 active 确认指向接通那通；只挂等待方，再用逐槽 CLCC 确认接通那通仍是 active。

## 踩过的坑
- 优先级表里 incoming=5、active=4 这种"越大越紧急"的写法看着合理，实际让振铃抢走了 active。上层照 active 挂断，挂的是来电而不是正在说的话。
- `AT+CHUP` 在只有一路时正确，在多路时会把不该挂的也挂掉，必须按状态换成 CHLD 定向形式。
- 跨卡来电可能不带卡槽标记、落在主上下文里（见逐槽 CLCC 定位那类经验），定向挂断不能依赖 slot 字段。

## 可复用的产物或模板
（无）