# F50 ↔ M8：一端开发、两端兼容的同步规矩

两台机是两条独立线：**F50 走 OpenWrt 6.18 单卡，M8 走 OpenWrt 6.18.54 双卡 16k**。
同一个功能在一端做出来，搬到另一端**从来不是复制粘贴**——硬件参数、卡槽模型、采样率、包结构全都不同。
所以这里定死一条流程：**AI 先判可行 → 人点头 → 才落地 → 各自真机验**，任何一步跳过都算违规。

## 一、三档分类（先看这个文件属于哪一档）

| 档 | 含义 | 处理方式 |
|---|---|---|
| **S（shared）** | 纯视觉/文案资产：`ui.css`、`ui.js`、`*.html`、文档、README | 可参考搬运，但**仍要人确认**，落地后两端各自截图核对 |
| **R（rewrite）** | 逻辑与参数同构但取值不同：`internal/audio/*`、`internal/gateway/*`、`cmd/*`、页面 `*.js`（绑数据）、`acl/menu json` | 必须按目标机常量改写：ALSA 节点、采样率、卡槽、机型串；改完两端分别验 |
| **D（device）** | 设备专属：`pkg/`、`m8-pkg/`、`install.sh`/`uninstall.sh`、`*.init`、`modules/`、`kbuild/` | **禁止跨端搬**。照抄等于把另一台的装机路径带进来，必炸 |

分档不是拍的，跑脚本看：

```sh
python3 scripts/sync-m8.py                # 全量报告
python3 scripts/sync-m8.py --only shared  # 只看能搬的
```

2026-11-09 首次基线（两边 overlay 归一后 54 组）：

```
shared   3   ui.css / ui.js / docs/ui-preview.html
rewrite 27   internal/audio/*、cmd/*、页面 js、acl json、patches、VERSION 等
device  24   m8-pkg/*（自带包、rpcd、init、python 小工具）
```

配对后的主要差异量（同一份东西两边差多少）：

| 文件 | 差异行数 | 说明 |
|---|---|---|
| `overview.js` | 241 | 主页：M8 双卡/电量/更多卡片，F50 单卡 |
| `sms.js` | 186 | 短信：M8 有分池与卡槽选择，F50 只有单卡 |
| `acl.d/luci-app-airsim.json` | 64 | 权限点：M8 多好几个 rpcd 动作 |

## 二、设备差异硬表（改之前对着抄，别靠记）

| 项 | F50 | M8 |
|---|---|---|
| 内核 | `6.18.60`（kano 线） | `6.18.54-m8-probe12` |
| 声卡 | `card1` `sprdphone-sc2730` | `card0` |
| 采集 | `hw:1,16`，8 kHz | `hw:0,9`，8 kHz |
| 播放 | `hw:1,53`，8 kHz | `hw:0,18`，**16 kHz 宽带** |
| 卡槽 | **单卡** | 双卡（有选卡、分池、卡号上下文） |
| 音频模块 | `/mnt/mu300-disk/extra/airsim-audio/modules/6.18.60/`（自编） | 自己那套 probe 模块 |
| 包结构 | 覆盖式更新 + `overlay/f50/`（strings.tsv + 整文件） | 自带 `m8-pkg/` 完整包 + `overlay/m8/`（路径对齐 src） |
| UI 页面 | `overview/manage/sms.js`（跟随上游插件包内根） | `ui.css`+`ui.js`+6 个页面（设计系统集中） |
| DSP 保留页 | **需要抢 2176 页**（有 `airsim-audio-retry`） | 无此机制 |
| 蓝牙 | 只有开关，不做通话桥 | 有 `airsim-bt` 一整套（含地址修正） |

## 三、同步流程（每次照走，不许跳）

