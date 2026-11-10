---
name: sprd-panel-esd-loop-white-screen-triage
description: 展锐平台 OpenWrt 安卓设备屏幕闪白/黑屏时，用内核日志的 ESD 检测循环判定是面板复位还是软件改动导致，并确认能否软件侧关闭
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [sprd, drm, lcd, esd, 白屏, 显示, 诊断, openwrt, 内核日志]
---
## 适用场景
在展锐（sprdphone/sprd_drm）平台的定制设备上，用户报“屏幕闪白屏、黑屏、花屏、屏幕没了”，且刚做过软件安装或改配置，需要判断是面板硬件/驱动自恢复循环还是软件改动引入。

## 关键做法
先确认设备是否还活着（SSH 能进、uptime、内存、服务状态），再抓显示侧内核日志：`logread | grep -e 'esd recovery' -e 'esd check failed' -e 'LCM_BIAS'`，并统计次数与间隔 `logread | grep -c 'esd recovery end'`。
典型故障指纹是三类行同时高频出现：`sprd_panel_esd_check ... esd check failed, read value = 0x00`、`======= esd recovery end =======`、`[KERNEL/LCM_BIAS]OCP2131 write enter !!`，间隔约 1.5–2 秒一次、累计数百次——说明面板寄存器读回全 0、驱动判定屏挂死并反复重新 `sprd_panel_prepare/enable`，属面板 ESD 自恢复循环，不是用户态软件所致。
再查软件侧是否有开关可关：`ls /sys/module/<drm模块>/parameters/`、`find /sys/kernel/debug -iname '*esd*'`。若都没有，说明只能断电重启或修硬件（排线/屏偏压供电），不要在用户态里继续折腾安装包。

## 踩过的坑
光看到“屏幕坏了”不足以定性，容易怀疑刚装的软件；先比对安装前后动过的路径（本项目只改 /opt 与 LuCI 文件，未碰 DRM/内核），再靠 ESD 循环计数与时间间隔把责任切开。设备“在线”不代表显示正常，SSH 通、服务 running 时面板可能仍在死循环复位。

## 可复用的产物或模板
`logread | grep -c 'esd recovery end'`；`logread | grep 'esd check failed' | tail -3`；`find /sys/kernel/debug -iname '*esd*'`