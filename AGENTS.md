> **两条内核线严格分开**：6.18（在研主线，`docs/6.18/`、`scripts/6.18/`）与 5.4（已冻结，
> `docs/5.4/`、`scripts/5.4/`）。新改动只落 6.18 线，别再碰 5.4 的包和脚本。
>
> **先读 [`docs/6.18/接手说明.md`](docs/6.18/接手说明.md)**：里面写了这台机器现在到哪一步、
> 怎么装模块、怎么判活、哪些还是坑、下一步升上游 r23 怎么落。

# 给下一个 AI 的交接说明（F50 / MU300 / UMS9620）

先读完这一页，再动手。这里写的都是**这台机器上实测过**的东西，不是上游文档的复述。

## 0. 先分清两条线，绝不混用

| | **F50 线（本仓库）** | M8 线（另一仓库） |
|---|---|---|
| 仓库 | `shangjie1998/airsim-f50-openwrt` | `shangjie1998/airsim-m8-openwrt` |
| 设备 | 中兴 F50 / MU300 | VIPME-M8 |
| 语音内核 | **5.4.254-gb50db5b6224c** | 6.18.54-m8-probe12 |
| 采样 | **8 kHz 直通** | 播放 16000 / 采集 8000（带 Upsampler） |
| 声卡 | `card1 = sprdphone-sc2730` | `card0` |
| 节点 | 播放 **hw:1,53** / 采集 **hw:1,16** | hw:0,9 / hw:0,18 |
| 显示名 | 未改 | 改过（VIPME-M8） |
| 版本口径 | `0.1.0.x-f50`（上游 `0.1.0` + 打包序号，见 README §9） | `0.1.0(N)-r22-m8` |

两台之间**抄任何文件都会让语音挂掉**。要动 F50 的代码，克隆上游另开一份，不要在 M8 的树上改。

## 1. 这台机器现在是什么状态（2026-11-08）

- 系统：OpenWrt 25.12.5 `r33051-f5dae5ece4`，`aarch64_generic`，`/opt` 在 `/mnt/mu300-disk` 上的 Linux 分区。
- **当前运行内核：`6.18.60`（mainline）**。数据联网在 6.18 上比 5.4 更干净；**但语音是死的**——
  `luci-app-airsim` 附带的音频驱动只按 5.4 编过，`/etc/init.d/airsim` 起来就 `active with no instances`。
  要语音就切回 5.4：`mu300-update rollback-boot`（或把 `/mnt/mu300-disk/boot/kernel` 写回 `5.4` 再 `mu300-update boot`）。
- 插件：上游原版 `luci-app-airsim-0.1.0-r22`，未改包名、未改名。
- 设备侧运行时补丁（重启后仍然生效，见 §4）：`sipa-dele-start` 的就绪放行、`mu300-at` 的 stdin、
  `audio-setup` 的空列表保护、防火墙 `flow_offloading=0`。

## 1b. 源码与包对不对得上（有据可查，别猜）

| 物 | 哈希 / 判据 | 结论 |
|---|---|---|
| `dist/luci-app-airsim-0.1.0-r22.apk` | `849f72fb5cf3613dc8e029682182f62f9722441ab716da06a30329c038cbcfb8` | 与上游 `v0.1.0-r22` release 公布的 digest **逐位相同**，设备上装的就是这个官方包 |
| `src/`（r22 sources tarball） | `68495e7369e2e2400510d6010c77db6fbf72d1d12995e0bc4fe3af81547a7bdd` | 同一 release 公布的源码快照 |
| `src/plugin` vs 上游解包 | `diff -r` 无差异 | **源码树未被改动过**：F50 的适配全在设备侧，不在源码里 |
| 上游锚点 | `8cd12b4`（main HEAD，`VERSION`=`0.1.0-r22`） | 源码可用 `git clone` + `checkout 8cd12b4` 复现，等价于上面的 tarball |