1. **跑基线**：`python3 scripts/sync-m8.py`，拿到本次要动的那几行的分档。
2. **AI 判可行**，产出一段结论（贴给人看），四句话必须齐：
   - 这次搬什么（文件 + 属于 S/R/D 哪档）
   - 哪些取值必须改（列出目标机的 ALSA 节点 / 采样率 / 卡槽 / 机型串）
   - 有什么拿不准的（要么列验证动作，要么说"没验过，不敢保证"）
   - 建议：直接搬 / 改写后搬 / 不搬
3. **人确认**。没点头不许动文件——这一步是硬闸，不是形式。
4. **落地**：只改本端 overlay，不碰 `src/`；改上游文件必须首行打 `// M8:` 或 `# F50:` 留痕。
5. **本端自验**：编 → 装 → 真机跑 → 留证据（命令输出/截图）。
6. **另一端再验**（若这次是双向同步）：验证通过才写进各自的账本/坑表。

## 四、UI 样式同步口径（重点）

- **视觉真源是 M8 的 `ui.css`**（279 行，`:root` 里一套 `--as-*` token：底色 `#f5f5f7`、玻璃卡、圆角 20/12、毛玻璃 22px/180%）。
- F50 **只搬 token 与组件样式**，不搬页面里的数据绑定（那边绑的是双卡/电量/分池字段）。
- F50 目前是"整文件替换上游页面"，所以落地方式建议：新增 `overlay/f50/files/.../airsim/ui.css`（同样的 token 名），页面 js 里只做**最少的** class 引用改动；**禁止**把 M8 的页面整份塞进 F50 覆盖上游。
- 冒烟判据（两端通用）：主页、通话页、短信页、设置页各截一张，比对底色、卡片圆角/阴影、按钮蓝 `#0071e3`、深色底 `#101013` 是否一致；布局信息密度可以不同（设备功能本就不同）。

## 五、云端心跳 / 拨号超时类：**不许跨端照搬**

M8 上出现"云端心跳断、媒体拨号 TLS 超时"，**不能推导 F50 也有**。原因：

- 两条链路的路由不同（F50 走 `sipа_eth0`/蜂窝 + flow_offloading=1；M8 是另一套网卡与探针内核）；
- 双卡与单卡的 AT 上下文、选卡残留完全不同，很多"心跳丢"其实是拨号选了别的卡（见 M8 的 `docs/PENDING-cloud-dial-missing-sim-account.md`）；
- TLS 预算（TCP 8s→15s、握手 8s→30s）是**按蜂窝实测调的**，照抄到另一台可能把真问题掩盖成超时。

**正确做法**：在目标设备上先复现（留 `/status` + 日志 + 时间戳），确认症状同源，再谈改。
复现不了就记"未复现"，别改。

## 六、禁止清单

- 禁止把 `m8-pkg/` 的任何文件搬进 F50；反之亦然。
- 禁止把 M8 的 `hw:0,9` / `hw:0,18` / 16 kHz 带进 F50（F50 是 `hw:1,16` / `hw:1,53` / 8 kHz）。
- 禁止把 F50 的 `airsim-audio-retry` 塞进 M8（M8 没有 DSP 抢页问题，装了纯空转）。
- 禁止用"另一端已经验过"当作本端验收证据。
- 禁止在两端仓库之间共享路径、脚本参数化、或做软链（工具放 `~/Documents/doubaokaifagongju`，项目脚本各留各的）。

## 七、脚本在哪、怎么用

`scripts/sync-m8.py` 只读，不改任何文件；默认按同级目录找 `../airsim-mu300`。

```sh
# F50 里跑，看 M8 有什么可搬
python3 scripts/sync-m8.py --only shared

# M8 里跑，反向看 F50 有什么可搬（路径按实际改）
python3 /path/to/airsim-f50-openwrt/scripts/sync-m8.py --f50 /path/to/airsim-f50-openwrt --m8 .
```

报告里 `单边` 表示只有一台有；有差异行数的表示两边都有、内容不同——后者才是真正的"待同步候选"。

## 八、功能级同步清单（2026-11-09 首次判定）

样式只是皮，真正要同步的是功能。逐个判过，结论如下（**未落地，等人点头**）：

