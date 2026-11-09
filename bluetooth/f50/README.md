# F50 在 OpenWrt 6.18 下启用蓝牙

设备 `192.168.77.1` · OpenWrt `25.12.5`（`armsr/armv8`，aarch64_generic，musl）· 内核 **6.18.60**
芯片：展锐 Unisoc WCN combo（DT 里 `sprd-marlin3`），WiFi 与蓝牙共用一棵 WCN。

> 先说三条最容易踩空的：
> 1. **Android 的 Magisk 模块（`F50音频与蓝牙修复_V1.0.0.zip`）在 OpenWrt 上一条都用不了** ——
>    `insmod` / `setprop` / `svc bluetooth enable` 都没有。它唯一有用的信息是：这颗 WCN 必须加载
>    `sprdbt_tty` 才有 `/dev/ttyBT0`。
> 2. 本机**没有 opkg，只有 apk（apk-tools 3.0.5）**，源已在 `/etc/apk/repositories.d/distfeeds.list` 配好。
> 3. `/etc/apk/world` 里钉的是 `kernel=6.12.94`，**实际跑 6.18.60** —— 装 BlueZ 会拉进一堆没用的 6.12.94 kmod。

## 最终状态（已验证）

| 项目 | 状态 |
|---|---|
| 驱动 | `sprdbt_tty` 已 `modprobe`，`/dev/ttyBT0`（root:dialout） |
| HCI | `hci0`，Primary / **Bus UART**，`UP RUNNING`，地址 `27:93:31:14:22:11` |
| 用户态 | BlueZ 5.83 + dbus，`bluetoothd` 常驻 |
| 自启 | `/etc/init.d/sprdbluetooth`（procd，`START=61`）+ `S60dbus` + `S62bluetoothd` |
| 累计错误 | 481+ events / 148+ commands / **errors: 0** |

> **未验证的一项**：整机重启后是否能自恢复。只做过"kill btattach + `rmmod sprdbt_tty` 再起服务"的模拟。
> 重启后自查：`ls /sys/class/bluetooth`、`hciconfig -a`、`ps | grep -E "btattach|bluetoothd"`。

## 控制器实测参数

```
HCI Version: 5.0 (0x9)   Revision: 0x400
LMP Version: 5.0 (0x9)   Subversion: 0x400
ACL MTU: 1021:8   SCO MTU: 240:3
Features: 0xff 0xff 0x8d 0xfe 0xdb 0x7d 0x3b 0xc3
```

| 能力 | 结论 |
|---|---|
| BR/EDR | 支持 |
| BLE | 支持（LE + BR 同时工作） |
| **LE Audio** | **不支持**（需 5.2+ ISO/LC3，这颗 5.0） |
| EDR 带宽 | 2Mbps / 3Mbps，A2DP 够 |
| 语音编码 | CVSD / A-law / u-law / 透明 SCO |
| LE LL Privacy | 不支持，`bluetoothd` 日志固定出现 `Failed to set privacy: Rejected (0x0b)`，无害 |

实测：经典扫描扫到小米电视、Redmi Note 5；LE 扫描扫到 `RM Magcooler 5pro`、`BYD BLE3` 等；
裸 H4 帧 `Reset` 回 `Command Complete status=0`。

## 蓝牙显示名：F50-<控制器地址后 6 位>

别的手机搜到的名字是 **`F50-142211`**（本机实测），规则是取 `hciconfig hci0` 的
`BD Address` 后 6 位、去冒号、转大写：

```
BD Address: 27:93:31:14:22:11  ->  142211  ->  F50-142211
```

为什么用控制器地址而不是序列号/设备标识：

| 候选 | 长度 | 稳定性 | 结论 |
|---|---|---|---|
| BD Address 后 6 位 | 6 | 每台控制器唯一，重启不变 | 采用 |
| 设备序列号（`009238d10a2acb06`） | 8+ | 稳，但蓝牙 attach 早于相关服务启动 | 备选 |
| 页面「设备标识」（`0p0JKELA`） | 8 | 由 `airsim-agent` 生成，attach 时未必就绪 | 不用 |
| 随机值 / 时间戳 | — | 每次开机都变，手机端会堆一堆配对记录 | 否决 |

实现落在两处：

- `sprd-bt-attach.sh`：hci0 上电成功后执行 `hciconfig hci0 name "$_btname"`。
  attach 由 procd 常驻拉起，**重启后自动重设**，不用手工写 nvram。
  取不到地址（`hciconfig` 异常）时 `_suf` 为空，名字退回固定 `F50`，
  不会出现 `F50-` 这种半截名字。
- `/etc/bluetooth/main.conf` 的 `Name = F50`（deploy.sh 里 sed 写入）：
  只作 BlueZ 兜底，避免名字回落成 `mu300`。

