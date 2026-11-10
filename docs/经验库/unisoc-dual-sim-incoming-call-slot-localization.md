---
name: unisoc-dual-sim-incoming-call-slot-localization
description: 双卡 Unisoc/OpenWrt 设备上判定来电落在哪个卡槽、或副卡来电通知完全不触发时，用逐槽 AT+CLCC 定位，而不是指望上层事件或单条 modem 通道带卡槽信息
metadata:
    type: work_experience
    source: harness
    created_at: "2026-11-08"
    tags:
        - at命令
        - clcc
        - 双卡
        - 卡槽定位
        - unisoc
        - openwrt
        - 来电
---
## 适用场景
双卡设备（Unisoc 平台、OpenWrt）上需要知道一通来电/通话落在哪个卡槽，或发现副卡来电的通知一条都不发、上层通话视图里看不到它。
## 关键做法
按槽逐条发 `AT+CLCC`，谁的响应对应正在进行的通话，就是哪个槽：
```sh
for s in 0 1; do
  MU300_SIM_SLOT=$s /usr/libexec/unisoc-modem/at -t 6 "AT+CLCC"
done
```
判定看响应字段：`+CLCC: <idx>,<dir>,<stat>,<mode>,<mpty>,<number>,<type>`，`dir=1` 为来电，`stat=4/5` 为振铃、`stat=0` 为接听。落在的槽会返回这条，另一槽只回 `OK`。
先用 `AT+CCID` 确认两槽各自能独立读到，说明按槽寻址本身可用。
默认槽可读内核参数 `/sys/module/sipc_wwan/parameters/card`；上游面板即用「每请求一个 slot 参数、缺省取该值、导出 `MU300_SIM_SLOT`」的模式，可直接照抄。
需要把槽位往下游传递时，把槽号存进自己的状态文件并作为模板占位符输出，同时用该槽自己的号码（`AT+CNUM` 按槽取）填充「本机号码」一类字段，否则永远显示主卡号。
## 踩过的坑
- 上层通话事件（安卓侧 `call_id/direction/state/number/mode` 之类）天生不含卡槽，别在那里找，加了也拿不到。
- 设备上若存在把 AT 转发给 mmcli 的 shim，它只对一个 modem 生效，等于只看得见主卡；**已实测确认**：卡槽1 来电时上层视图的 `active` 为 `null`，副卡来电在该链路完全不可见、一条通知都不会发。因此必须把逐槽 CLCC 作为唯一数据源，不能把单 modem 的上层视图当主路径或兜底。
- 一次查询只能看一个槽，两槽结果必须分别取，不能指望一条响应同时报两卡。
- 起呼瞬间数据面常短暂中断，投递可能拿到 `http_code=000`；此时应入队重试而不是丢弃，否则真来电的通知就白丢一条。
## 可复用的产物或模板
```sh
MU300_SIM_SLOT=$s /usr/libexec/unisoc-modem/at -t 6 "AT+CLCC"
cat /sys/module/sipc_wwan/parameters/card
```