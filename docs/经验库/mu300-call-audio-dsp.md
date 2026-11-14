# F50 通话音频：接通了但全程没声音

2026-11-14 排查记录。号码/短信修完之后暴露出来的第二个问题：电话能拨出去、
能接通，但两边都听不到声音。

## 一句话结论

**设备侧音频后端没起来，手机连上 PCM 也被拒**。根因有三层叠在一起：

1. `mu300-voice` 缺 `/opt/mu300/lib/run-dir.sh`（固件比音频配件包旧，函数库不存在）；
2. DSP 核没执行（`ipc->dsp_ready 0`），所有音频通路（D5/D16/D53）一并打不开；
3. DSP 能不能起来**取决于开机时的内存预留时序**，只 restart 服务不行，必须整机重启一次。

## 排查路径（四层，每层都有一套独立证据）

### 第一层：手机根本没连 7580

`/status` 的 gateway 事件计数：

```
control_authenticated: 166   control_request: 180
pcm_connection: 47   pcm_authenticated: 47   pcm_backend_not_ready: 47
```

**连接和认证 47 次全过，全败在设备侧后端**。所以问题不在 7580 监听地址、不在
防火墙、不在手机端连错端口——`0.0.0.0:7580` 那种改动是白改。

`pcm_backend_not_ready` 来自 `gateway.go:414`：gateway 已连上本地 bridge 7580、
已发 `AIRSIMPCM1` 握手，但 bridge 没回 `AIRSIMREADY`。bridge 要过完
工具检查 → 设备发现 → `AT+CLCC` 确认在通话中 → `mu300-voice audio start dsp`
→ arecord/aplay 起来，才会回 READY。

### 第二层：mu300-voice 启动即死

```
/usr/bin/mu300-voice: .: line 74: can't open '/opt/mu300/lib/run-dir.sh': No such file or directory
```

音频配件包比固件新，依赖了固件后来才加的 `run-dir.sh`。
`stage-device.sh` 里本来就有 `[ -r /opt/mu300/lib/run-dir.sh ] || die 'MU300 runtime library missing'`
这条硬校验，说明当初装配件包时它是在的。

补上兼容实现（`overlay/f50/files/opt/mu300/lib/run-dir.sh`，提供
`run_dir_take <dir> <uid> <name>`）后，`--help` 能打出来了，`last_error` 也清了——
**但仍然没声音**，说明还有更深一层。

### 第三层：音频场景起不来

```
mu300-voice: the call scene did not start: /dev/snd/pcmC1D5p: I/O error
```

内核侧同一时刻：

```
[Audio:SMSG] ERR: aud_smsg_send, channel 0 not opened, ipc state : 2!,ipc->dsp_ready 0
[ASoC: VBC ] dsp_vbc_iis_master_start, Failed to set, ret: -5
```

**`ipc->dsp_ready 0`——DSP 核没跑。** 把 D16（录音）、D53（放音）单独试，一样失败：

```
arecord hw:1,16  → audio open error: Invalid argument
aplay  hw:1,53   → pcm_write: write error: I/O error
```

**所有通话音频通路都依赖 DSP，没有绕开的第二条路。**

### 第四层：DSP 为什么没执行

`mu300-audio-dsp status` 当时说的：

```
dsp: no reply - run 'start'
```

手动 `mu300-audio-dsp start`：

```
audio drivers: 19/19 loaded
DSP power is on but no command reply was verified
```

`dmesg` 里 `aud_send_cmd: failed to send command(4), ret=-5`、连 DVFS 都在报
`aud_send_cmd failed`、`vbc_dsp_func_startup return error`。

排掉的几个假嫌疑：

| 嫌疑 | 结论 |
|---|---|
| DSP 固件版本不对 | **hash 完全一致**（`378ceea7…b937`，等于脚本里的 `FW_URL_SHA256`） |
| 内存没预留够 | `the DSP's memory ... is reserved`，`0 pages missing` |
| 驱动没加载 | `19/19 loaded` |
| 有人抢 DSP | `vbc_put_agdsp_aud_access ... en = disable` 手动开 `agdsp_access_en 1` 无效 |

## 真因：开机时序

`reserve` 只在 `/etc/init.d/airsim-audio-reserve` 的 **`boot()`** 里跑：

```sh
# /etc/init.d/airsim-audio-reserve  START=10
boot() {
    /mnt/mu300-disk/extra/airsim-audio/bin/mu300-audio-dsp reserve > /tmp/airsim-audio-reserve.log
}
start() { :; }
```

