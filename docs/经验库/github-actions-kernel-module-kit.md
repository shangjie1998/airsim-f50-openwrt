---
name: github-actions-kernel-module-kit
description: 在 GitHub Actions 的 arm64 免费机器上按内核版本给嵌入式设备编外置模块（造 kit → 编模块）时，怎么避开 modpost 全线 undefined，以及怎么触发和读失败日志
metadata:
    type: work_experience
    source: unknown
    created_at: "2026-11-10"
    tags:
        - github-actions
        - 内核模块
        - 交叉编译
        - arm64
        - modpost
        - vermagic
        - 嵌入式
---
## 适用场景
设备内核变了，厂商 `.ko` 装不上（`struct module size must match the kernel's built struct module size at run time`），本机是 x86、没 Docker、磁盘不够，想借 GitHub Actions 的 `ubuntu-24.04-arm` 免费机器按目标内核重编一套。流程是「造 kit（构建树）→ 用 kit 编外置模块」两阶段。

## 关键做法
1. 造 kit 必须**整编** vmlinux 加 modules：`make -C $SRC ARCH=arm64 -j$(nproc)`，编完强制检查 `vmlinux` 和 `Module.symvers` 都存在再往下走。
2. 配 `runs-on: ubuntu-24.04-arm`，装 `build-essential bc bison flex libssl-dev libelf-dev patch xz-utils git curl cpio rsync`。
3. 自己下载的源码树搬移进 kit（`mv`）而不是 `cp -a`，整编一份内核要 6~8G，复制会爆盘。
4. 触发编译：`workflow_dispatch` 面板可能被上一次报错弹层遮挡（`BU_TARGET_OBSCURED`），改用推 tag 更稳：`git tag -f k-<KREL> && git push -f origin k-<KREL>`，工作流配 `on: push: tags: ['k-*']`。
5. 看失败原因：Actions 的 step 日志是虚拟化列表，`get_page_text` 只拿到几百字；点页面右上角 **Explain error**，Copilot 会把关键几行贴出来，比翻日志快。

## 踩过的坑
- **只编 `make modules` 会全线失败**。缺 `vmlinux.o` 时 modpost 把 vmlinux 导出符号全判 undefined，出现 `ERROR: modpost: "register_pernet_subsys" [...] undefined!`、`suppressed 8745 unresolved symbol warnings`，收尾 `Makefile.modpost:147: modules-only.symvers Error 1`。造 kit 时必须整编；整编后同一套脚本顺利通过造 kit 阶段（约 9 分钟）。

## 可复用的产物或模板
（无）