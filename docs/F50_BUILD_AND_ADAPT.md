# F50 从源码重建与适配口径

本文件回答三个问题：**怎么把这台机器上跑的那个包装回去**、**换内核/换参数要改哪里**、
**怎么确认改完没坏**。

---

## 1. 这份源码是哪来的

`src/` 是上游 release 的源码快照，原样保存，未改动：

| 来源 | `https://github.com/kai-wu-cortex/airsim-mu300/releases/download/v0.1.0-r22/airsim-mu300-0.1.0-r22-sources.tar.gz` |
|---|---|
| 大小 | 31494845 字节 |
| sha256 | `68495e7369e2e2400510d6010c77db6fbf72d1d12995e0bc4fe3af81547a7bdd` |
| 版本 | `src/plugin/VERSION` = `0.1.0-r22` |
| AirSIM 固定提交 | `cc2ade3124ac34bec7d8c441f4b66a57a6dfec62`（见 `src/plugin/patches/AIRSIM_BASE`） |

tarball 解开是三块，仓库里对应成三个目录：

| 包内路径 | 仓库路径 | 内容 |
|---|---|---|
| `plugin/` | `src/plugin/` | LuCI 插件、`airsim-mu300d`、PCM/配对/网关代码、`packaging/`、`patches/`、`vendor/` |
| `AirSIM/` | `src/AirSIM/` | 客户端与 `module-agent` 源码（构建时打 `patches/AirSIM-mu300.patch`） |
| `dependencies/` | `src/dependencies/` | 重建音频扩展要用的依赖源码（mu300-audio、mu300-linux 内核构建、alsa、openwrt-packages） |

要换上游版本：整块替换 `src/plugin`、`src/AirSIM`、`src/dependencies`，并同步更新上表的 sha256。

---

## 2. 构建环境

| 依赖 | 要求 | 说明 |
|---|---|---|
| Git | 任意 | `prepare-agent.sh` 要拉 AirSIM 固定提交 |
| Go | **1.24+** | 两个二进制都是 `CGO_ENABLED=0 GOOS=linux GOARCH=arm64` 交叉编译 |
| Node.js | 22 可用即可 | 跑 `scripts/*.test.mjs` |
| Docker | 需要能跑 `node:22-alpine` | `package-apk.sh` 在容器里打包 |
| curl / OpenSSL | — | 拉依赖、签名 |

在 macOS 上能直接构建（交叉编译不依赖 OpenWrt SDK，因为产物是静态 Go 二进制 + 现成 apk 树）。

## 3. 构建步骤

```sh
cd src/plugin

# 1) 单测（可选但强烈建议，先证明代码本身是好的）
go test -race ./...
go vet ./...
node --test scripts/overview.test.mjs scripts/sms.test.mjs \
            scripts/install.test.mjs scripts/manage.test.mjs

# 2) 准备 Agent 工作树（把补丁打到 AirSIM 的隔离副本，不动上游）
AGENT_TREE=$(sh scripts/prepare-agent.sh)
(cd "$AGENT_TREE/module/module-agent" && go test -race ./... && go vet ./...)

# 3) 拉构建依赖（对应 src/dependencies 里的那几个源码包）
docker pull node:22-alpine

# 4) 出包
sh scripts/package-apk.sh
# 产物在 out/，签名私钥默认在 out/signing/（不上传、不进仓库）
```

`package-apk.sh` 内部顺序是 `build.sh` → `fetch-deps.sh` → `fetch-sources.sh` → 组装 `packaging/openwrt/root` →
按 `AIRSIM_COMPLETE_INSTALL=1` 把**已审计的音频扩展包**并进同一个 apk。
`AIRSIM_PACKAGE_KEY` 可指定签名私钥；不指定就用本机 `out/signing/` 里那把。

> **公钥不是同一把。** 自己编出来的包，`/etc/apk/keys/` 里要放**你自己**的公钥，
> `dist/install.sh` 里写死的公钥指纹 `0ce703e7…cf1de` 只对应 Release 里那个包。混用会报签名校验失败。

## 4. 装到设备

```sh
# 1) 放签名公钥
scp airsim-dev.pem root@192.168.77.1:/etc/apk/keys/

# 2) 离线安装（不要加 --allow-untrusted）
apk --no-network add --upgrade /tmp/luci-app-airsim-0.1.0-r22.apk
```

安装包自带两道门槛（`install.sh` 里也能看到，绕过它们等于赌）：

1. **内核版本**必须是 `5.4.254-gb50db5b6224c`（音频扩展只有这版驱动）；
2. **内核配置哈希**：
   ```sh
   gzip -dc /proc/config.gz | grep -E '^(CONFIG_|# CONFIG_)' | LC_ALL=C sort | sha256sum
   # 必须等于 1057cb97e0d2d7f26816e8bf7841faac1153833a01d4893cdc196b75ae6356b3
   ```