**取列的坑**：`hciconfig` 那行长这样 ——
`BD Address: 27:93:31:14:22:11  ACL MTU: 1021:8  SCO MTU: 240:3`，
地址是**第 3 列**，用 `awk '{print $4}'` 拿到的是 `ACL`，名字会变成 `F50-`。

## 部署

```sh
modprobe sprdbt_tty          # 固件里本就有这个 ko，只是没加载
apk update
apk add bluez-libs bluez-utils bluez-utils-extra bluez-daemon dbus
cp sprd-bt-attach.sh /usr/sbin/sprd-bt-attach.sh && chmod 755 /usr/sbin/sprd-bt-attach.sh
cp sprdbluetooth.init /etc/init.d/sprdbluetooth && chmod 755 /etc/init.d/sprdbluetooth
cp bt-switch.sh /usr/sbin/f50-bt-switch.sh && chmod 755 /usr/sbin/f50-bt-switch.sh
sed -i 's/^#AutoEnable=true/AutoEnable=true/' /etc/bluetooth/main.conf
/etc/init.d/dbus enable; /etc/init.d/sprdbluetooth enable; /etc/init.d/bluetoothd enable
/etc/init.d/dbus restart; /etc/init.d/sprdbluetooth restart; /etc/init.d/bluetoothd restart
```

或直接 `sh deploy.sh`（顺序已串好，干净固件首次部署用）。

## 两个关键实现细节（别自作聪明改）

**① attach 一律交给 `btattach`。** 对 `/dev/ttyBT0` 手工 `TIOCSETD` 设 `N_HCI`(15) 会成功，
但紧接着 `HCIUARTSETPROTO`（`0x400455C8`）返回 `EPROTONOSUPPORT`，协议号 0~15 全试遍都失败；
系统 `btattach` 用同一套机制却能拉出 `hci0`。

**② rfkill 有两个蓝牙节点，只解一个是白解。**
`rfkill1 name=bluetooth`（`sprdbt_tty` 注册，管 WCN 上电）、`rfkill2 name=hci0`（attach 后出现）。
只解 rfkill1 时 `hciconfig hci0 up` 报 `No error information (132)`（musl 下 132 = `ERFKILL`）。
两个都要写 0；之后 dmesg 可见 `WCN BASEstart_marlin [MARLIN_BLUETOOTH] power 1`。

## 开关（页面口子）

`/usr/sbin/f50-bt-switch.sh on|off|status`，状态存 `/etc/config/f50-bt` → `f50-bt.settings.enabled`，
`sprdbluetooth` 启动时读它，`0` 就跳过 → 关了重启仍是关。详见 [../README.md](../README.md)。

## 日常使用与排查

```sh
bluetoothctl            # scan on / scan le / pair / trust / connect / info
hciconfig -a
logread | grep -i -E "bluetooth|sprd"
```

`hcitool lescan` 在这台固件上报 `Set scan parameters failed: I/O error`，换
`bluetoothctl --timeout 15 scan le` 就能扫到，别以为 LE 坏了。

## 已知坑

1. **装 BlueZ 会带进 10 个"死" kmod**（6.12.94 版本，落到 `/lib/modules/6.12.94/`），运行时完全无效，
   modprobe 只认 `uname -r`。本次蓝牙一个 kmod 都没用到 —— 不要据此判断"驱动是装包装的"。
2. **音频**：硬件通路在（`sprdphonesc2730` 声卡、`/dev/audio_pipe_bthal`），但用户态没有任何音频软件。
   A2DP 可行（需装 pulseaudio + `module-bluetooth-discover`）；HFP/SCO 风险高（SCO 音频不走 HCI）；
   LE Audio 不可行（5.0）。**没有尝试做"蓝牙通话"**，AirSIM 通话仍走原 PCM 桥。
3. **稳定性边界**：已验证服务被 kill、驱动 rmmod 后自动恢复；未验证连接态长时间 soak 与整机重启。

## 回滚

```sh
/usr/sbin/f50-bt-switch.sh off
/etc/init.d/sprdbluetooth disable && /etc/init.d/bluetoothd disable
apk del bluez-daemon bluez-utils bluez-utils-extra dbus
```

## 文件清单

| 文件 | 部署位置 | 说明 |
|---|---|---|
| `sprd-bt-attach.sh` | `/usr/sbin/` | modprobe → 等 `/dev/ttyBT0` → 解 rfkill → 等 `hci0` up → `exec btattach` |
| `sprdbluetooth.init` | `/etc/init.d/sprdbluetooth` | procd 服务，`START=61`，`respawn 3600 5 0`，**带开关闸** |
| `bt-switch.sh` | `/usr/sbin/f50-bt-switch.sh` | 蓝牙总开关，状态写 `/etc/config/f50-bt` |
| `deploy.sh` | 不部署 | 干净固件一键部署 |
