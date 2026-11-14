# 厂商 CLI 契约清单（F50）

上游 agent 会直接 exec 几个**厂商固件自带的**命令行工具。这些不是我们的文件，
刷机（或厂商镜像更新）会把它们换回新版，**调用契约可能在无声中改变**——最典型的是
`-` / `--stdin` 这种「从 stdin 读」的约定：新版脚本只认 argv，于是传进去的 `-` 被当成
一条 AT 命令原样发给基带，回一个错误但退出码仍是 0，上层只看到「功能坏了」。

所以：**每一处对厂商 CLI 的调用都必须在下面登记，并说明保护方式。**
`build/tools/check-consistency.py` 会扫源码里的这些绝对路径，没登记的构建直接报错。

| 调用路径 | 谁在用 | 契约 | 保护 |
|---|---|---|---|
| `/usr/bin/mu300-at` | `module-agent/mu300.go:28`（`-t N -`，末尾 `-` = 命令走 stdin） | argv + stdin | `overlay/f50/files/usr/bin/mu300-at` 兼容层（探测式，刷机后自动回退 argv） |
| `/opt/mu300/bin/mu300-sms` | `module-agent/mu300_sms.go:333`（`send --stdin`，stdin 是「号码\n正文」两行） | argv + stdin | `overlay/f50/files/opt/mu300/bin/mu300-sms` 兼容层 + `install.sh` 把原版备份成 `.fw-real` |
| `/usr/bin/alsaucm_test` | `module-agent/voice_runtime.go:710`（语音校准，配 `/run/alsaucm_test` FIFO） | 二进制，非脚本 | **本机不存在**（2026-11-14 实地确认）。属另一款内核 3.18.44 / mdm9607 设备的方案，F50 不适用。不保护，只登记 |
| `/opt/mu300/lib/run-dir.sh` | 音频配件包 `bin/mu300-voice:74`（`. "$RUN_DIR_LIB"` 后调 `run_dir_take`） | shell 函数库，提供 `run_dir_take <dir> <uid> <name>` | **本机不存在**（固件版本比配件包旧）。缺了 `mu300-voice` 启动即失败，PCM 后端永远 not ready，表现为「通话接通但静音」。`overlay/f50/files/opt/mu300/lib/run-dir.sh` 补实现（也可用 `MU300_RUN_DIR_LIB` 指向别处） |

## 这台设备上不存在的“常用”命令

这些不是厂商 CLI，而是我们写 shell 时**习惯性以为到处都有**的东西。这台 F50 的
busybox 是裁剪过的，**缺了不会报错，只会静默给出空值或不同行为**，比直接失败更难查。

| 命令 | 状态 | 踩过的坑 |
|---|---|---|
| `stat` | **不存在**（`ash: stat: not found`） | `run-dir.sh` 里用 `stat -c %u` 取目录属主，永远拿到空值 → 每次都判“目录被占用”并 `rm -rf`，把 `mu300-voice` 的 `scene.pid` 一起删了，场景永远判“没在跑”。改用 `ls -ldn \| awk '{print $3}'` |
| `timeout` | **不存在** | 想用 `timeout 6 cmd` 判断命令是否常驻，直接 `ash: timeout: not found`。改用 `cmd & p=$!; sleep N; kill -0 $p` |
| `nohup` | **不存在** | 后台观察脚本起不来。用 `setsid`（这个有）或 `start-stop-daemon` |
| `depmod` / `base64` / `od` | **不存在** | 只有 `hexdump` |

写 shell 前先在设备上 `command -v` 一下，别照抄 POSIX 手册。

## 两条铁律

1. **改厂商 CLI 的调用方式，不许改设备。** 改设备上的文件刷一次机就没了，等于白干。
   只能落在 `overlay/f50/files/` 里，跟着包走。
2. **兼容层必须扛得住厂商脚本换代。** 不要判断「当前是哪个版本」，而是**探测能力**
   （grep 真脚本里有没有那个 flag）：支持就原样转交，不支持就替它做转换。这样厂商
   将来升级或回退都不影响。

## 加新依赖时

源码里第一次出现新的 `/opt/mu300/bin/*` 或 `/usr/bin/*` 调用时，**同一次提交**里要
加上这里的表格行，否则 `check-consistency.py` 会拦下来。

判断风险的方法：`file` 看一眼是不是 shell 脚本——脚本才有「契约随固件变」的问题；
静态二进制接口一般稳定，但也别假设。

**检查的覆盖边界**：`check-consistency.py` 只扫 Go 源码里的绝对路径字符串，因此
下面这类它扫不到，得靠人登记：
- shell 脚本里 `.`/`source` 的厂商函数库（如 `/opt/mu300/lib/*.sh`）；
- 厂商脚本通过环境变量间接引用的路径（如 `MU300_RUN_DIR_LIB`）；
- 只在配件包里出现、Go 侧完全不知情的依赖。
所以本表不是「检查器清单」，是**人的记忆**：凡是我们的件要依赖厂商固件里的东西，
不管它是怎么被调起来的，都记一行。
