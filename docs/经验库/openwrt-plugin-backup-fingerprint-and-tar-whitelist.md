---
name: openwrt-plugin-backup-fingerprint-and-tar-whitelist
description: 给嵌入式设备的自建 LuCI 插件加备份/恢复功能时，怎么防别人家的备份被导进来、以及为什么路径白名单会把自己的包也挡掉
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [openwrt, luci, rpcd, 备份恢复, tar, 白名单, 硬件指纹, shell, 静默失败]
---
## 适用场景
设备侧的插件要给用户"备份到电脑 / 从电脑恢复"，且备份里含配对凭据或设备身份，不能让别人设备的备份被导进来时。

## 关键做法
1. 机器标识：用稳定硬件指纹，如 `/sys/block/mmcblk0/device/cid` + `/proc/device-tree/serial-number`，拼起来算 sha256，取前 8 位作为人可读短标识。
2. 备份包内写 `meta.json`（`format`、`format_version`、`fingerprint`、`short_id`、`model`、`hostname`、`plugin_version`、`created_at`），恢复前先只读这个文件比对指纹，不符立刻拒绝并回显"文件那位 / 本机这位"两个短标识，让人一眼看出拿错文件。
3. 解包前用 `tar tzf` 列表做路径白名单，只放行本插件自己写的路径（`meta.json`、`config/` 下指定配置、`agent-data/`、`data/`），防止借备份覆盖系统文件；同时限制包体大小。
4. 覆盖前把现场另存一份（如 `/tmp/airsim-before-restore-<时间戳>`），再改文件；若服务原本在跑，改完自动重启。
5. 传输：设备没有 base64 命令时用 `openssl base64 -A` 打包进 JSON 交给浏览器；需要下载就用前端 `atob` + `Blob` 生成 `.tar.gz` 触发保存。

## 踩过的坑
- **白名单把自己的包也挡了**：`tar tzf` 输出除文件外还有根条目 `./` 和目录条目 `./config/`、`./data/`，只按文件路径写规则会导致"备份包内含不允许的路径，已拒绝导入"。白名单必须显式放行目录条目和根条目。
- 报错信息要能自证：同时回显 `file_id` / `local_id`，否则用户只会看到"拒绝"而不知道是拿错文件还是包坏了。

## 可复用的产物或模板
（无）