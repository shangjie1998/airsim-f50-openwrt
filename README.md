# AirSIM on 中兴 F50（MU300 / UMS9620）— OpenWrt 部署记录

把 AirSIM 通话网关跑在中兴 F50 上：**5.4 内核下双向通话已实机打通**，数据联网也已修复。
本仓库是这个过程的实测记录与修复脚本，不是刷机包，也不是 AirSIM 官方发布。

| | |
|---|---|
| 设备 | 中兴 F50（`MU300`，`Spreadtrum UMS9620`），OpenWrt 侧地址 `192.168.77.1` |
| 系统 | OpenWrt **25.12.5 `r33051-f5dae5ece4`**，`aarch64_generic` |
| 语音内核 | **`5.4.254-gb50db5b6224c`**（厂商内核，`mu300-audio` 的驱动按它编） |
| 数据内核 | `6.18.60`（mainline，数据联网更干净，**语音失效**） |
| 插件 | 上游原版 `luci-app-airsim-0.1.0-r22`（未改包名、未改名） |
| 上游 | [kai-wu-cortex/airsim-mu300](https://github.com/kai-wu-cortex/airsim-mu300) |

> **客户端断网了？** 先跑 `sh scripts/修断网.sh --check`，症状对得上就 `--fix`。一条根因：
> flowtable 里含 `sipa_eth0` 导致整张防火墙规则表加载失败（详见 §4 第 5 条与
> [docs/F50-数据联网修复.md](docs/F50-数据联网修复.md)）。

同 SoC 的另一条线（VIPME-M8，内核 6.18.54，改过播放采样率与显示名）见
<https://github.com/shangjie1998/airsim-m8-openwrt>。两台机器内核、音频路由、播放参数都不同，
**安装包与文档不能互换**。

---

## 1. 结论先说

- **通话**：5.4 内核下打通，双向有声。一路约 40 s 通话实测 `downlink_frames=2062`、
  `received_uplink_frames=2060`、`uplink_frames=1990`、`last_error` 空，事件链
  `dial_request → pcm_connection → pcm_authenticated → pcm_ready → pcm_closed` 完整。
- **数据联网**：`sipa_eth0` 拿到地址、默认路由、DNS，设备与 LAN 客户端都能出网，
  整机重启后自动拨号成立（已两次冷启动验证）。
- **6.18 内核**：数据比 5.4 还干净，但 `luci-app-airsim` 附带的音频驱动只编了 5.4，
  `/etc/init.d/airsim` 起不来（`active with no instances`），语音没有。要语音就待在 5.4。

为什么不在 Android 上做：F50 的 ROM 没有 `android.app.role.DIALER`，绕过角色后信令全通但**下行音频永远是 0**
（`pcm_capture=all_zero`、`/dev/snd` 只有 `timer`、`tinymix` 打不开 mixer）。转 OpenWrt 后由
`mu300-audio` 重建内核模块 + `agdsp.bin` 把 modem 语音 PCM 经 ALSA 暴露出来，这条路才成立。

## 2. 语音面：设备节点在哪

| 用途 | 节点 | 说明 |
|---|---|---|
| 播放（上行注入基带） | `hw:1,53` | `FE_ST_VOICE_PCM_P` |
| 采集（下行对端声音） | `hw:1,16` | `FE_ST_DUMP of DAC1` |
| 声卡 | `card1 = sprdphone-sc2730` | 与 M8 同名不同卡号 |
| DSP 内存保留 | `/sys/module/audio_reserve/parameters/missing` = `0` | ranges `0xaf700000,0x300000;0xafa00000,0x600000` |
| 音频扩展状态 | `/usr/lib/airsim/audio-setup --status` → `ready` | |

**F50 是 8 kHz 直通**，不涉及 M8 那条"16 kHz 播放 + Upsampler"的修正——这也是两台机器最容易搞混的地方。

## 3. 装机流程（可重放）

1. 切 5.4 内核 → 重启进 OpenWrt，`uname -r` 应为 `5.4.254-gb50db5b6224c`。
2. 校验内核配置哈希（音频扩展的硬门槛）：
   ```sh
   gzip -dc /proc/config.gz | grep -E '^(CONFIG_|# CONFIG_)' | LC_ALL=C sort | sha256sum
   # 必须等于 1057cb97e0d2d7f26816e8bf7841faac1153833a01d4893cdc196b75ae6356b3
   ```
3. 落地签名密钥 `/etc/apk/keys/airsim-dev.pem`（公钥 sha256 `0ce703e7…cf1de`）。
4. 离线安装 `luci-app-airsim-0.1.0-r22.apk`（`apk --no-network add --upgrade`）。
5. **先做 §4 的两个脚本修复**，再 `audio-setup --install` → 重启 → `audio-setup --resume`
   → `uci set airsim.main.enabled=1` → `/etc/init.d/airsim start`。
6. LuCI → 服务 → AirSIM 取六位配对码，iPhone 连 `ZTE-F50` 热点后配对。

开机顺序已布好：`S10airsim-audio-reserve`（先占 DSP 内存）→ `S97airsim-audio-setup`（续跑校验，通过后拉 AirSIM）。

## 4. 踩到的坑（脚本都在 `scripts/`）

| # | 现象 | 根因 | 修法 |
|---|---|---|---|
| 1 | `MU300 runtime library missing`，安装直接失败 | 新 rootfs 裁掉了 `/opt/mu300/lib/run-dir.sh`，而 `audio-preflight` 硬性要求它 | 从上游 `mu300-linux` 补回该文件（644） |
| 2 | `audio-setup --install` 报 `tar: empty archive`，`status=failed` | `stage-device.sh` 在"旧文件一个都没有"时对空列表跑 busybox `tar -cpf`，返回 1 触发 `set -e` | `scripts/音频暂存-空列表保护.sh`：空列表直接 `: > previous.tar`，`restore()` 同样加 `-s` 判断 |
| 3 | Agent 来电轮询一直失败，`last_poll_error: MU300 AT request failed (AT+CLCC)`，但手动 `mu300-at 'AT+CLCC'` 是 OK | 设备自带旧版 `mu300-at`（10-27）不认 `-`（stdin），把连字符当 AT 命令发出去 | `scripts/mu300-at-补stdin.sh` 补上 stdin 支持（原件备份 `.bak-20261107`） |
| 4 | 数据不拨号，日志 `sipa delegate did not become ready; refusing unsafe PDP activation` | 5.4 的 `sipa-dele.ko` **没有** `sipa_dele_wait` 节点，`delegate_ready()` 把"读不到就绪接口"当硬失败；其实模块是注册后才 insmod 的，委托线程是活的 | 改 `/opt/mu300/bin/sipa-dele-start` 的 `delegate_ready()`：模块已加载则记日志继续，未加载仍拒绝 |
| 5 | 设备能上网、连热点的终端显示"无互联网" | `flowtable` 含 `sipa_eth0`（自定义 ARPHRD 虚拟口），fw4 整张规则表拒绝加载，留下的是 `forward policy drop` 的旧表 | **最终方案**：`uci set firewall.@defaults[0].flow_offloading='0'`（不生成 flowtable，5.4 的 EBUSY 冲突从源头消失），一键脚本 `scripts/修断网.sh`；上游 hotplug 钩子方案实测可用但未采用，留档 `scripts/19-mu300-fw4-钩子版.sh` |
| 6 | 切 6.18.60 后 `airsim` 起不来 | 音频附加包的 ko 只编了 5.4 | 回 `mu300-update rollback-boot`，或等带 6.18 驱动的新版 audio 包 |

细节、实测输出与回退步骤见 [docs/F50-通话打通.md](docs/F50-通话打通.md) 与
[docs/F50-数据联网修复.md](docs/F50-数据联网修复.md)。

## 5. 已知限制

- **5.4 内核上 5G 数据要靠本次修复才通**；`mu300-update apply` 若重铺 `/opt`，`sipa-dele-start`
  和 `mu300-at` 的改动会被覆盖，需要重做。
- **不要在 LuCI 防火墙页勾"软件流量分载"**：一勾就把 5.4 的 flowtable 坑重新打开，症状是设备正常、客户端断网。
- 换卡后若拨不起来，APN 需手填（设备无内置 APN 库）：移动 `cmnet`、联通 `3gnet`/`uninet`、
  电信 `ctnet`（部分卡 `ctnet@mycdma.cn`）。
- `mu300-voice` 空闲时 dsp 显示 `no reply` 属正常——DSP 只在通话时由 `prepare.local` 拉起。
- 信号强度读不出来（`AT+CSQ: 99,99`），个别 ping 300 ms+，与本次改动无关。
- 上游验收范围是 U30 Air，F50 没有官方回归；**本仓库的记录才是 F50 的实测依据**。

## 6. 快速自检

```sh
uname -r                                             # 5.4.254-gb50db5b6224c
cat /proc/asound/cards                               # sprdphone-sc2730
cat /sys/module/audio_reserve/parameters/missing      # 0
/usr/lib/airsim/audio-setup --status                  # ready
wget -qO- http://127.0.0.1:7581/status | head -c 400   # audio_available:true
printf 'AT+CLCC\n' | /usr/bin/mu300-at -t 6 -          # OK（验证 stdin 补丁在）
/etc/init.d/airsim status                             # running
ubus call network.interface.wan status | head -20      # up=true, l3_device=sipa_eth0
```

## 7. 上游与致谢

本仓库是**个人部署记录**，不是 [kai-wu-cortex/airsim-mu300](https://github.com/kai-wu-cortex/airsim-mu300)
或 mu300-linux 的官方发布，也不是 GitHub Fork（同一账号对同一上游只能 fork 一次，那个位置给了 M8 仓库）。

| 项目 | 用途 | 作者 |
|---|---|---|
| [airsim-mu300](https://github.com/kai-wu-cortex/airsim-mu300) | LuCI 插件、daemon、Agent、音频桥、打包与安装流程 | **kai-wu-cortex** |
| [AirSIM](https://github.com/kai-wu-cortex/AirSIM) | 配对 / VoWLAN / Relay 协议与客户端 | AirSIM contributors |
| [mu300-linux](https://github.com/dikeckaan/mu300-linux) | OpenWrt 设备侧基础设施、`run-dir.sh` 等运行时库 | **dikeckaan** 及贡献者 |
| [mu300-audio](https://github.com/dikeckaan/mu300-audio) | 音频驱动、DSP 场景、`audio-setup` | **dikeckaan** 及贡献者 |
| [OpenWrt / LuCI](https://openwrt.org/) · [ALSA](https://www.alsa-project.org/) | 系统、管理界面、音频工具 | 各上游贡献者 |

感谢 **kai-wu-cortex** 把插件与安装流程开源——本记录里的每一步都建立在它之上；
感谢 **dikeckaan** 在 MU300 Linux 与通话音频栈上的工作。上游引用不代表作者对本记录的背书。

## 8. 许可证

本仓库只包含文字记录与几个设备侧补丁脚本，沿用上游的非商业定位：
AirSIM Agent 为 **PolyForm Noncommercial 1.0.0**，mu300-audio 按文件采用 **GPL-2.0 / MIT**。
脚本可自由取用，但不得用于商业目的；涉及上游组件的部分以各自许可证为准。

## 9. 仓库结构与「从源码重来」

这个仓库现在**不只是记录**：源码、依赖和已验收的安装包都在里面，下一个接手的人和 AI 可以从零重建。

```text
src/plugin/              上游 r22 插件源码（go.mod、internal/、packaging/、patches/、vendor/）
src/AirSIM/              AirSIM 客户端与 module-agent 源码（固定提交 cc2ade3）
src/dependencies/        重建音频扩展用的依赖源码包（mu300-audio、内核构建树、alsa、openwrt-packages）
src/UPSTREAM-SOURCES.md  上游源码包出处与哈希
dist/                    luci-app-airsim-0.1.0-r22.apk + 公钥 + install.sh + SHA256SUMS
docs/F50_BUILD_AND_ADAPT.md   构建环境、构建步骤、换内核/换参数口径、验证清单
AGENTS.md                给下一个 AI 的最短路径（参数、坑、别做什么）
scripts/                 设备侧补丁脚本（重放用）
```

已验收的包：

| 文件 | 大小 | sha256 |
|---|---|---|
| `dist/luci-app-airsim-0.1.0-r22.apk` | 9229711 | `849f72fb…cbcfb8` |
| `dist/airsim-dev.pem`（签名公钥） | 625 | `0ce703e7…2cf1de` |

上面这个 apk 的 sha256 与上游 `v0.1.0-r22` release 公布的 digest **逐位相同**；`src/` 是同一 release
公布的源码快照，与上游解包 `diff -r` 无差异。也就是说：**包和源码是同一版本、可对账的，但源码里没有
F50 的补丁**——F50 的适配全在设备侧（`sipa-dele-start`、`mu300-at`、音频暂存脚本），以 `scripts/` 留档重放。

一次性拿全的整包：`dist/airsim-f50-0.1.0-r22-bundle.tar.gz`（apk + 公钥 + 安装脚本 + `修断网.sh` +
`SHA256SUMS` + 安装说明），Release 附件里也有。

重建最短路径（细节见 `docs/F50_BUILD_AND_ADAPT.md`）：

```sh
cd src/plugin
AGENT_TREE=$(sh scripts/prepare-agent.sh)   # 需要 Go 1.24+、Docker（node:22-alpine）
sh scripts/package-apk.sh                   # 产物在 out/，签名私钥留在 out/signing/
```

**自编的包公钥与 `dist/` 里那把不同**，装的时候必须换成自己的公钥，不能沿用 `dist/install.sh`
里写死的指纹。

管线里的三道硬门槛，改任何一个参数都得同步改这三处：内核版本 `5.4.254-gb50db5b6224c`、
内核配置哈希 `1057cb97…6356b3`、以及音频节点的 `hw:1,53` / `hw:1,16`。
