---
name: kernel-version-gate-in-installer
description: 自编设备安装包依赖的驱动只对某一内核编译过时，如何在安装脚本前置判断内核并给出回退指引，避免"装得上但功能全废"的误装
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [openwrt, 内核版本, 安装包, 闸门, 可回退, 覆盖安装]
---
## 适用场景
自编的覆盖式安装包，其关键能力（音频、语音、模块）依赖只针对某一个内核版本编译的 ko 或驱动。在别的内核上包能装、进程能起、界面正常，但核心功能不可用，用户很容易误判为"包坏了"或"功能齐了"。

## 关键做法
在安装脚本最开头、改动任何文件之前读 `uname -r`，只放行目标前缀（如 `5.4.*`），否则打印当前内核、真实原因和回退命令，并以非 0 退出码结束；另留一个显式强制开关（如 `F50_ALLOW_OTHER_KERNEL=1`）供"只想看界面、不要功能"的场景。同时在包内 `MANIFEST` 常驻写 `kernel_required`、`kernel_note`、`verified` 三个字段，让设备上可自查适用边界，不必翻文档。

## 踩过的坑
不设闸门时，错核上安装会成功、服务却是 `active with no instances`，`audio_available=false`，容易被当成新包引入的故障。另外回退途径常是一次性的：镜像回退只写不存，第二次就没得退；而喂本地内核包重新安装会重写备份镜像，恢复可回退性——这两条要在指引里写清，别让用户以为随时能来回切。

## 可复用的产物或模板
```sh
K=$(uname -r)
case "$K" in
5.4.*) : ;;
*)
	if [ "${ALLOW_OTHER_KERNEL:-}" = 1 ]; then
		echo "警告：当前内核 $K，只支持 5.4.x，核心功能会不可用（已强制继续）" >&2
	else
		echo "当前内核 $K，只支持 5.4.x（驱动只编了这个版本）。" >&2
		echo "先回退：<回退命令> 然后重启；确要强装：ALLOW_OTHER_KERNEL=1 sh install.sh" >&2
		exit 3
	fi
	;;
esac
```