两者任一不符，说明音频扩展的 ko 与当前内核对不上，装了也起不来——这正是 6.18 上语音失效的原因。

装完还要按 `scripts/` 里的两个脚本做运行时修复（空列表保护、`mu300-at` stdin），
再 `audio-setup --install` → 重启 → `audio-setup --resume` → `/etc/init.d/airsim start`。

## 5. 换内核会让什么失效

| 能力 | 依赖 | 5.4 厂商内核 | 6.18 mainline |
|---|---|---|---|
| 数据联网 | `sipa_*`、netifd `mu300cell.sh` | 需要 `sipa-dele-start` 放行补丁 | **原生可用**（有 `sipa_dele_wait`/`sipa_dele_ready`） |
| 语音 | `mu300-audio` 编出的 ko + `agdsp.bin` | 可用 | **不可用**（ko 只按 5.4 编） |
| 防火墙 | fw4 + flowtable | 有 5.4 事务坑，见 `docs/F50-数据联网修复.md` | 无此问题 |

要在 6.18 上恢复语音，只有一条路：用 `src/plugin/packaging/openwrt-audio/` 那套打包逻辑，
针对 6.18.60 的内核头重新编一份音频模块（依赖 `src/dependencies/mu300-audio-v1.0.5-source.tar.gz`
与 `mu300-linux-kernel-build-source.tar.gz`），再放进新的音频扩展包。
编之前先确认设备上 `/lib/modules/6.18.60/` 与构建用的内核树一致，否则 vermagic 对不上、insmod 会拒。

## 6. 适配别的硬件/参数要改哪里

F50 这套参数落在以下几处，换机器时逐项核对（**照 M8 的参数抄必挂**）：

| 参数 | F50 值 | 出现在 |
|---|---|---|
| 采样 | 8 kHz 直通，不做重采样 | `src/plugin/internal/audio/`（重采样路径在 M8 才启用） |
| 播放节点 | `hw:1,53` | `src/plugin/HARDWARE_ADAPTATION.md`、设备侧 `/usr/lib/airsim/` 配置 |
| 采集节点 | `hw:1,16` | 同上 |
| 声卡 | `card1` = `sprdphone-sc2730` | 同上 |
| 内核 | `5.4.254-gb50db5b6224c` | `packaging/openwrt-audio/` 的文件名与安装门槛 |
| 配置哈希 | `1057cb97…6356b3` | `install.sh`、`audio-preflight` |

改完参数后**必须重新跑 §7 的验证**，别只看 `airsim` 起来了就算完——曾经出现过"服务 running、
但下行采集全是 0 字节"的情况，要靠帧计数确认。

## 7. 验证（改完必做）

```sh
# 服务与音频扩展
/etc/init.d/airsim status                  # running
/usr/lib/airsim/audio-setup --status       # ready
wget -qO- http://127.0.0.1:7581/status | head -c 400

# AT 通道（验证 stdin 补丁）
printf 'AT+CLCC\n' | /usr/bin/mu300-at -t 6 -

# 打一通电话，然后看帧计数是否真的在涨
wget -qO- http://127.0.0.1:8575/api/calls/status
#   downlink_frames / received_uplink_frames / uplink_frames 都在涨、last_error 为空 = 双向成立
#   只有服务 running 但帧计数不动 = 音频链路没接上，别放过

# 数据
ubus call network.interface.wan status | head -20     # up=true, l3_device=sipa_eth0
uci get firewall.@defaults[0].flow_offloading         # 0
nft list chain inet fw4 accept_to_wan | tail -3       # 有 oifname "sipa_eth0" accept
```

电话实测时关注 `docs/F50-通话打通.md` 里那份数据表的口径（约 40 s 通话：
`downlink_frames≈2062`、`received_uplink_frames≈2060`），量级差太多就是链路有问题。

## 8. 发布与留档

版本号按 `0.1.0.N-r22-f50` 走（`VERSIONING.md`）：主版本与 `r22` 跟上游，`N` 每打包一次 +1。

- 已验收的包：`dist/luci-app-airsim-0.1.0-r22.apk`
  （sha256 `849f72fb5cf3613dc8e029682182f62f9722441ab716da06a30329c038cbcfb8`）。
- `dist/SHA256SUMS` 同时记了 apk、公钥、安装脚本、上游源码包的哈希。
- 重新发布会改变包内容与签名，**必须同时更新** `SHA256SUMS`、本文件的哈希、以及 `README` 里的口径，
  否则下一位接手的人会拿旧指纹去验新包。