**所以别指望在 `src/` 里找到 F50 补丁。** F50 那几处让它真能干活的关键改动，改的是设备上的运行库文件
（`/opt/mu300/bin/sipa-dele-start`、`/opt/mu300/bin/mu300-at`、`/usr/lib/airsim/` 下的音频暂存脚本），
仓库里以 `scripts/` 的重放脚本留档。重编一个包不会把这些改动带进去，装完必须单独打补丁。

## 1c. 品牌名改动范围（MU300 → F50）

照 M8 那次的规矩：**只换给人看的字段，标识一个都不动**。已经在源码里改过，自己重编包即生效。

改了（`src/plugin` 内）：mDNS 实例名 `AirSIM-F50-xxxx` 与 TXT 的 `device_name=AirSIM F50` / `model=F50`、
health 接口的 `device_name`/`model`、`/status` 的 `product=AirSIM for F50`、pushRegistration 的
`android_device_name`/`android_model`、音频报错串、LuCI 页面文案与 acl 描述、apk 的 `description`。

**没改（改了会出问题）**：`agent_kind=mu300`（客户端靠它判设备类型）、Go module 与二进制名
`airsim-mu300d`、`/usr/lib/airsim` 路径、`mu300-at`/`mu300-sms` 等工具名、`mu300-` 呼叫前缀、
升级检查 UA `AirSIM-MU300-update-check`、历史 release 说明里的旧串。

改完验证看这些：`dns-sd -B _airsim-vowlan._tcp` 实例名是 `AirSIM-F50-…`；`dns-sd -L` 的 TXT 里
`device_name=AirSIM F50` 且 `agent_kind=mu300`；`curl /status` 的 `product` 是 `AirSIM for F50`。

## 2. 设备访问

- 管理地址 `192.168.77.1`（OpenWrt 侧），`root`，密码不在仓库里，按现场提供。
- 热点 ESSID `ZTE-F50`，`wlan0` 与 `usb0` 桥进 `br-lan`。
- 常用只读检查见 §6。

## 3. 硬件与语音通道（F50 侧）

| 用途 | 节点 | 说明 |
|---|---|---|
| 播放（上行注入基带） | `hw:1,53` | `FE_ST_VOICE_PCM_P` |
| 采集（下行对端声音） | `hw:1,16` | `FE_ST_DUMP of DAC1` |
| 声卡 | `card1 = sprdphone-sc2730` | |
| DSP 内存预留 | `/sys/module/audio_reserve/parameters/missing` 应为 `0` | ranges `0xaf700000,0x300000;0xafa00000,0x600000` |
| 音频扩展状态 | `/usr/lib/airsim/audio-setup --status` → `ready` | |

**F50 没有喇叭、没有麦克风**——声音全靠 AirSIM 的 PCM 桥经网络走，不要去找本地 mixer 或
`/dev/snd` 的扬声器，那边什么都没有。Android 侧那条路已判定不可行（ROM 不注册
`android.app.role.DIALER`，绕过角色后信令通但下行 PCM 永远 `all_zero`）。

## 4. 四个必须知道的坑（脚本都在 `scripts/`）

| # | 现象 | 根因 | 修法 |
|---|---|---|---|
| 1 | 安装报 `MU300 runtime library missing` | 新 rootfs 裁掉了 `/opt/mu300/lib/run-dir.sh` | 从 `mu300-linux` 补回（644） |
| 2 | `audio-setup --install` 报 `tar: empty archive`，`status=failed` | `stage-device.sh` 对空文件列表跑 busybox `tar -cpf`，返回 1 触发 `set -e` | `scripts/5.4/音频暂存-空列表保护.sh` |
| 3 | 来电轮询永远失败 `MU300 AT request failed (AT+CLCC)`，手动同条 AT 正常 | 设备自带旧版 `mu300-at` 不认 `-`（stdin），把连字符当命令发出去 | `scripts/mu300-at-补stdin.sh`（原件留 `.bak-20261107`） |
| 4 | 数据不拨号：`sipa delegate did not become ready; refusing unsafe PDP activation` | **5.4 的 `sipa-dele.ko` 根本没有 `sipa_dele_wait` 节点**（6.18 才有），而 `delegate_ready()` 把"读不到"当硬失败 | 改 `/opt/mu300/bin/sipa-dele-start` 的 `delegate_ready()`：`/proc/modules` 里有 `sipa_dele` 就记日志放行。**6.18 上不需要这个补丁**（有 `sipa_dele_wait`/`sipa_dele_ready`，走原路径） |

