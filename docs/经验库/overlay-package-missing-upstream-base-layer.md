---
name: overlay-package-missing-upstream-base-layer
description: 出厂或刷机后的空设备装自建 overlay 插件包，服务起不来、ubus 对象缺失时，补齐包内上游基础层并区分首次安装与覆盖更新
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [openwrt, 打包, overlay, src-overlay, init.d, rpcd, 首次安装, 静默失败]
---
## 适用场景
仓库是「上游 src/ + 自建 overlay/」分层结构，打包脚本只把 overlay 那棵树打进安装包。设备刷机或恢复出厂后（overlay 内容全丢），装同一份包会出现：ubus 里没有插件自己的对象、`/etc/init.d/<插件>: not found`、页面路径 404，但安装脚本本身报成功。
## 关键做法
先确认哪些文件来自上游打包目录而不是 overlay：`/usr/libexec/rpcd/<插件>`、`/etc/init.d/<插件>`、启动前置脚本、menu.d 与 acl.d 骨架通常都在上游 `packaging/<平台>/root/` 里。打包时先铺上游这棵树，再叠 overlay 同名文件（overlay 优先），保证包既能覆盖更新也能在空设备上装完整。
安装脚本不要用「某个二进制存在」硬性拦截整个流程，而是把它当判定条件拆成两条路：存在＝覆盖更新（正常备份）；不存在＝首次安装（跳过备份、`mkdir -p` 目标目录、安装后自动打开服务开关并 `enable` 开机自启，因为上游默认配置常是关闭状态）。两条路共用的部分保持不变。
## 踩过的坑
包只含 overlay 时，覆盖更新场景看不出问题，只有在空设备上才暴露，且报错发生在服务启动阶段而不是安装阶段，容易被误判成内核或依赖问题。改成先铺上游基础层后，同一个包在刷机后的设备上一次装好、对象齐全、服务可启动。
首次安装若不动服务开关，装完服务是停的，页面显示「服务已关闭」，看起来像安装失败。
## 可复用的产物或模板
```sh
rsync -a "$HERE/src/plugin/packaging/<平台>/root/" "$STAGE/files/"
rsync -a "$HERE/overlay/<机型>/files/" "$STAGE/files/"
```
install.sh 里：
```sh
[ -f /usr/lib/<插件>/<主程序> ] && FRESH=0 || FRESH=1
if [ "$FRESH" = 1 ]; then
	uci -q set <插件>.main.enabled=1 && uci -q commit <插件>
	/etc/init.d/<插件> enable >/dev/null 2>&1 || true
fi
```