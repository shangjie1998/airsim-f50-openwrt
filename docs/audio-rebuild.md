# F50 语音驱动按内核重编（跨对话可续）

这份文档是给"换会话、换人、换模型"准备的：不用记得上一轮聊了什么，读完这三节就能接上。

> 装不上、服务不启、页面显示旧内容这类问题不在本文范围：先看 `docs/踩坑记录.md`。
> 云端一键编（不用装 Docker）：<https://github.com/shangjie1998/airsim-f50-audio-kits>

## 现状（2026-11-10）

| 项 | 值 |
|---|---|
| 设备内核 | `6.18.54`（vermagic `6.18.54 SMP mod_unload aarch64`） |
| 设备系统镜像 | `mu300-r20261109.184229-g115621f5-sab8ca2c5`（nightly） |
| 上一版系统 | `mu300-r20261106.185421-gb35932ab-sf1806e05`，内核 `6.18.60` |
| 手上音频驱动 | 只给 `6.18.60` 编的 25 个 `.ko`（`audio/f50/payload/6.18.60/`） |
| 当前行为 | `prepare` 检测 vermagic 不匹配 → 自动写 `/etc/airsim/no-audio` → 服务正常起、无语音 |

结论：不是 OpenWrt 升级（两边都是 `25.12.5 r33051-f5dae5ece4`），是上游 mu300-linux 的 nightly 换了内核基线（1109 构建比 1106 新，内核从 6.18.60 退到 6.18.54）。

## 三条路，先选一条

| 路线 | 做法 | 代价 | 适用 |
|---|---|---|---|
| A 换回旧系统 | 刷 `mu300-r20261106…`（6.18.60） | 现成的 25 个 ko 立刻可用；要重刷机 | 只想马上出声 |
| B 按 6.18.54 重编 | 本文档下面四步 | 要内核构建树 + 模块源码 + Linux 机器 | 长期跟上游走 |
| C 维持 no-audio | 什么都不做 | 页面/短信/来电推送正常，通话无语音 | 不急 |

> 现状说明：`airc` 无论走 A 还是 B，**代码都不用改**。`prepare` 的自动检测会在驱动补齐后自己删掉 `no-audio` 标记并尝试起 DSP。

## B 路线的硬前置（缺一个都别开工）

1. **6.18.54 的内核构建树**：`Makefile`、`.config`、`Module.symvers`、`include/generated/autoconf.h` 齐备，`make -s -C <tree> kernelrelease` 必须输出 `6.18.54`。
   - 设备上 `/lib/modules/6.18.54/` 只有 195 个 `.ko`，**没有构建树**，不能就地重编。
   - `Module.symvers` 很关键：缺了编出来的 `.ko` 可能符号对不上，移植后 `insmod` 报 `Unknown symbol`。
2. **模块源码**：确认上游 mu300-linux 里这 25 个模块的源码在不在（vendor 内核整树一般都在 `drivers/`、`sound/soc/` 下）。只有 `.ko` 没源码就做不了，只能走 A 或等厂商。
3. **aarch64 交叉工具链**：版本要能编 6.18.54；`CONFIG_SMP=y`（vermagic 里带 `SMP`）。
4. **Linux 主机**：deb 容器就够（`make`、`gcc-aarch64-linux-gnu`、`flex`、`bison`、`libssl-dev`、`bc`、`python3`）。macOS 上不保证。

## 四步走

```sh
# 0) 先把清单看清楚：要编哪些 .ko
cat audio/f50/module-list.txt

# 1) 编（后台跑，日志写盘，只看尾巴）
nohup sh audio/f50/build-krel.sh --krel 6.18.54 \
  --tree /path/linux-6.18.54 --tag nightly-20261109 --jobs 8 \
  > /tmp/audio-6.18.54.log 2>&1 &

# 2) 只看结论
tail -n 30 /tmp/audio-6.18.54.log

# 3) 校验：清单 25 个，缺 0，vermagic 错 0
sh audio/f50/build-krel.sh --check 6.18.54+nightly-20261109

# 4) 打可选件包（会自动带 KERNEL 元数据、按 vermagic 卡一道）
sh audio/f50/make-opt.sh 6.18.54+nightly-20261109
```

## 判据（什么算成功）

- `payload/6.18.54+nightly-20261109/` 里 25 个 `.ko` + `KERNEL`，`KREL=6.18.54`。
- 每个 `.ko` 的 `vermagic=6.18.54 SMP mod_unload aarch64`（脚本逐个核过）。
- `make-opt.sh` 打出的包名带 `-k6.18.54+nightly-20261109`，sha256 落盘。
- 真机装上后：`/etc/airsim/no-audio` 自动消失、维护页音频卡变「已安装」、`ubus call airsim status` 的 `audio_available` 变 `true`。

## 常见失败与含义

| 现象 | 含义 |
|---|---|
| `kernelrelease` 不等于 `6.18.54` | 构建树不是这版内核，换树；别硬编 |
| `vermagic=6.18.54 …` 但 `insmod` 仍报 `struct module size must match` | 模块源码/头文件与设备内核不完全同源，等于白编 |
| `Unknown symbol` | `Module.symvers` 不全或 `.config` 与设备内核不一致 |
| 包里少某个 `.ko` | 该模块的 `CONFIG_*=m` 没开，或源码里已删；不要绕过，先查 `.config` |
| 编出来的 `.ko` 装上去静默不工作 | 检查 `TAG` 与设备镜像版本是否对应，两版内核混用是最常见的坑 |

## 换会话怎么续

盘上的东西就是上下文：

- `audio/f50/module-list.txt` —— 要编哪些模块
- `audio/f50/build-krel.sh` —— 编译 + 收集 + vermagic 校验
- `audio/f50/make-opt.sh` —— 打多内核可选件包
- `audio/f50/payload/<KREL>+<tag>/` —— 已编好的成品
- `audio/f50/payload/_6.18.54-待重建/` —— 还没编的占位（`_` 开头，打包会跳过）
- 经验库 `experience/flash-recovery-reinstall-overlay-plugin.md`、`overlay-package-missing-upstream-base-layer.md`

新开一轮只要说"按 `docs/audio-rebuild.md` 继续"，读这份文档即可，不必复述历史。
