> **主包已不绑内核**（`0.1.0.26-r23-any` 起）：`5.4.254` / `6.18.60` / `7.3.1` 一条包都装得，
> 只剩 `aarch64` 一道闸。内核支持与水音来源的**唯一口径**在 [`docs/内核矩阵.md`](docs/内核矩阵.md)，
> 别在别处复述。
>
> **仓库按职责分目录**：`build/`（造包，含包内 `pkg/install.sh`）、`parts/{audio,bluetooth}`（配件）、
> `ops/`（设备侧一次性脚本）、`overlay+f50/src`（**CI 触发路径，不许改名**）、`docs/`（按主题）、
> `archive/`（只留档）。新文件按职责放，别往根目录扔。
>
> **先读 [`docs/接手说明.md`](docs/接手说明.md)**：里面写了这台机器现在到哪一步、
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

## 1. 这台机器现在是什么状态（**2026-11-14 为准**）

> 这一节下面的旧描述（"6.18 语音是死的、要语音必须切 5.4"）**已经作废**。
> 现在 6.18.60 上号码、短信、通话双向音频三项全部真机打通。

- 系统：OpenWrt 25.12.5，`aarch64_generic`，`/opt` 在 `/mnt/mu300-disk` 上的 Linux 分区。
- **当前运行内核：`6.18.60`（mainline）**，数据联网与语音都在这上面跑通了。
- 版本：**`0.1.0.30-r23-any`**（Release `v0.1.0.30-r23-any`）。
- 语音走**自编音频配件包** `audio-opt 1.0.4`（`parts/audio/`，只有 `.ko`，按内核分目录）；
  主包负责二进制 + `overlay/f50/files` 整棵树。
- **设备上不再有"手打的运行时补丁"**。以前靠 `ops/*.sh` 手打的那几处，这轮全固化成了
  `overlay/f50/files/` 下的兼容层，随包安装、刷机后自动生效（见 §4）。
- 现在只剩人为 UCI 设置还留在设备上：防火墙 `flow_offloading=0`（见 §5）。

### 1a. 2026-11-14 固化的三个兼容层（**别再手改机器**）

| 文件（在 `overlay/f50/files/` 下） | 顶上哪个厂商契约 |
|---|---|
| `usr/bin/mu300-at` | 新版厂商脚本不认末尾的 `-`（命令走 stdin），只认 argv。上层 `mu300-at -t N -` 被当成 AT 原样发出，回 `+CME ERROR: 0` 但退出码 0，**静默失败**。兼容层探测真脚本里有没有 `if [ "$cmd" = - ]`，有就转交、没有就自己读 stdin 按 argv 传 |
| `opt/mu300/bin/mu300-sms` | 同一个坑（`grep -c stdin` = 0）。`send --stdin` 报 "send needs a phone number and a text"。REAL 指向 `.fw-real`，缺真脚本直接报错，**绝不回退原路径**（会无限递归） |
| `opt/mu300/lib/run-dir.sh` | 音频配件包的 `mu300-voice` 要 `.` 它并调 `run_dir_take`，但固件比配件包旧、这库不存在 → `mu300-voice` 启动即死 → PCM 后端永远 not ready → 手机连上也没声 |

**铁律（这轮踩了两次）**：

1. **凡是手工在机器上改的东西，刷机就白干**。要修就往 `overlay/f50/files/` 里写，进包。
2. **`overlay/f50/files/` 下文件权限就是设备最终权限**：新 shell 脚本先 `chmod 755` 再 `git add`，
   否则 git 记 `100644`、tar 照搬 644，root 也 exec 不了（本轮 `mu300-sms`/`run-dir.sh` 就栽了）。
3. **写任何 shell 前先 `command -v` 验命令**。这台设备**没有 `stat`**（不报错、只静默返回空值，
   会让 `if` 判反），也没有 `timeout`/`nohup`/`od`/`base64`。

## 1b. 源码与包对不对得上（有据可查，别猜）