> 判据：`sipa_dele_wait` 在不在，直接决定这个补丁要不要打。
> `ls /sys/bus/platform/drivers/sipa_delegate/soc:ipa-apb:sipa-dele/`

## 5. 客户端断网（本仓库最值钱的一条，`scripts/修断网.sh`）

**症状**：设备自身能上网，连热点的手机/电脑显示"无互联网"，DHCP 正常、DNS 正常、桥接正常。

**根因链**：`fw4` 渲染的 flowtable 里含 `sipa_eth0`（SIPA 委托出来的自定义 ARPHRD 虚拟口，`link/[519]`），
5.4 内核在整表事务里建这个 flowtable 会失败 → **整张防火墙规则表拒绝加载**，跑的还是开机 S19 那版
（那时 `br-lan` 与 `sipa_eth0` 都还没建）→ `chain accept_to_wan` 是空的，而 `forward` 的 policy 是 `drop`
→ 客户端包全被丢。设备自己的流量走 output 链，不经过 forward，所以"设备能上网、客户端不能"。

**最终方案**（已在设备上落地，重启后自洽）：

```sh
uci set firewall.@defaults[0].flow_offloading='0'
uci commit firewall
/etc/init.d/firewall restart
```

关掉软件流卸载后 fw4 根本不生成 flowtable，那个事务冲突从源头消失；顺带 LuCI 里“保存防火墙”也不会再翻车。
代价只是转发不走快路径，LTE 几十兆量级 CPU 看不出差别。

**上游的钩子方案**（`scripts/5.4/19-mu300-fw4-钩子版.sh`）实测可用但未采用：它在 ifup 时先删 flowtable 再 reload。
注意这台机器的 5.4 内核连 `nft delete flowtable inet fw4 ft` 都返回 `Resource busy`，必须先
`nft delete table inet fw4`（整表）才有用；而且它覆盖不到非 ifup 的 reload，所以选了“关卸载”。

**别做的事**：不要在 LuCI 防火墙页勾“软件流量分载”——一勾就把这个坑重新打开。

## 6. 快速自检

```sh
uname -r                                              # 5.4.254-gb50db5b6224c（要语音）或 6.18.60（只有数据）
cat /mnt/mu300-disk/boot/kernel                       # 5.4 / 6.18，当前选择
cat /proc/asound/cards                                # sprdphone-sc2730
/usr/lib/airsim/audio-setup --status                  # ready
/etc/init.d/airsim status                             # running
wget -qO- http://127.0.0.1:7581/status | head -c 400   # audio_available:true / running
printf 'AT+CLCC\n' | /usr/bin/mu300-at -t 6 -           # OK（验证 stdin 补丁在）
ubus call network.interface.wan status | head -20      # up=true, l3_device=sipa_eth0
uci get firewall.@defaults[0].flow_offloading          # 必须是 0
nft list chain inet fw4 accept_to_wan | tail -3        # 必须有 oifname "sipa_eth0" ... accept
cat /proc/net/nf_conntrack | grep '192.168.77.'        # 客户端 NAT 后源地址应是 sipa_eth0 的地址
ping -c2 223.5.5.5                                     # 设备自身出网
```

## 7. 要重编安装包

见 [docs/5.4/F50_BUILD_AND_ADAPT.md](docs/5.4/F50_BUILD_AND_ADAPT.md)。要点：

