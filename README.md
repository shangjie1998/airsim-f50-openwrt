# AirSIM for F50 — 把通话网关装进 OpenWrt 手机（**6.18 内核主线**）

把 AirSIM 的通话网关（配对、控制面、通话音频桥、短信）装进一台跑 OpenWrt 的中兴 F50
（`MU300` / 紫光展锐 `UMS9620`），让 iPhone 在同一局域网里用这台手机的 SIM 卡接打电话、收发短信。

**不是刷机包，不是 APK，不改 A 卡槽的 Android。** OpenWrt 跑在 TF 卡（B 卡槽），
本仓库只往 OpenWrt 的 `/usr/lib/airsim` 与音频扩展目录里落地文件，一条命令可完整卸载。

| | |
|---|---|
| 本适配仓库 | <https://github.com/shangjie1998/airsim-f50-openwrt> |
| 上游项目 | <https://github.com/kai-wu-cortex/airsim-mu300>（本仓库源码钉在 `v0.1.0-r23`） |
| 已验收版本 | **`0.1.0.5-r23-6.18`**（2026-11-09 真机通话双向有声） |
| 插件安装包 | [Release `v0.1.0.5-r23-6.18`](https://github.com/shangjie1998/airsim-f50-openwrt/releases/tag/v0.1.0.5-r23-6.18) · 5.7 MB（仓库内另有 [`dist/`](dist/) 同款） |
| 音频模块包 | `0.1.0.4-r22-6.18` 里的 25 个 `.ko`（内核绑死 6.18.60，见 §3） |
| 接手说明 | [docs/6.18/接手说明.md](docs/6.18/接手说明.md) —— 现状、装机三步、判活五条、未修复问题 |
| 编译与复现 | [docs/6.18/F50-6.18音频自编.md](docs/6.18/F50-6.18音频自编.md) —— 自造 kit、四个编译坑 |
| 与 M8 的差异 | [docs/F50-vs-M8.md](docs/F50-vs-M8.md) |
| 5.4 归档线 | [docs/5.4/](docs/5.4/) —— 已冻结，只在归档里留着 |

---

## 1. 适配环境（全部为真机实测值）

| 项目 | 实测值 |
|---|---|
| 机型 | **中兴 F50**（`MU300`），`Spreadtrum UMS9620`（与 VIPME-M8 同 SoC） |
| 系统 | **OpenWrt 25.12.5 `r33051-f5dae5ece4`**，target `armsr/armv8`，arch `aarch64_generic`，跑在 **TF 卡 B 卡槽** |
| 内核 | **`6.18.60`**（mainline，`mu300-linux` 那条分支；**非**厂商 5.4） |
| 声卡 | `card1 = sprdphone-sc2730` |
| 通话采集 | `FE_ST_VOICE_PCM_C` = `hw:1,16` |
| 通话播放 | `FE_ST_VOICE_PCM_P` = `hw:1,53`（8 kHz 直通，不做上采样） |
| 音频扩展目录 | `/mnt/mu300-disk/extra/airsim-audio/`（`bin/ ctlset etc/ integration/ modules/<KREL>/`） |
| 地址 / 账号 | `192.168.77.1`，root |
| SIM | 实测 `8617702465721`，`已注册`，`phone_number_status=available` |

**安全边界（硬约束）**：**A 卡槽的 Android 系统一个字节不动**（不刷机、不碰 boot / bootloader / 分区表 / modem NV）；
只动 B 卡槽 TF 卡上的 OpenWrt；设备重启是高危动作，**由使用者本人按**，任何自动化都不代劳。

## 2. 仓库结构：上游原版 + 个性化叠加

上游只发**剥掉 `.git` 的 sources 包**，所以本仓库不靠 `git archive` 构建，改成"两份树合成"：

| 目录 | 内容 |
|---|---|
| `src/` | **上游 `v0.1.0-r23` 原样**，一个字未改（升级时整棵换掉即可） |
| `overlay/f50/` | 本适配的全部个性化改动：`strings.tsv`（文案替换规则）、`files/`（LuCI 页面与 acl 整文件）、`apply.py`、`build.sh` |
| `upstream/r23/` | 上游来源、校验锚点、r22→r23 差异文件清单 |
| `pkg/airsim-f50-0.1.0.5-r23-6.18/` | 当前插件包（二进制 + LuCI + `install.sh` + `MANIFEST` + `VERSION`） |
| `archive/5.4-pkg/` | **5.4 覆盖包归档，不删只挪**，主页与脚本都不再引用 |
| `docs/6.18/`、`scripts/6.18/` | 6.18 线文档与设备侧脚本 |
| `docs/5.4/`、`scripts/5.4/` | 5.4 冻结线归档 |
| `kbuild/` | 自编内核模块用的 kit 与源码树（`.gitignore`，不入库） |

相对上游改了什么（**只改显示文案，标识一律保留**）：

| # | 差异 | 位置 |
|---|---|---|
| 1 | 显示名 `AirSIM for F50`；mDNS 前缀 `AirSIM-F50-`；云端上报 `device_name=AirSIM F50` | `main.go`、`management.go`、`gateway.go`、LuCI |
| 2 | LuCI 三个页面与 acl 描述里的 MU300 文案改 F50 | `overlay/f50/files/` |
| 3 | 音频设备参数（`Card 1` / `hw:1,16` / `hw:1,53`）走设备自检，不写死 | `internal/audio/devices.go` |
| 4 | **保留**：`agent_kind=mu300`、`AIRSIM_MU300_MODE/RELAY`、`NewMU300EarlyMediaBackend`、`android_model="MU300"`、二进制名 `airsim-mu300d`、路径 `/usr/lib/airsim` | — |

`overlay/f50/build.sh` 编之前会跑 `apply.py`：**任何一条文案规则一次都没命中就直接报错退出**，
上游哪天改了措辞，升级当场就能发现，不会静默漏改。

## 3. 直接安装（插件 + 音频模块，两步走）

### 3-1 装音频模块（只在没装过 `modules/6.18.60/` 时做，做完必须重启）

```sh
# 主机侧
scp dist/airsim-f50-0.1.0.4-r22-6.18-audio-modules.tar.gz root@192.168.77.1:/tmp/airsim-audio-6.18.60-modules.tar.gz
scp scripts/6.18/装6.18音频模块.sh root@192.168.77.1:/tmp/

# 设备侧
ssh root@192.168.77.1 'sh /tmp/装6.18音频模块.sh --install'
```

装完 **必须重启一次**：`audio_reserve` 要在开机 `S10` 抢那 9 MB 连续区，
运行中途 `insmod` 会报 `2304 pages not taken`，而且模块是 `[permanent]` 卸不掉。

重启后自检：

```sh
ssh root@192.168.77.1 'sh /tmp/装6.18音频模块.sh --verify'
```

| 判据 | 期望 |
|---|---|
| `mu300-audio-dsp drivers` | 打印 `/mnt/mu300-disk/extra/airsim-audio/modules/6.18.60` |
| `mu300-audio-dsp status` | `drivers: 19/19 loaded`、`memory: 0 pages missing`、`power-up: yes` |
| `cat /proc/asound/cards` | 出现 `1 [sprdphonesc2730]: sprdphone-sc2730` |
| `dmesg \| grep aud_send_cmd` | `out,cmd ... ret-value:0` |
| `wget -qO- http://127.0.0.1:7581/status` | `audio_available: true`，`devices = {Card:1, Capture:"hw:1,16", Playback:"hw:1,53"}` |

### 3-2 装插件包（覆盖式更新，先备份再换）

```sh
scp dist/airsim-f50-0.1.0.5-r23-6.18.tar.gz root@192.168.77.1:/tmp/r23pkg.tar.gz
ssh root@192.168.77.1 'cd /tmp && rm -rf airsim-f50-0.1.0.5-r23-6.18 && tar xzf r23pkg.tar.gz && ./airsim-f50-0.1.0.5-r23-6.18/install.sh'
```

`install.sh` 会先检查内核（非 6.18 直接退出码 3，不写设备一个文件），把旧二进制备份到
`/usr/lib/airsim/backup-<版本>/`，再停服务 → 换二进制与 LuCI → 起服务。

回滚：把备份目录里的 `airsim-mu300d`、`airsim-agent`、`VERSION` 拷回 `/usr/lib/airsim/` 后
`/etc/init.d/airsim restart`。

### 3-3 配对

LuCI → 服务 → AirSIM（或 `http://192.168.77.1/cgi-bin/luci/admin/services/airsim`）取配对码，
iPhone 端 AirSIM 添加设备时选 **`AirSIM-F50-…`**。

## 4. 已知限制与坑

| 现象 | 说明 |
|---|---|
| `arecord -D hw:1,16` 不通话时报 `Invalid argument` | **不是故障**：没有上行流就没得读；判活只能靠 dmesg |
| `aplay` 写 `hw:1,53` 恒报 `pcm_write: I/O error` | **假阴性**：没通话时 DSP 不消费 voice PCM；上游 `dsp_alive()` 拿它当探针，本机必须放宽 |
| `mu300-audio-dsp status` 的 `power-up` 偶显 `unknown` | `amixer` 在 `/usr/lib/airsim/bin`，不在默认 PATH；脚本必须显式带 `PATH=/usr/lib/airsim/bin:/opt/mu300/bin:/opt/mu300/busybox-bin:...` |
| 设备没有 `/usr/bin/timeout`、`/bin/timeout` | 只有 `/opt/mu300/busybox-bin/timeout`，`prepare.local` 的 PATH 已带上，别乱改 |
| 上游 `audio-preflight` / `audio-setup` / `stage-device.sh` 仍写死 5.4 | 6.18 走不了官方 `audio-setup --install`，只能按 §3-1 手装 |
| 音频模块的 CONFIG 哈希与设备不一致 | 设备 `233c7a62…` vs 自编 `d5628675…`，只差两个纯调试项；`set_fits()` 已放宽为"声明了 CONFIG 就放行" |
| iOS 已配对设备仍显示旧名 | 设备名缓存在 App 本地，改名后需删设备重新配对 |
| 签名 apk 一直没出 | 需要能跑 `apk-tools 3.x` 的 Linux；Mac 上没 Docker |
| r23 的 Tailscale 私网配对 / 云端缓冲协商 | **未在 F50 上验收**，本包只启用上游原有路径 |

## 5. 版本口径

`airsim-f50-0.1.0.<N>-r<NN>-<内核线>`，全小写，tag 前加 `v`：

- `0.1.0` 与上游一致，`r<NN>` 对齐上游 release（本版 `r23`）
- `<N>` 是本仓库打包序号，**每出一包 +1，只给 6.18 线加**
- `6.18` 是内核线标识；5.4 线永久冻结在 `0.1.0.3-r22-f50`

`scripts/bump-version.sh` 负责 +1 并同步 `VERSION`、`PACKAGING-VERSION`、`AGENT-VERSION`，
遇到非 6.18 线直接拒绝。

## 6. 想让 AI 帮你重写或调试

把仓库链接交给 AI，让它按顺序读：

1. [docs/6.18/接手说明.md](docs/6.18/接手说明.md) —— 现状、为什么必须自编、装机三步、判活五条、未修复问题
2. [docs/6.18/F50-6.18音频自编.md](docs/6.18/F50-6.18音频自编.md) —— 自造 kit 的完整命令与四个编译坑
3. `overlay/f50/` —— 要改文案就改 `strings.tsv`，要改页面就换 `files/`，然后 `sh overlay/f50/build.sh`
4. `scripts/bump-version.sh` 升号 → 重打 tar → `scp` 到 `/tmp` 再装

不要做的六件事：不刷 A 卡槽 Android、不动分区表、不改 modem NV、不覆盖厂商同名脚本、
不装别内核编的音频模块、不远程 `reboot`。

## 7. 5.4 线（已冻结）

2026-11-09 起封存。归档包在 [`archive/5.4-pkg/`](archive/5.4-pkg/)，文档在 [`docs/5.4/`](docs/5.4/)，
脚本在 [`scripts/5.4/`](scripts/5.4/)。**只留档，不再重建、不再出新版**。

## 8. 与 M8 的差异

两台同 SoC、两套参数，**包和文档不可互换**。逐项对照见 [docs/F50-vs-M8.md](docs/F50-vs-M8.md)。

## 9. 上游与致谢

本仓库是**独立适配工程**，不是 [kai-wu-cortex/airsim-mu300](https://github.com/kai-wu-cortex/airsim-mu300) 的官方发布。

| 项目 | 用途 | 作者 |
|---|---|---|
| [AirSIM](https://github.com/kai-wu-cortex/AirSIM) | 配对 / VoWLAN / Relay 协议、Agent 基线 | AirSIM contributors |
| [airsim-mu300](https://github.com/kai-wu-cortex/airsim-mu300) | daemon、Agent、LuCI、音频桥与打包流程，本适配的直接上游 | **kai-wu-cortex** |
| [mu300-linux](https://github.com/dikeckaan/mu300-linux) / [kanoqwq 分支](https://github.com/kanoqwq/mu300-linux) | 6.18 主线内核与设备基础设施 | **dikeckaan**、**kanoq** 及贡献者 |
| [mu300-audio](https://github.com/dikeckaan/mu300-audio) | 音频驱动、DSP 场景与工具 | **dikeckaan** 及贡献者 |
| [OpenWrt / LuCI](https://openwrt.org/) · [ALSA](https://www.alsa-project.org/) | 管理界面、包管理、音频工具 | 各上游贡献者 |

感谢 **kai-wu-cortex** 把 AirSIM 的 MU300 侧完整开源；感谢 **dikeckaan** 与 **kanoqwq** 在内核与通话音频栈上的工作。
上游引用不代表作者对本适配的背书。

## 10. 许可证

本适配沿用上游非商业定位，见 [LICENSE](LICENSE)、[NOTICE](NOTICE)。
AirSIM Agent 为 **PolyForm Noncommercial 1.0.0**；mu300-audio 按文件采用 **GPL-2.0 / MIT**；
ALSA 按组件采用 **LGPL-2.1-or-later / GPL-2.0-or-later**。各组件保留各自权利，
整包不能称为无商业限制的开源软件。
