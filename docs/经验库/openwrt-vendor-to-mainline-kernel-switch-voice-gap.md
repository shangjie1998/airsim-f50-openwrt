---
name: openwrt-vendor-to-mainline-kernel-switch-voice-gap
description: 厂商定制 OpenWrt 设备从 vendor 内核切到 mainline 内核时，如何判断哪些依赖闭源驱动的功能会失效，以及切换前后该做什么
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-07
 tags: [openwrt, kernel-switch, mainline, vendor-kernel, audio-driver, sipa, rollback]
---
## 适用场景
设备上已并存 vendor 内核与 mainline 内核（如 /lib/modules 下同时有 5.4.x 和 6.18.x），想切内核验证某个功能在新内核上是否可用。典型是蜂窝模组/PDP 拨号、语音 DSP 这类依赖厂商专有模块的能力。

## 关键做法
1. 切换前先确认存在厂商自带的更新工具（如 `mu300-update kernel <版本>`），它会重新组装 boot 分区并回读校验，比手工 dd 安全。
2. 先整分区备份 boot 镜像（`dd if=/dev/<boot分区> of=<linux分区>/boot/boot-<版本>-backup.img bs=1M`），记下 sha256 前几位。多数工具还带"连挂 N 次自动退回 android"和 `rollback-boot`，但独立备份是最后的兜底。
3. 切换工具的输出要读懂三件事：装进哪个系统（openwrt/ubuntu）、模块数量、新 boot 镜像版本号；缺任何一项都不算切换完成。
4. 重启后按顺序核验：`uname -r` → 数据功能（就绪接口是否存在、拨号地址、默认路由）→ 防火墙链是否带新设备 → 从另一台机器实测客户端出网。
5. 功能失效时优先看服务 init 脚本自己的错误输出，而不是服务状态。`active with no instances` 这类含糊状态通常掩着真正原因，手动 `start` 一次就能拿到。

## 踩过的坑
- 通用音频模块（snd_pcm、snd_aloop、snd_timer）在新内核上照样能 lsmod 加载，但这不代表语音通道可用。厂商专有的 DSP/PCM 附加包里的 .ko 是按旧内核 vermagic 编的，切换后会直接报"drivers for <旧版本> and this is <新版本>"，依赖它的语音服务起不来。判断方法：切之前先 `modinfo` 目标内核模块目录下厂商专有 .ko，看 vermagic 是否匹配，或直接看附加包的安装日志。
- 不要指望"新内核更规范所以功能更全"。本次实测相反：mainline 内核上 sipa 反而有 vendor 内核缺失的就绪接口、也没有 vendor 内核的 flowtable 事务冲突，数据侧更干净；但语音侧因为闭源驱动缺失直接归零。两类功能的迁移动因完全不同，必须分开判断。
- 切换后 5.4 时代打过的兼容补丁（如"就绪接口缺失则放行"）会变成空跑，不冲突但要知道它已不再生效，别在排障时被它误导。

## 可复用的产物或模板
```sh
# 切前备份
dd if=/dev/mmcblk0p38 of=/mnt/mu300-disk/boot/boot_b-backup.img bs=1M
sha256sum /mnt/mu300-disk/boot/boot_b-backup.img | cut -c1-16

# 查厂商专有驱动支持哪些内核
modinfo /lib/modules/<新内核>/<厂商模块>.ko | grep vermagic

# 切换与回退
<厂商工具> kernel 6.18
<厂商工具> rollback-boot
```