- 构建机需要 Git、**Go 1.24+**、Node.js、Docker（`node:22-alpine`）、curl、OpenSSL；
- 入口 `sh scripts/package-apk.sh`（内部会 `build.sh` → `fetch-deps.sh` → `fetch-sources.sh`）；
- Agent 固定在 AirSIM 提交 `cc2ade3124ac34bec7d8c441f4b66a57a6dfec62`（见 `src/plugin/patches/AIRSIM_BASE`）；
- **自己编的包公钥和 Release 里那个不同**，不能沿用发布版 `install.sh` 里写死的公钥指纹；
- 装的时候 `apk --no-network add --upgrade ./luci-app-airsim-0.1.0-r22.apk`（先放好 `airsim-dev.pem` 到 `/etc/apk/keys/`）。

## 8. 别踩的几件事

- **不要 `apk upgrade` 全量**。这台镜像的 `/usr/share/ucode/fw4.uc` 是被厂商改过的
  （`unisoc-flowtable-ports-v1`，会把只以 `l3_device` 存在的 `sipa_eth0` 纳进设备发现），
  升级 `firewall4` 会把它换回官方版，数据网立刻变差。
- **官方 kmods 源指向 6.12.94 内核**，与这台 5.4/6.18 都不匹配，`kmod-*` 一律不要从官方源装。
- `mu300-update apply` 若重铺 `/opt`，会覆盖 `sipa-dele-start` 与 `mu300-at` 的补丁，装完要重打。
- 重启只能手动物理/网页操作，不要在脚本里带 `reboot`。
- 换卡后若拨不起来，APN 手填（设备无内置 APN 库）：移动 `cmnet`、联通 `3gnet`/`uninet`、
  电信 `ctnet`（部分卡 `ctnet@mycdma.cn`）。

## 9. 文件地图

```text
README.md                    部署记录总览（结论、装机、坑、自检）
AGENTS.md                    本文件：给下一位接手者的最短路径
VERSIONING.md                版本编号规则（0.1.0.N-r22-f50，每打包一次 N+1）
docs/5.4/F50-通话打通.md          通话验收与调试全过程
docs/5.4/F50-数据联网修复.md       拨号修复（sipa）与客户端断网（flowtable）两条完整链路
docs/5.4/F50_BUILD_AND_ADAPT.md  从源码重建安装包、换内核/换硬件的适配口径
device-patches/              从设备取回的两处运行库补丁（含原件与 .diff，可直接重放）
scripts/                     设备侧补丁脚本（重放用）
src/plugin/                  上游 r22 插件源码（含 vendor、packaging、patches）
src/AirSIM/                  AirSIM 客户端/Agent 源码（固定提交）
src/dependencies/            重建音频扩展所需的依赖源码包
dist/                        已验收的安装包、公钥、安装脚本、SHA256SUMS
```

## 1d. 自编包怎么出（不给 docker 也行）

交叉编译只要 Go：`/Users/shangjie/Documents/doubaokaifagongju/go/go-sdk/go/bin`
（1.24.7 darwin/arm64，下载源见该目录 README）。项目自己的脚本留在本仓库，不放进工具目录。

```sh
export PATH=/Users/shangjie/Documents/doubaokaifagongju/go/go-sdk/go/bin:$PATH
cd src/plugin
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -mod=vendor -buildvcs=false -trimpath \
  -ldflags "-s -w -X main.version=0.1.0-r22" -o /tmp/f50-out/airsim-mu300d ./cmd/airsim-mu300d
AIRSIM_UPSTREAM_REPO=<本地 AirSIM 克隆> sh scripts/prepare-agent.sh     # 打印出 agent 源码目录
cd <上面打印的目录>/module/module-agent && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
  -trimpath -ldflags "-s -w -X main.agentVersion=0.4.7-mu300.<发布号>" -o /tmp/f50-out/airsim-agent .
```

`prepare-agent.sh` 默认去 `src/plugin/../AirSIM` 找上游，那份是 tarball 拷贝（没有 `.git`），
必须用 `AIRSIM_UPSTREAM_REPO` 指到真正的 AirSIM 克隆，否则报
`Not a valid object name cc2ade3…^{commit}`。