| 物 | 哈希 / 判据 | 结论 |
|---|---|---|
| `dist/luci-app-airsim-0.1.0-r22.apk` | `849f72fb5cf3613dc8e029682182f62f9722441ab716da06a30329c038cbcfb8` | 与上游 `v0.1.0-r22` release 公布的 digest **逐位相同**，设备上装的就是这个官方包 |
| `src/`（r22 sources tarball） | `68495e7369e2e2400510d6010c77db6fbf72d1d12995e0bc4fe3af81547a7bdd` | 同一 release 公布的源码快照 |
| `src/plugin` vs 上游解包 | `diff -r` 无差异 | **源码树保持上游原样**：F50 的适配全在 `overlay/f50/`（files + patches），一个字都没写进 `src/` |
| 上游锚点 | `8cd12b4`（main HEAD，`VERSION`=`0.1.0-r22`） | 源码可用 `git clone` + `checkout 8cd12b4` 复现，等价于上面的 tarball |

**所以别指望在 `src/` 里找到 F50 补丁。**（这一节是 11-08 写的，那时确实是"全在设备侧"；
架构在 11-12 之后变了，**现在 F50 的适配分两层，都不在 `src/` 里**）：

| 层 | 位置 | 管什么 |
|---|---|---|
| **文件层** | `overlay/f50/files/` 整棵树 | 直接铺到设备根目录的**厂商脚本兼容层**（§1a 那三个）、`/usr/lib/airsim/` 那些运行时脚本。刷机后随包装上 |
| **补丁层** | `overlay/f50/patches/*.patch` | 上游 Go 代码的改动，构建时由 `apply.py` 打进合并树。规则见 §8c，目前 9 个 active |

旧的 `ops/*.sh` 只当**历史留档**，不要再拿它手打设备——那些内容已经进了上面两层。

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

## 4. 四个老坑（**1、3 已固化进包，不用再手打**）

| # | 现象 | 根因 | 现在怎么办 |
|---|---|---|---|
| 1 | 安装报 `MU300 runtime library missing` | 新 rootfs 裁掉了 `/opt/mu300/lib/run-dir.sh` | **已固化**：`overlay/f50/files/opt/mu300/lib/run-dir.sh` 由主包铺上去（§1a）。旧脚本 `ops/` 只留档 |
| 2 | `audio-setup --install` 报 `tar: empty archive`，`status=failed` | `stage-device.sh` 对空文件列表跑 busybox `tar -cpf`，返回 1 触发 `set -e` | `ops/legacy/音频暂存-空列表保护.sh`（这条仍要单独处理） |
| 3 | 来电轮询永远失败 `MU300 AT request failed (AT+CLCC)`，手动同条 AT 正常 | 设备自带 `mu300-at` 不认 `-`（stdin），把连字符当命令发出去 | **已固化**：`overlay/f50/files/usr/bin/mu300-at` 探测式兼容层（§1a）。上层那段 `exec.Command(...,"-")` 保持不动，由兼容层适配 |
| 4 | 数据不拨号：`sipa delegate did not become ready; refusing unsafe PDP activation` | **5.4 的 `sipa-dele.ko` 根本没有 `sipa_dele_wait` 节点**（6.18 才有），而 `delegate_ready()` 把"读不到"当硬失败 | 改 `/opt/mu300/bin/sipa-dele-start` 的 `delegate_ready()`：`/proc/modules` 里有 `sipa_dele` 就记日志放行。**6.18 上不需要这个补丁**（有 `sipa_dele_wait`/`sipa_dele_ready`，走原路径） |

> 判据：`sipa_dele_wait` 在不在，直接决定这个补丁要不要打。
> `ls /sys/bus/platform/drivers/sipa_delegate/soc:ipa-apb:sipa-dele/`

**另外两个这轮新增的，比上面更阴**（详见 [docs/经验库/mu300-call-audio-dsp.md](docs/经验库/mu300-call-audio-dsp.md)）：

- **`mu300-sms --stdin` 同样的 stdin 坑**，报 "send needs a phone number and a text"。
- **`stat` 不存在且静默返回空值**：`[ "$(stat -c %u dir)" != 0 ]` 会恒判"被占用"→`rm -rf`→
  把 `mu300-voice` 的 `scene.pid` 删了→场景永远判"没在跑"→`pcmhost` 越积越多互相抢 ALSA 设备。
  取属主改用 `ls -ldn | awk '{print $3}'`，读不出就原样沿用不删。

