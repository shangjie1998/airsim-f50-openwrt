---
name: kernel-exact-release-kit-and-audio-modules-selfbuild
description: 上游按内核小版本发布驱动 kit、但设备跑的那个小版本没有 kit 时，如何自己造构建树、编出模块并在真机装上
metadata:
    type: work_experience
    source: unknown
    created_at: "2026-11-09"
    tags:
        - openwrt
        - 交叉编译
        - 内核模块
        - kit
        - vermagic
        - config哈希
        - 展锐
        - 真机验证
---
## 适用场景
按 `<KREL>/` 分目录放驱动的扩展（如展锐音频）只发布了旧小版本的 kit，设备实际内核小版本没有对应 kit，且 vermagic 必须字面相等。

## 关键做法
先看内核开关决定可省多少：`MODVERSIONS` 关则只看 vermagic、不校验符号 CRC；`MODULE_SIG` 关则不用签名；`CONFIG_SND=y` 则自身 snd 符号在主内核里，外部模块的 undefined 符号留到 insmod 解析。用同版本源码树编出构建树即可，不必出 Image：`cp .tmp_vmlinux1 vmlinux`、`cp vmlinux.symvers Module.symvers`，再 `make O=$O ARCH=arm64 CROSS_COMPILE=… modules_prepare` 生成 `scripts/module.lds`。造 kit、编模块后逐条 `strings *.ko | grep vermagic` 必须与设备现成模块逐字相同。装到设备前用现成 `.ko` 的 vermagic 做前置比对，不一致直接退出。

## 踩过的坑
- `DEBUG_INFO_COMPRESSED_ZSTD` 依赖 `$(cc-option,-gz=zstd)`，gcc 13+ 才有；低版本交叉会 `want y got unset` 中止，把它当纯调试项忽略。
- 半成品 vmlinux 没有 `.BTF` 段，`DEBUG_INFO_BTF_MODULES` 那步会失败；关掉后必须删 `include/config/auto.conf` 再 `olddefconfig`，否则旧 auto.conf 让改动不生效。
- 交叉 strip 必须显式写 `aarch64-linux-gnu-strip`，宿主 strip 认不出 aarch64 ELF。
- 外部模块 MODPOST 报内核自身 snd 符号 undefined 时，加 `KBUILD_MODPOST_WARN=1` 放行。
- 上游启动脚本常拿 `CONFIG`（`/proc/config.gz` 排序后 sha256）与模块 `KERNEL` 逐位比，自编树差一个调试项就永远“not loaded”；这类闸门要放行并说明差在哪几项，原文件留 `.orig`。
- 用 `aplay` 打一段数据当“DSP 活着”的探针，在没有通话场景时必然 `write error: I/O error`，是假阴性；判定应改看 dmesg 里 `aud_send_cmd out,cmd ret-value:0` 且无 `failed to get command`。
- 预留内存的模块必须在干净启动时最早加载；运行中途 insmod 会报 `N pages not taken`，且模块标 `[permanent]` 卸不掉，只能重启。
- 多个补丁要从同一 `.orig` 一次生成；先打 A 再以 `.orig` 为基准打 B 会把 A 冲掉。

## 可复用的产物或模板
校验对：`strings /lib/modules/$(uname -r)/现成.ko | grep vermagic` vs `strings 自编.ko | grep vermagic`；
config 哈希：`gzip -dc /proc/config.gz | grep -E '^(CONFIG_|# CONFIG_)' | LC_ALL=C sort | sha256sum`。