出包：`sh scripts/bump-version.sh` 拿新版本 → 放二进制与页面进 `pkg/airsim-f50-<版本>/`
→ `tar czf dist/airsim-f50-<版本>.tar.gz -C pkg/airsim-f50-<版本> .`。
apk v3 签名包仍需 apk-tools 3.x（本机与云主机都没有 Docker），暂不做。

## 1e. 装机验证记录（2026-11-08，5.4 内核）

用 `dist/airsim-f50-0.1.0.2-r22-f50.tar.gz` 覆盖安装、`mu300-update rollback-boot` 回 5.4 后实测：

| 项 | 命令 | 实测 |
|---|---|---|
| 内核 | `uname -r` | `5.4.254-gb50db5b6224c` |
| 服务 | `/etc/init.d/airsim status` | `running`（3 个进程：daemon ×2 + agent） |
| 音频 | `ubus call airsim status` | `audio_available: true` |
| 显示名 | `ubus call airsim status` | `"product": "AirSIM for F50"` |
| 包版本 | `head -1 /usr/lib/airsim/MANIFEST` | `0.1.0.2-r22-f50` |
| mDNS | Mac 上 `dns-sd -B _airsim-vowlan._tcp` | 实例 `AirSIM-F50-0p0JKELA`，TXT `device_name=AirSIM\ F50` / `model=F50` / `agent_kind=mu300` |

设备没插卡（没有余额），所以只验到"插件起得来、音频可调用"这一层；通话本身没复验。
6.18.x 上同样的包会停在 `active with no instances`（prepare 的音频预检过不去），这不是包的问题，
要语音就留在 5.4。

## 1f. 断网时切内核（离线喂包）

设备没插卡、没网，`mu300-update` 里凡是要 `latest_release` 或 `fetch_verified` 的都走不通。能救场的只有两条：

- **回滚**：`mu300-update rollback-boot`（把 `prev.body` 写回，一次性；不联网）
- **喂本地包**：`MU300_RELEASE=<tag> MU300_KERNEL_BUNDLE=<本地 tar.gz> mu300-update kernel <5.4|6.18>`

本地包从 `dikeckaan/mu300-linux` 的 Releases 取，tag 跟设备已装系统一致就行（2026-11-08 用的
`v2026.11.02.1`，其中 `mu300-kernel-6.18.tar.gz` 53,175,572 字节，sha
`64893b18…e130`）。设备上核完 sha 再跑；跑完会打印 `slot b armed`，新镜像落在不常用的 b 槽，
起不来会自动退回 Android。包里的 `./devices` 必须含 `f50`。

注意 `mu300-update kernel` 每次都会重新写 `prev.*`，所以喂包这条路可以反复走，不受 rollback 一次性限制。

## 1g. 装机验证记录（2026-11-08 晚，6.18 内核）

同一个包在 6.18 上的实测结果，跟 5.4 差别只在音频：

| 项 | 实测 |
|---|---|
| 内核 | `6.18.60`（模块 2958 个，`mu300-update kernel 6.18` 装好后 `installed.kernel` 与 `kernel` 都是 `6.18`） |
| 服务 | `/etc/init.d/airsim status` = `active with no instances`（prepare 的音频预检过不去，procd 不建实例） |
| 手动起 | `/usr/lib/airsim/airsim-mu300d --interface br-lan` 可正常跑，日志首行 `F50 plugin ready; pairing port 8580` |
| `audio_available` | `false` |
| 显示名 | `"product": "AirSIM for F50"`、mDNS 实例 `AirSIM-F50-0p0JKELA` |

结论：**6.18 上跑得起来、显示名正常，只是通话不可用**。要语音就 `rollback-boot` 回 5.4
（喂包那条路会重写 `prev.*`，所以随时能回）。想在 6.18 上自启，得等 6.18 版的音频 ko。