## 5. 客户端断网（**5.4 时代的问题**，脚本已归档 `archive/5.4-pkg/修断网.sh`，6.18 线不再需要）

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

**上游的钩子方案**（`ops/legacy/19-mu300-fw4-钩子版.sh`）实测可用但未采用：它在 ifup 时先删 flowtable 再 reload。
注意这台机器的 5.4 内核连 `nft delete flowtable inet fw4 ft` 都返回 `Resource busy`，必须先
`nft delete table inet fw4`（整表）才有用；而且它覆盖不到非 ifup 的 reload，所以选了“关卸载”。

**别做的事**：不要在 LuCI 防火墙页勾“软件流量分载”——一勾就把这个坑重新打开。

## 6. 快速自检

```sh
uname -r                                              # 6.18.60（当前，语音也在这上面跑通）
cat /mnt/mu300-disk/boot/kernel                       # 5.4 / 6.18，当前选择
cat /proc/asound/cards                                # sprdphone-sc2730（card1）

# —— 语音/DSP：只看这一行，别的都不算证据 ——
/usr/bin/mu300-audio-dsp status | grep '^dsp:'
#   dsp: running - it answered a command on the first try   ← 活的
#   dsp: no reply - run 'start'                             ← 死的（整机重启一次）
# 注意：`mu300-audio-dsp start` 的输出和退出码都不能当证据。

/usr/bin/mu300-voice status                           # 要有 scene: running
/etc/init.d/airsim status                             # running
curl -s http://127.0.0.1:7581/status                  # audio / gateway.events / bind_hosts
netstat -ln | grep 7580                               # 只应听 127.0.0.1（不要放开到 0.0.0.0）

# —— 三个兼容层在不在 ——
printf 'AT+CLCC\n' | /usr/bin/mu300-at -t 6 -          # OK（验证 stdin 兼容层在）
/sbin/ifconfig >/dev/null; command -v stat || echo '本机没 stat，正常'

ubus call network.interface.wan status | head -20      # up=true, l3_device=sipa_eth0
uci get firewall.@defaults[0].flow_offloading          # 必须是 0
nft list chain inet fw4 accept_to_wan | tail -3        # 必须有 oifname "sipa_eth0" ... accept
ping -c2 223.5.5.5                                     # 设备自身出网
```

**装完包必须整机重启一次**，不能只 `restart` 服务：DSP 内存只在
`/etc/init.d/airsim-audio-reserve` 的 `boot()` 里 reserve（`start()` 是空的），
`/run/airsim-dsp-prepared` 标记还会挡住后续重试。装完不重启的典型症状是
**电话能接通但双向没声音**。

## 7. 要重编安装包（**现在只走这三个脚本**）

```sh
sh overlay/f50/build.sh              # 改上游 Go 后先重编（含补丁体检 + apply.py + 编译）
sh build/make-package.sh --build      # 出主包（先重编再打），默认打 dist/airsim-f50-<VER>.tar.gz
sh build/bump-version.sh              # 升版本号：序号 +1，r23 不动，内核范围保持 any
```

- 改上游 Go 只改 `overlay/f50/patches/*.patch`，见 §8c。**不要 `cd src/plugin && go build`**。
- 主包 = 两个二进制 + `overlay/f50/files` 整棵树；**配件包不带进主包**（`parts/README.md` 铁律）。
- 音频配件包 `parts/audio/make-opt.sh` 只打 `.ko` + install.sh + MANIFEST，模块没变就不用重打。
- 发布后跑闸三核对线上：`python3 build/tools/check-release.py --tag <tag> dist/<包>`。
- 旧的 apk 签名包流程（`scripts/package-apk.sh`）仍需要 apk-tools 3.x，本机做不了，暂不做。

## 8. 别踩的几件事

- **不要 `apk upgrade` 全量**。这台镜像的 `/usr/share/ucode/fw4.uc` 是被厂商改过的
  （`unisoc-flowtable-ports-v1`，会把只以 `l3_device` 存在的 `sipa_eth0` 纳进设备发现），
  升级 `firewall4` 会把它换回官方版，数据网立刻变差。
