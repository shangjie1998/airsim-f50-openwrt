# F50 与 M8 的差异对照

两台都是紫光展锐 `UMS9620`，都跑 AirSIM 通话网关，但**内核线、音频路由、版本口径全不一样**。
**安装包与文档绝不可互换**，也不要在对方的源码树里改东西。

## 1. 硬参数对照

| 项 | 中兴 F50（本仓库） | VIPME-M8 |
|---|---|---|
| 适配仓库 | `airsim-f50-openwrt` | `airsim-m8-openwrt` |
| 内核 | **6.18.60**（mainline `mu300-linux`） | **6.18.54-m8-probe12**（自建变体） |
| 系统存放 | TF 卡 B 卡槽（A 卡槽 Android 不动） | 内置 `e5root` 分区 `/dev/mmcblk0p74` |
| 声卡 | `card1 = sprdphone-sc2730` | `card0 = sprdphone-sc2730` |
| 通话采集 | `FE_ST_VOICE_PCM_C` = `hw:1,16` | `FE_ST_VOICE_C` = `hw:0,9` |
| 通话播放 | `FE_ST_VOICE_PCM_P` = `hw:1,53` | `FE_ST_VOICE_PCM_P` = `hw:0,18` |
| 播放采样率 | **8 kHz 直通** | **16 kHz**（走上采样，否则对端听到 2 倍速） |
| 网络 | `192.168.77.1` | `192.168.9.1` |
| 音频模块来源 | **自编**（上游 kit 只钉 6.18.58/59，全 404） | 上游/自建 kit |
| 版本口径 | `0.1.0.<N>-r<NN>-6.18` | `0.1.0.<N>-r<NN>-m8` |
| 当前版本 | `0.1.0.5-r23-6.18` | `0.1.0.26-r22-m8` |

## 2. 代码组织差异

| 项 | F50 | M8 |
|---|---|---|
| 上游源码 | `src/`（v0.1.0-r23 原样） | 直接在工作树里改 |
| 个性化改动 | `overlay/f50/`（`strings.tsv` + `files/` + `apply.py`） | `patches/` + `m8-pkg/` |
| 构建 | `sh overlay/f50/build.sh`（本机 Go 交叉编） | `docs/M8_BUILD_AND_ADAPT.md` 里的脚本 |
| LuCI | 复用上游页面，只换文案 | 复用上游页面 + 自建来电通知页 |

## 3. 移植时最容易踩的坑

1. **采样率抄错**：把 M8 的 16 kHz 上采样搬到 F50，会让对端听到加速；把 F50 的 8 kHz 直通搬到 M8，会变声。
2. **声卡号抄错**：F50 是 `card1`（`hw:1,x`），M8 是 `card0`（`hw:0,x`），差一就整条音频不通。
3. **内核模块混用**：两台的 `.ko` vermagic 不同，装上直接 `invalid module format`；F50 必须用 `modules/6.18.60/`。
4. **安装包混用**：`install.sh` 里的内核闸门按各自的内核写，交叉装会退 3 或装坏。
5. **版本号混用**：末段机型标识不同，混了以后 Release 里两个包对不上号。

## 4. 共性（可以互相借鉴的部分）

- 显示名只改文案、内部标识保留（`agent_kind=mu300`、`AIRSIM_MU300_MODE/RELAY`、二进制 `airsim-mu300d`）
- 真机验收判据同一套：模块加载数 / 内存缺页 / 声卡节点 / PCM 通路 / dmesg 命令返回
- 通话验收看 `/status` 的 `downlink_frames`、`uplink_frames`、`dropped_uplink_frames`
- 都不远程 `reboot`，重启一律由使用者本人按
