---
name: upstream-installer-fresh-device-file-assumptions
description: 上游安装脚本在全新设备上首次运行时，因假定被覆盖的旧文件/目录已存在（含空列表 tar 打包）而 set -e 中断，如何定位并安全绕过而不破坏其校验
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-07
 tags: [openwrt, busybox, tar, set-e, 安装脚本, 上游漏洞, rootfs裁剪, 预检]
---
## 适用场景
给设备刷上游发行的"完整安装包"（自带 pre-install/post-install、stage 脚本、状态机与 manifest 校验）做首次部署，脚本在检查阶段莫名失败，而设备此前从未安装过该软件。

## 关键做法
1. 先读安装脚本全文，把每个 `fail`/`die` 的判定点逐条列出，确认失败点是在"平台是否满足"还是在"上次残留是否干净"。
2. 预检要求的文件若在新 rootfs 里被裁掉，先去上游仓库的 overlay/rootfs 目录取原版正文回来补齐，别自己造一个同名占位。
3. 脚本用 `tar -cpf x.tar -C / -T list` 备份旧文件时，全新设备上 list 为空，busybox tar 直接报 `empty archive` 并返回 1；`set -e` 下会连带把已建好的状态目录写成 failed，下次运行又被"已存在候选需复核"挡住，看起来像两个不同故障。
4. 绕过方式：把脚本复制一份到临时路径，只改空列表那一行（`> x.tar` 清出空文件，`-s list` 为真才打包），restore 里对应的解包也加 `-s` 判断；原始文件保持不动，避免 manifest/校验和失配。改完先跑 `--check` 只读阶段，通过再跑 `--stage`。
5. 手动补完脚本跳过的状态写入（staged-boot、status=reboot_required、enable 开机项），让官方状态机继续认这条安装。

## 踩过的坑
- 第一次失败留下的空备份目录会让第二次运行被"Existing candidate/backup requires review"拦住，清残留前先看清目录里的 `present`/`progress` 是否为空，空的可清。
- 改 `tar -cpf` 那行仍有成员计数问题，必须改成"列表为空就不建归档"才真正通过。
- 手工补状态时不要凭空造字段，照原脚本同名字段和取值写，否则后续 resume 分支会走错。

## 可复用的产物或模板
`sed -e 's|^tar -cpf X|: > X; if [ -s LIST ]; then tar -cpf X -C / -T LIST; fi|' -e 's|^    tar -xpf X|    if [ -s X ]; then tar -xpf X -C /; fi|' 原脚本 > 补丁脚本`