- **官方 kmods 源指向 6.12.94 内核**，与这台 5.4/6.18 都不匹配，`kmod-*` 一律不要从官方源装。
- `mu300-update apply` 若重铺 `/opt`，会覆盖 `sipa-dele-start` 与 `mu300-at`。**`mu300-at` 现在不用担心**：
  重装主包会把兼容层重新铺回去（`install.sh` 装前备份原厂 symlink、装后覆盖），
  `uninstall.sh` 再按记录还原。`sipa-dele-start` 那条仍是设备侧补丁，重铺 `/opt` 后要单独重打。
- **刷机会把一切手改清空**。所以规矩是：**凡是手工在机器上改的东西，一律先固化进
  `overlay/f50/files/` 或 `overlay/f50/patches/`，再往下走**。这轮（11-14）就是因为
  前几轮手打的补丁没进包，刷完机号码和通话音频全废了一遍。
- 重启只能手动物理/网页操作，不要在脚本里带 `reboot`。
- 换卡后若拨不起来，APN 手填（设备无内置 APN 库）：移动 `cmnet`、联通 `3gnet`/`uninet`、
  电信 `ctnet`（部分卡 `ctnet@mycdma.cn`）。

## 8b. 公开仓库：发布前必须脱敏（硬规则，2026-11-11）

这个仓库是**公开**的，任何进库的东西一进来就全网可见——文档、测试脚本、示例、注释、截图都算。

**不许写进仓库的东西**：真实手机号 / SIM 号 / IMEI / 口令 / 身份证 / 家庭住址 / 私人邮箱。

**照这个写**：
- 号码一律用明显假的：`+8613800000000`、`13800138000`、`15550000001`。
- 文档里要表达"实测过"，写「实测可用（号码不写进仓库）」，别把号抄上去，也别写「打码成 `86*****5721` 这种保留头尾的样子」——留头尾一样能对。
- 设备地址 `192.168.77.1` / `192.168.50.230` 是统一约定，可以写；SSH 口令不写。

**提交前跑这两条，有一条不干净就别提交**：
```sh
git ls-files | grep -vE "^\.ko$|\.apk$" | xargs grep -nE "1[3-9][0-9]{9}" \
  | grep -vE "13800138000|15550001111|15550000001|13800000000|192\.168|0\.1\.0"
git log --all --oneline -S"<可疑串>"    # 历史里也翻不到才算清完
```
真发生过一次：README 的 SIM 行和 `call-forward test` 的示例号都是真号，当前文件改了、历史里还在，
最后靠 `git filter-repo --replace-text` 重写历史 + 强推 main 与 tag 才算清掉。
做法与验证步骤见 `docs/踩坑记录.md`「公开仓库脱敏与历史重写」一节。

## 8c. 改上游 Go 只走补丁层（硬规则，2026-11-12）

`src/` 是上游原样快照，**永远不要直接改**。所有上游 Go 的改动写成
`overlay/f50/patches/*.patch`，构建时由 `overlay/f50/apply.py` 打进合并树。

踩过的坑：把改动直接写进 `src/` 之后，补丁还在顶层照旧打 → 3/4 hunk reject，
`apply.py` 退出，标准构建其实是坏的；而直接 `cd src/plugin && go build` 又能编出
"看着正常但缺功能"的二进制。两个方向都能骗过眼睛。

必须遵守：

1. 改上游 Go：只改 `patches/*.patch`，不在 `src/` 动手。
2. 新补丁必须登记 `patches/manifest.tsv`（状态 `active`）。`apply.py` 遇到未登记的
   补丁会直接退出，`check-patches.py` 也会报。
3. 构建只走 `sh overlay/f50/build.sh`。它做三件事：rsync `src/` → 跑
   `check-patches.py` 体检 → `apply.py` 打补丁（打不上硬失败）→ 编译。
   **不要**用 `cd src/plugin && go build` 验证，那棵树没有补丁。
4. 补丁改了行为就带测试，测试 hunk 写进**同一个** patch（谁改行为谁负责测试）。
   例：`AirSIM-f50-tar-update.patch` 把主包改成 tar.gz + `.sha256`，就得同时改
   `updates_test.go`，否则测试在"Release 缺少安装包校验和"挂掉。