| # | 功能 | M8 出处 | F50 现状 | 档 | 判定 |
|---|---|---|---|---|---|
| 1 | 备份与恢复（指纹校验 + tar 白名单 + 拖拽 + 二次确认） | `rpcd/airsim` 的 `backup_make`/`backup_restore`/`airsim_fingerprint` + `maintenance.js` | **没有**（rpcd 只有 status/settings/updates/pair/call） | R | **建议做**。设备无关，但白名单路径要换成 F50 的（`/usr/lib/airsim`、`/etc/config/airsim`、`/etc/config/f50-bt`），指纹里的型号串改 `AirSIM for F50`，8MB 上限够用 |
| 2 | 自报号码补 `+86`（E.164） | `patches/AirSIM-e164.patch` → `module-agent/sms.go` 的 `parseCNUM` | **没有**；但 `src/AirSIM/module/module-agent/sms.go` 就在，同一文件 | R | **建议做**。只影响显示，单卡更简单；要重编 agent |
| 3 | 配对客户端列表 / 删除 | `main.go` 的 `/pair/clients`、`/pair/remove` + `gateway.go` 的 `ClientInfo`/`Remove` | **没有接口**；但 `/status` 已有 `paired_clients=2`，说明表在 | R | **建议做**。换手机后旧配对清不掉，这是真痛点；要重编 daemon |
| 4 | 来电转发 | `m8-pkg/airsim-call-forward*` + `call-forward` 主程序 | 没有；但 `/usr/bin/mu300-at` 在 | R | **可做，排后面**。M8 版按卡槽切 AT，F50 单卡要删掉选卡逻辑；先验 `mu300-at` 单卡路径 |
| 5 | 外观设置页（浅/深/跟随 + 密度 + 玻璃） | `ui.css` + `ui.js` + `settings.js` | 无 | S | **建议做**，纯前端，零硬件风险 |
| 6 | 连接设置单独成页 | `connect.js` | 挤在 `manage.js` 里 | S/R | 可做，字段 F50 都有（relay/端口/LAN） |
| 7 | 维护页（备份/更新/诊断/版本说明） | `maintenance.js` | 部分在 `manage.js` | R | 跟 #1 一起做，不然是空壳 |
| 8 | 短信池镜像 + 分槽 | `mu300-sms`、`sms-pool-sync.py`、`sms-dash.py` | 无 | D | **不搬**。双卡专属，F50 单卡没有池的概念 |
| 9 | 流故障回调（`OnStreamFault`）+ 每秒轮询 | `session.go`、`main.go`、`fast-hangup.patch` | 有 URC 事件，挂断本来就快 | — | **不搬**。这是"M8 有问题不代表 F50 有"的典型：M8 没 URC 才要每秒轮，F50 照搬等于白耗 |
| 10 | 日志裁剪 `keepLogsBounded` | `main.go` | 根盘 29G、只用了 724M | — | **不搬**。M8 根盘 4G 才需要 |
| 11 | 16k 播放 / AP voice scene | `devices.go`、`voice_scene.go` | 8k `hw:1,53`，不需要 scene | — | **不搬**。硬件路径不同 |
| 12 | 云端心跳/拨号 TLS 预算 | `PENDING-cloud-dial-missing-sim-account.md` | 未复现 | — | **不搬，先在 F50 复现** |

### 建议顺序

1. **#1 + #7（备份/恢复 + 维护页）**：收益最大、不碰音频、不动内核，一台做完另一台抄结构只换路径。
2. **#2 + #3（+86 + 配对管理）**：都要重编（agent / daemon），凑一次打包，省一次装机。
3. **#5 + #6（外观页 + 连接页）**：纯前端，可和上面任意一波合并。
4. **#4（来电转发）**：最后做，要真机验 AT 单卡路径。

每做一项，两端各跑一次 `python3 scripts/sync-m8.py` 复核分档，并在各自账本/坑表登记。
