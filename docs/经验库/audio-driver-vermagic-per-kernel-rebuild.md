---
name: audio-driver-vermagic-per-kernel-rebuild
description: 设备内核变了导致厂商 .ko 装不上（struct module size must match）时，先确认上游有没有该内核的构建套件，再决定自编、换回旧镜像还是等上游，并给出按 release 分类打包与校验的做法
metadata:
    type: work_experience
    source: unknown
    created_at: "2026-11-10"
    tags:
        - openwrt
        - 内核模块
        - vermagic
        - 交叉编译
        - 构建套件
        - 音频
        - 嵌入式
---
## 适用场景
刷系统后厂商或第三方内核模块装不上（`struct module size must match the kernel's built struct module size at run time`），需要判断能否按当前内核 `uname -r` 重建，以及怎么重建、怎么验证。

## 关键做法
先别急着编，按三层顺序确认能不能编：

1. **认 vermagic**：`grep -ao 'vermagic=[^[:space:]]*' xxx.ko`，与设备 `uname -r` 比对。这句报错就是 ABI 拒绝，任何 shell 兜底、重试、改权限都无效，只有「重编」或「换回对应内核的镜像」两条路。
2. **找模块来源**：设备上通常有一份来源清单（如 `sources.json`）记着源码仓库、commit、参考模块包的 sha256。照它找到模块源码仓库，读它的编译脚本，看清依赖什么。
3. **查上游是否已为这个 KREL 出过构建套件**。很多厂商级模块仓库不是"给源码自己编"，而是靠预制的 kit（内核头文件＋Module.symvers＋外置源码）＋固定 Docker 镜像在 arm64 上编。查 `kits.lock` 之类的锁文件**全部历史**（`git log -p -- kits.lock`）能看出它到底支持过哪些 KREL。设备上的 KREL 不在历史里，就说明上游从没为它编过，自编等于把上游内核构建重跑一遍。

确认要自编时的硬条件（缺一个就是白编）：与设备同一版的内核源码＋同样的 `.config`（可 `zcat /proc/config.gz` 从设备导出）、`Module.symvers` 齐备（缺了会 `Unknown symbol`）、`CONFIG_SMP=y`（vermagic 带 `SMP`）、`CONFIG_LOCALVERSION` 与设备一致、交叉工具链版本对得上、**arm64 主机**（套件里的 host 程序是 arm64，x86 上跑不了）、足够磁盘。

产物按 KREL 分目录（`payload/<KREL>` / `payload/<KREL>+<tag>`），日志单独文件、只回读结论；未完成的内核留 `_` 前缀占位目录，打包脚本跳过，既不阻塞全量打包又留痕。

## 踩过的坑
- 只看到 `vermagic` 就够了，忽略 `Module.symvers`：编出来能过 vermagic 检查，装上去照样 `Unknown symbol`。
- 以为有源码就能编：上游用的是「kit ＋ 固定镜像 ＋ arm64 原生」，没有对应 KREL 的 kit 就无从下手；而 kit 只有上游发过的那几个 KREL。
- 在 x86 云主机上试编：套件内主机程序是 arm64，直接跑不了；磁盘不够时内核源码＋ThinLTO 输出也放不下。
- `grep -o` 取 vermagic 可能不带尾随空格，比较时 `case "$v" in "$KREL"|"$KREL "*)` 两种都要匹配，只匹配带空格的会漏。
- `set -u` 下 `echo "缺 $n，共 $m"` 里变量紧跟中文标点，dash 会把 UTF-8 字节吃进变量名报 `unbound variable`；中文后接变量一律写 `${n}`。
- 重编完别忘了真机验证：驱动补上后应能看到模块被加载、原先自动写的"无驱动"标记被清掉。

## 可复用的产物或模板
按 KREL 分类的目录约定与校验入口（本仓库 `parts/audio/`）：

```sh
# 校验已有产物：清单逐个核 vermagic，不用真编
sh parts/audio/build-krel.sh --check 6.18.60

# 自编（需同一版构建树，且 kernelrelease 必须等于 --krel）
sh parts/audio/build-krel.sh --krel 6.18.54 --tree /path/linux-6.18.54 --tag nightly-20261109
```

脚本内建的判据：`make -s -C <tree> kernelrelease` 必须等于 `--krel`，每个 `.ko` 的 vermagic 必须匹配，才允许产出 `KERNEL` 元数据（KEY/KREL/CONFIG=配置哈希/TAG）。