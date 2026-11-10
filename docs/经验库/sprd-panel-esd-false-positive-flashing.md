---
name: sprd-panel-esd-false-positive-flashing
description: 展锐平台 OpenWrt 设备小屏周期性白闪、整屏闪一下又恢复时，判定为面板 ESD 检测误判并临时关闭、开机持久化
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [openwrt, 展锐, sprd-drm, mipi面板, esd, 白闪, sysfs, 真机验证]
---
## 适用场景
OpenWrt / 展锐(Unisoc)平台设备自带 MIPI 小屏出现规律性白闪——每隔一两秒整屏白一下再恢复，重启只能暂时缓解、之后复发，且与上层页面渲染无关时使用。

## 关键做法
1. 先排除上层：确认显示服务（cage/cog、framebuffer、WebKit 页面）进程在跑、页面本身深色底正常，把问题归到面板侧。
2. 定量取证：采够时间从 /proc/kmsg 或 dmesg 数 `esd check failed` / `esd recovery` 的条数，条数与"每秒一次"的量级对上，就说明是驱动 ESD 轮询在反复走恢复流程。
3. 看参数：路径形如 `/sys/devices/platform/soc/*/<dsi>/display/panel0/`，读 `esd_check_enable`、`esd_check_period`、`esd_check_register`、`esd_check_value`。若报错日志里读回值恒定（如 0x00）而期望值另有其值（如 0x9c），即该屏 DSI 读回通道根本不返回期望值，检测本身误配。
4. 临时关闭：`echo 0 > esd_check_enable`，再采样同样时长，事件数应归零。
5. 持久化：写 `/etc/init.d` 启动脚本（START=99）循环重试写入 `esd_check_enable=0`（面板 sysfs 可能晚于脚本就绪），`enable` 生成 rc.d 链接，避免重启复发。
6. 根治：dts 把 `sprd,esd-check-enable` 置 0 重编内核，sysfs 只是运行时补丁。

## 踩过的坑
- 短采样容易误判：90 秒窗口恰好 0 条会得出"仅上电瞬间"的错误结论，必须按条数而不是有没有来判定。
- 关闭检测后若屏幕整屏纯白，不是页面丢了，而是面板停在 esd recovery 的 disable→enable 中途（背光仍亮、dpms 仍为 On）。此时 `echo` 进 `/sys/class/drm/*/dpms` 会 Permission denied，不要指望它，直接重启屏幕 UI 服务（如 e5-infoscreen）重建输出即可。
- 设备 busybox 常无 base64，传脚本用 `openssl base64 -d -A` 解回。
- 排查完记得清理塞进 /tmp 的 grep 探针与临时结果文件。

## 可复用的产物或模板
```
#!/bin/sh /etc/rc.common
START=99
STOP=10
PANEL=/sys/devices/platform/soc/soc:dpuvsp/31300000.dsi/31300000.dsi.0/display/panel0
start() {
	[ -e "$PANEL/esd_check_enable" ] || return 0
	i=0
	while [ "$i" -lt 60 ]; do
		echo 0 > "$PANEL/esd_check_enable" 2>/dev/null && return 0
		sleep 1; i=$((i + 1))
	done
}
```