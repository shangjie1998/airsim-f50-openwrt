# F50 蓝牙（与 AirSIM 插件无关的独立能力）

中兴 F50 的 WCN（展锐 `sprd-marlin3`，WiFi + 蓝牙共用）在 OpenWrt 6.18 上**可以被拉起来**，
这里放的是那次适配的全部脚本与文档，和 `overlay/f50/`（AirSIM 插件改名）不是一回事：
**蓝牙不影响通话，也不是 AirSIM 插件的一部分**，装不装都不影响 `airsim` 服务。

| | |
|---|---|
| 适用设备 | 中兴 F50（`MU300` / `UMS9620`），OpenWrt `25.12.5`，内核 **6.18.60** |
| 适配文件 | [`f50/`](f50/) —— attach 脚本、procd init、一键部署、开关脚本 |
| 原始归档 | [`../dist/F50-BT-OpenWrt.zip`](../dist/F50-BT-OpenWrt.zip)（2026-11-09 交付原包） |
| 技术细节 | [`f50/README.md`](f50/README.md) —— 控制器参数、为什么必须 btattach、两个 rfkill 节点、已知坑 |

## 目前已验证到哪一步（2026-11-09）

| 项 | 状态 |
|---|---|
| 驱动 | `sprdbt_tty` 已加载，`/dev/ttyBT0`（`493,0` root:dialout） |
| HCI | `hci0`，Primary / Bus UART，**UP RUNNING**，地址 `27:93:31:14:22:11` |
| 用户态 | BlueZ 5.83 + dbus，`bluetoothd` 常驻 |
| 自启 | `S60dbus` → `S61sprdbluetooth` → `S62bluetoothd`（都已 enable） |
| 扫描 | 经典扫描、LE 扫描都实测扫到设备 |
| **整机重启** | **未验证**（只做过 kill 服务 / `rmmod` 后的冷启动模拟） |
| 蓝牙音频 | 未做。A2DP 可行、HFP/SCO 风险高、LE Audio 这颗 5.0 芯片不支持 |

## 怎么装

```sh
# 把 bluetooth/f50/ 四个脚本传上去，cd 进去
sh deploy.sh
```

干净固件首次部署用 `deploy.sh`；已有环境的，**只重放 `sprdbluetooth.init` 与 `bt-switch.sh` 两个文件**即可
（init 现在带开关闸，旧版没有）。

## 蓝牙总开关（给页面留的口子，已可用的 CLI 接口）

页面还没做（M8 那边在做 UI），但**服务侧接口今天就能用**：

```sh
/usr/sbin/f50-bt-switch.sh on       # 开：拉起 dbus → sprdbluetooth → bluetoothd
/usr/sbin/f50-bt-switch.sh off      # 关：停 bluetoothd、sprdbluetooth，hci0 消失
/usr/sbin/f50-bt-switch.sh status   # 打印 enabled=… hci0=yes|no
```

约定，页面接上去时按这个来：

| 项 | 值 |
|---|---|
| 状态存放 | `/etc/config/f50-bt` → `f50-bt.settings.enabled`（`1` / `0`） |
| 开机行为 | `/etc/init.d/sprdbluetooth` 的 `start_service` 先读该值，`0` 就直接跳过 → **关了重启仍是关** |
| 未配置时 | 按"开"处理，保证首次部署即可用 |
| LuCI 调用 | 页面 JS 里 `fs.exec('/usr/sbin/f50-bt-switch.sh', ['on'|'off'])`，不用新增 rpcd 白名单 |
| 展示用的状态 | 开关值读 `f50-bt.settings.enabled`；运行态看 `/sys/class/bluetooth/hci0` 是否存在 |

`dbus` 与 `bluetoothd` 在"关"状态下不强制停 dbus（系统其它服务也可能用它）；`bluetoothd` 会停，
所以**关了之后不会再有蓝牙守护进程空转**。

## 与 AirSIM 的关系（为什么最后没做成"蓝牙电话"）

想法是让 AirSIM 走蓝牙耳机通话，实测判断走不通，记在这儿免得下次重走：

- 这颗 WCN 的 **SCO 音频不走 HCI**，走片内 `/dev/audio_pipe_bthal` 直达 `ump9620` codec，
  那是 Android vendor HAL 才认的路径；BlueZ 在 OpenWrt 上大概率拿不到 SCO 数据流。
- 控制器只到 **BT 5.0**，没有 ISO/LC3 → **LE Audio 不可行**。
- 结果：蓝牙当"能连、能扫、能传文件/音乐（需另装 pulseaudio）"用没问题，当通话承载不行。
- 通话仍走 AirSIM 原有 PCM 桥（`card1 sprdphone-sc2730`、`hw:1,16` / `hw:1,53`）。