5. 验证补丁真进了产物：`strings` 按包查功能串 —— plugin 侧查 `airsim-mu300d`，
   Agent 侧查 `airsim-agent`。"补丁数对得上"不算证据，符号可能被内联/裁剪。
6. 打不上就去改 hunk 上下文，不许 `--fuzz` / `-l` / `--forward` 绕过。
7. 写测试断言要贴代码意图，不要为了让测试变绿放宽语义（例如"安装已执行但版本
   未确认"按设计不回滚，就不该断言零安装）。

另外一条同类教训：写内核/版本判断时按**多档前缀**写全（`5.4.*` / `6.18.*` / `7.3.*`），
别拿某一档当唯一判据。之前 `airsim-bt` 的 `kernel_ok` 只比 `6.18.60`，在 7.3.1 上
恒为 false，就是漏写了其它档。

## 9. 文件地图

```text
README.md                    部署记录总览（结论、装机、坑、自检）
AGENTS.md                    本文件：给下一位接手者的最短路径
VERSIONING.md                版本编号规则（0.1.0.N-r23-any，每打包一次 N+1）
docs/接手说明.md                给人看的现状：装机三步、判活、未修复问题
docs/厂商CLI契约.md             ★ 依赖登记表 + 两条铁律 + "本机缺哪些常用命令"
docs/经验库/mu300-call-audio-dsp.md  ★ 通话静音的四层排查 + 三处判断错误复盘
docs/经验库/mu300-at-stdin-mode-mismatch.md  ★ stdin 契约不匹配那条坑
docs/历史/5.4-*.md            5.4 时代三条完整链路（历史，参考用）
overlay/f50/files/            ★★ 直接铺到设备根目录的整棵树（三个厂商脚本兼容层在这儿）
overlay/f50/patches/          ★★ 上游 Go 的补丁层（9 个 active，见 manifest.tsv）
overlay/f50/build.sh          构建入口：rsync src → 体检 → apply.py 打补丁 → 编译
overlay/f50/apply.py          补丁应用器（字母序、打不上硬失败）
build/                       造包（make-package.sh / bump-version.sh / pkg/ 包内安装脚本 / tools/）
build/tools/check-consistency.py  一致性闸（第 6 条=未登记厂商 CLI 路径直接拒包）
build/tools/check-package.py      闸二（包内容体检）
build/tools/check-release.py      闸三（线上资产 vs 本地，大小+哈希）
parts/                       配件源码与 payload（audio 按内核分 payload/<KREL>/；主包不带配件）
ops/                         设备侧一次性脚本（★ 只作历史留档，别再手打设备）
archive/                     历史包与旧整包，只留档
src/plugin/                  上游插件源码（保持原样，一个字都不改）
src/AirSIM/                  AirSIM 客户端/Agent 源码（固定提交）
src/dependencies/            重建音频扩展所需的依赖源码包
dist/                        当前这一版的安装包（不进 git）
```
src/AirSIM/                  AirSIM 客户端/Agent 源码（固定提交）
src/dependencies/            重建音频扩展所需的依赖源码包
dist/                        已验收的安装包、公钥、安装脚本、SHA256SUMS
```

## 1d. 自编包怎么出（不给 docker 也行）—— **手工方式，仅用于排查**

> 正常出包一律走 §7 的三个脚本（`overlay/f50/build.sh` + `build/make-package.sh`）。
> 下面这套手工命令只在"想绕过构建脚本单独验证某个二进制"时用；
> **注意手工编出来的二进制不含补丁**（没走 `apply.py`），别拿它当发布产物。

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

出包：`sh build/bump-version.sh` 拿新版本 → 放二进制与页面进 `pkg/airsim-f50-<版本>/`
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

> **这条结论已过期，别再照它做事（2026-11-11 修正）**：6.18.60 如今语音可用 —— 自编的 25 个 `.ko`
> 由配件包 `audio-opt 1.0.3` 按内核落地，`audio_reserve missing=0`。上面那段是"当时还没有 6.18 音频模块"
> 的现场记录，留作历史；**唯一口径以 [`docs/内核矩阵.md`](docs/内核矩阵.md) 为准**。
