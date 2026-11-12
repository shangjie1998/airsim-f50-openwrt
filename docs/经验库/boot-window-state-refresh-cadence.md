---
name: boot-window-state-refresh-cadence
description: 设备重启后客户端长时间看到「号码未提供」/某状态项为空、要等一个外部动作（打一通电话）才出现时，怎么判定是开机空窗而非真丢数据，并把刷新节奏按开机窗口改掉
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-12
 tags: [嵌入式, 开机窗口, 刷新节奏, cnum, 时间线采样, 静默失败, openwrt]
---
## 适用场景
设备重启后，客户端一直显示某个状态项为空（典型是本机号码「号码未提供」），用户打一通电话或做别的动作后立刻就有了。要判断这是采集失败、数据丢失，还是开机窗口里资源还没就绪、而固定间隔的轮询把「刚就绪的那一刻」漏掉了。

## 关键做法

**先抓时间线，再改代码。** 别拿「现在有值」当修好了——那可能是旧节奏碰巧赶上，或者用户已经打过电话触发下发，没有首现时刻就没有证据。

1. 采样脚本必须**先等设备掉线、再等恢复**。重启命令下发后设备可能几分钟才连得上；手动等 ping 通了再采，往往已经过了 0 秒窗口（本次第一回采样时 `uptime` 已经 298 秒，整段白采）。
2. 三个量一起记：**本机经过秒数、设备 `/proc/uptime`、目标接口的字段值**。没有 uptime 就分不清「服务还没起、接口整条无响应」和「服务起了但字段是空的」——这两种在输出上一模一样。
3. 服务刚起时 SSH 可能 `Connection refused`（dropbear 比网络栈晚），采样要带重试，别把这一次失败当终点。
4. 拿到首现点后用设备侧时间戳交叉验证：`phone_number_updated_at`、`last_at_success_at`、进程 `start_ticks`，避免只信客户端单侧。
5. 修复方向选「按开机窗口加密重试」，不要选「换数据源」：换源（读 USIM 电话簿、`AT+CUSD` 之类）各家卡差异太大，容易开新坑；加密重试只影响开机头几分钟的 AT 频率，拿到值或超时自动回落，代价可控。
6. 判据抽成纯函数再单测（如 `numberRetryDue(n, uptime, number)`），边界逐条钉死：正好到窗口末尾、已有值不加速、非加密周期点不触发。节奏类逻辑不做单测，改完没法回归。

## 踩过的坑
- `AT+CNUM` 读的是 SIM 里存的 MSISDN，不少运营商卡出厂不写，或者要等 SIM 初始化/网络注册完成后才回值。固定 30 秒一轮时，只有第 0 秒和第 30 秒两次机会；如果第 30 秒那次还早，就要再等 30 秒，客户端这段时间一直显示未提供。
- 只按「当前有号码」判断修复成功，是最容易犯的错。本次实测的正确证据是：从开机 0 秒起采，`uptime` 20–34 秒接口无响应（agent 未起），**`uptime` 36 秒首次读到号码，全程没打电话**。旧节奏在这一刻没有采样点，本来就抓不到。
- 设备重启属于阻断型操作，自动化下发会被系统拦，必须交用户手动下发；不要换写法（拆命令、改脚本、走别的通道）绕过去。

## 可复用的产物或模板
```
# 1) 等掉线（确认重启真的开始）  2) 等恢复  3) 立刻从开机早期采样
until ! ping -c 1 -W 1000 $DEV >/dev/null 2>&1; do sleep 1; done
until ping -c 1 -W 500 $DEV >/dev/null 2>&1; do sleep 0.5; done
ssh $DEV "for i in \$(seq 1 50); do
  t=\$(date -u +%H:%M:%S); u=\$(cut -d. -f1 /proc/uptime)
  h=\$(wget -qO- http://127.0.0.1:8575/api/health)
  echo \"\$t uptime=\$u \$(echo \$h | grep -o '\"phone_number\":\"[^\"]*\"')\"
  sleep 1; done"
```

节奏判据（Go，已在 0.1.0.28 落地，对应 `mu300PollLoop` + `numberRetryDue`）：
```go
func numberRetryDue(n int, uptime time.Duration, number string) bool {
    if n%30 == 0 { return true }                              // 常规 30 秒一轮
    return uptime < 3*time.Minute && number == "" && n%5 == 0  // 开机 3 分钟内、无号时 5 秒一轮
}
```
实测效果：开机 36 秒出号（旧节奏最快要等到 30 秒采样点且那一次必须成功）；通话后号码不丢，`phone_number_updated_at` 照常刷新。