然后 `/etc/init.d/airsim`（START=98）调 `/usr/lib/airsim/prepare`，里面：

```sh
elif _dir=$(audio_extra_for_kernel); then
	if [ ! -f /run/airsim-dsp-prepared ]; then
		mu300-audio-dsp start
		touch /run/airsim-dsp-prepared
	fi
```

**`/run/airsim-dsp-prepared` 这个标记会挡住后续的 `dsp start`**。而 `start()`
那半边是空的——**`restart` 服务不会重新 `reserve`**。

于是：只要不是整机重启，DSP 就可能停在半死不活的状态，而且标记还在，谁都不再重试。

**整机重启一次之后**：

```
dsp: running - it answered a command on the first try
```

再跑 `mu300-voice audio start dsp`：

```
mu300-voice: downlink unmuted
mu300-voice: VOICE-RUNNING hw:1,5 both directions, 8000 Hz, output dsp
```

## 还要留意的两个背景事实

1. **厂商固件是故意删掉通话音频的**。`/etc/init.d/mu300-audio` 是空壳，注释写着：
   *Call-audio programs and their extra DSP memory reservation were removed; do not start or load the audio DSP here.*
   是我们音频配件包把它装回来的。
2. **配件包作者自己也没在这个型号上跑通完整音频**，脚本注释原文：
   *ZTE built this board without a speaker* / *What still does not work is audio actually moving: every scene takes exactly one buffer and then stops*。
   所以 DSP 起来≠声音一定通，`FINDINGS 24` 里记的“一个 buffer 就停”可能还会咬人。

## 可复用判据

判断 DSP 到底活没活，看这一行就够：

```sh
/usr/bin/mu300-audio-dsp status | grep '^dsp:'
# dsp: running - it answered a command on the first try     ← 活的
# dsp: no reply - run 'start'                               ← 死的
```

`mu300-audio-dsp start` 的退出码和输出都**不能**当证据：它说
`DSP power is on but no command reply was verified`，只能说明“上了电但没应答”，
真正的判据是 `status` 里 `dsp:` 那一行，以及 `dmesg` 里
`aud_send_cmd` / `failed to get command` 的对照。

## 真因：三个坑叠在一起

### 坑一：缺 run-dir.sh（见上）

### 坑二：设备没有 stat 命令，把状态文件删了（最阴的一个）

补 `run-dir.sh` 时我按 POSIX 习惯写了：

```sh
owner=$(stat -c %u "$dir")
```

**这台设备的 busybox 里根本没有 `stat`**：

```sh
$ stat -c %u /tmp/t1
ash: stat: not found
```

于是 `owner` 永远为空，每个检查周期都判成“目录被别人占了”，然后 `rm -rf` 重建：

```
run_dir_take: mu300-voice: /run/mu300-voice 被 uid=? 占用，替换
```

**后果是 `/run/mu300-voice/` 里的 `scene.pid`、`output`、`scene.log` 全被删掉**。
而厂商脚本的场景判活就是：

```sh
scene_running() { [ -f "$SCENE_PID" ] && kill -0 "$(cat "$SCENE_PID")" 2>/dev/null; }
```

`scene.pid` 没了 → 永远判“没在跑”。更糟的是 `pcmhost` 进程其实**还活着**，
但没人认它，下一次 `audio_start` 又起一个新的，越积越多（实测同时挂了 3 个
`pcmhost ... /dev/snd/pcmC1D5p /dev/snd/pcmC1D5c 8000 1 0`），互相抢同一对设备，
新的那个卡在 `prepared` 之后出不来 `started`，于是 `mu300-voice` 等 5 秒超时，
再往上 `session.go:87` 的同步 `run()` 等不到返回，18 秒 deadline 一到就被杀，
`/status` 里留下 `last_error: /usr/bin/mu300-voice failed: signal: killed`。

**改用 `ls -ldn`（busybox 支持，实测返回 `0`）**，并且读不出属主时**原样沿用不删**。

### 坑三：DSP 只能在开机时起来（见下）

## 修法（已固化进包）

- `overlay/f50/files/opt/mu300/lib/run-dir.sh` —— `run_dir_take` 兼容实现，
  取属主用 `ls -ldn`，读不出时不动目录
- `build/pkg/install.sh` —— 装包前备份厂商原版；装完自检 DSP，没 running 就提示整机重启
- `docs/厂商CLI契约.md` —— 登记 `run-dir.sh` 依赖
- **装完包必须整机重启一次**，不能只 `restart` 服务：`reserve` 只在 boot 阶段跑

