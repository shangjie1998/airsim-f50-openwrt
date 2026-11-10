---
name: luci-self-update-link-fork-repo
description: 把自建 LuCI 插件的「检查更新/下载」从上游仓库改到自己 fork 时，为什么只改字符串会选错包或永久 no_release，以及该怎么一并改
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [openwrt, luci, 插件更新, fork, github-release, 版本号解析, 静默失败]
---
## 适用场景
自建或 fork 的 OpenWrt/LuCI 插件页面带「检查更新 / 下载并更新」，发布物形态与上游不同（例如上游发 apk，自己发整包 tar.gz），要把搜索源和链接指到自己仓库时。

## 关键做法
先找全落点：前端 URL 常量 + 后端 `releasePage` / `releasesAPI`（或同类常量），别漏后端。接着读后端两段逻辑——"哪个 Release 算可安装"（资产名匹配）和"版本号怎么解析"（正则）。只要自家资产名或 tag 后缀与上游规则不同，就必须同时改资产匹配和版本正则，再决定在线安装是否保留。若安装动作（apk 签名校验等）与自家包制式不兼容，就别硬留「下载并更新」，把 no_release 文案改成指引用户去 Release 页手动下载整包，避免点出注定失败的流程。改完用浏览器回读 Release 链接的 href，再点一次「检查更新」确认返回状态和文案。

## 踩过的坑
只改前端 URL 常量，后端仍打上游 API，检查更新会拿上游包当可装版本，装到自家设备上；版本正则不认自家后缀（如 `-m8`）会判成 unknown_version，永远比不出来；替换设备上正在运行的可执行文件时 scp 会报 `Text file busy`，要先停服务、替换、再启动。

## 可复用的产物或模板
（无）