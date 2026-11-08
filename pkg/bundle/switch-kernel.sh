#!/bin/sh
# F50 在 5.4 与 6.18 之间切换内核（在设备上跑，不是在本机跑）
#
# 为什么要切：语音只有 5.4 能用。音频扩展包里的 ko 是按
# 5.4.254-gb50db5b6224c 编的，6.18.x 上 /usr/lib/airsim/prepare 过不了音频预检，
# /etc/init.d/airsim 会停在 "active with no instances"，重启也不自启（数据联网不受影响）。
#
# 先看清现状：
#   uname -r                                  # 现在跑的是哪个
#   cat /mnt/mu300-disk/boot/kernel           # 已选内核：5.4 / 6.18
#   cat /mnt/mu300-disk/boot/installed.kernel # 当前启动镜像带的内核
#   cat /mnt/mu300-disk/boot/prev.kernel      # 备份镜像带的内核
#   cat /mnt/mu300-disk/boot/prev.tag         # 备份镜像版本（v2026.11.02.1 = 5.4 那版）
#
# 回 5.4（离线就能干，不需要网络）：
#   mu300-update rollback-boot      # 把 prev.body 写回 boot 分区并校验 sha256，
#                                   # 同时把 kernel choice 设成 prev.kernel 的值
#   sync; reboot                    # 重启后 uname -r 应为 5.4.254-gb50db5b6224c
#
# 注意 rollback-boot 是**一次性**的：它只写不存，不会把当前（6.18）镜像另存为新的 prev。
# 所以从 5.4 再想上 6.18，得走下载那条路：
#   mu300-update kernel 6.18        # 需要联网（从 dikeckaan/mu300-linux 拉 mu300-kernel-6.18.tar.gz）
#   然后 reboot
# 同理 mu300-update kernel 5.4 也能回 5.4，但它要联网；断网环境下只有 rollback-boot 可用。
#
# 设备没网时怎么上 6.18（已在 2026-11-08 实测走通）：
#   1) 在能上网的机器上下内核包（跟设备系统同 tag，例：
#      https://github.com/dikeckaan/mu300-linux/releases/download/v2026.11.02.1/mu300-kernel-6.18.tar.gz
#      53,175,572 字节，sha256 64893b185c5717b3c40699ab4863fb1f291a0e03eb869d50fdcf05a78014e130）
#   2) scp 到设备 /tmp/，设备上 sha256sum 核一遍
#   3) 用 MU300_KERNEL_BUNDLE 指定本地包、MU300_RELEASE 指定 tag（跳过联网查 latest_release）：
#      MU300_RELEASE=v2026.11.02.1 \
#      MU300_KERNEL_BUNDLE=/tmp/mu300-kernel-6.18.tar.gz \
#      mu300-update kernel 6.18
#   4) reboot。装完会打印 "slot b armed"，新镜像写在不常用的 b 槽，起不来会自动退回。
#
# 包里的 ./devices 必须列着 f50，否则脚本会拒绝（U30 Air 与 F50 的 ramdisk 不同）；
# 包内 ./kernel.release 应与你选的内核一致（6.18 那个是 6.18.60）。
#
# 起来以后验证：
#   uname -r
#   /etc/init.d/airsim status                      # 应为 running，不是 "active with no instances"
#   ubus call airsim status | grep audio_available  # 应为 true
#   dns-sd -B _airsim-vowlan._tcp                   # 从同一局域网的 Mac 看，实例名应是 AirSIM-F50-<8位>
#
# 切回 5.4 后 device-patches/ 里那三个补丁（mu300-at 读 stdin、sipa-dele-start 放行、run-dir.sh）
# 如果之前被 mu300-update apply 覆盖过，按 device-patches/README.md 重放。