## 我在这一轮判断错的地方（留着给下次的自己看）

这一轮走了三层弯路，每一层都是"看起来很像原因"，但都不是。记下来不是自罚，
是因为**这三种错误有很强的复发性**，下一个坑大概率还是这三个形状之一。

### 错一：把"客户端连不上某端口"当成原因

`/api/calls/audio/host/config` 返回 `host: 192.168.225.1` 这种 F50 出厂 USB 网段的死值，
我据此判断"客户端按这个地址连 7580 连不上，所以没声音"，于是把 7580 从
`127.0.0.1` 放开到 `0.0.0.0`，还把 host 改成动态。

**错在哪**：我没有先去数"到底有没有人连"。实测 50 秒，7580 上只有 `127.0.0.1` 的
本机内部连接，**用户的手机从头到尾没碰过 7580**。真正的流量在 `/status` 里：

```
pcm_connection: 47   pcm_authenticated: 47   pcm_backend_not_ready: 47
```

**连上和认证 47 次全成功，全败在设备侧后端**。也就是"客户端那边一切正常"，
我却去修客户端链路，还把监听面扩大到全网卡，平白多开一个口子。

**教训**：**"地址看起来是错的" ≠ "客户端在用它"**。先看计数器，再动手。
`502/404` 这种"连不上"和"连上了但对面没答应"是两件完全不同的事，
`pcm_backend_not_ready` 这个名字已经在喊"问题在我这边"了。

### 错二：写 shell 时假设常用命令都存在

补 `run-dir.sh` 时取目录属主，我按 POSIX 习惯写了 `stat -c %u`。
**这台设备的 busybox 里根本没有 `stat`**（`ash: stat: not found`）。

**错在哪**：这条命令**不报错**——它只是静默返回空字符串。于是

```sh
owner="" ; [ "$owner" != "0" ]  → 真  →  rm -rf "$dir"
```

每个检查周期都把 `/run/mu300-voice` 删一遍，把厂商脚本的 `scene.pid` 一起删了。
厂商判场景活没活就靠 `[ -f "$SCENE_PID" ] && kill -0 ...`，pid 文件没了 → 永远判
"没在跑"。更糟的是 `pcmhost` 进程其实还活着，但没人认它，下一次又起一个新的，
越积越多（实测同时挂 3 个），互相抢同一对 ALSA 设备，新的那个卡在 `prepared`
之后出不来 `started`。

**教训**：**在这个设备上，任何非必需命令都要先 `command -v` 验一下**。
`stat` / `timeout` / `nohup` / `od` / `base64` 全都没有。
静默返回空值比直接报错危险得多——它会让 `if` 判反，而且不留痕迹。

### 错三：先怀疑"配置/参数"，后怀疑"进程根本没起来"

前两层我都在找"哪个参数不对"（host 地址、监听、防火墙、音量、mute），
而实际链条是 **`mu300-voice` 压根没启动成功 → 场景没起 → 后端永远 not ready**。

**教训**：**先确认"东西在不在跑"，再确认"跑得对不对"**。
`/status` 里那句 `last_error: /usr/bin/mu300-voice failed: exit status 2` 一直在那儿，
比任何配置项都显眼，我却在外面绕了两圈才回来看它。

## 验证通过的证据链（修好之后）

```sh
$ /usr/bin/mu300-voice audio start dsp
mu300-voice: downlink unmuted
mu300-voice: VOICE-RUNNING hw:1,5 both directions, 8000 Hz, output dsp

$ /usr/bin/mu300-voice status
scene:    running, output dsp (hw:1,5)
dsp:      running - it answered a command on the first try

$ ls /run/mu300-voice/
output  profiles-loaded  saved  scene.log  scene.pid
```

真机拨号：**双向有声**。

## 排查时踩过的死路（别再走）

- 以为要修 7580 监听地址/防火墙/host 下发 → 手机根本不连 7580，它走 8582
- 以为 DSP 固件版本不对 → hash 和脚本里的 `FW_URL_SHA256` 一字不差
- 以为内存没预留 → `the DSP's memory ... is reserved`，`0 pages missing`
- 以为被别的进程抢 → 手动 `amixer cset agdsp_access_en 1` 无效
- 用 `timeout N cmd` 判是否常驻 → **设备没有 `timeout`**，用 `cmd & p=$!; sleep; kill -0 $p`
