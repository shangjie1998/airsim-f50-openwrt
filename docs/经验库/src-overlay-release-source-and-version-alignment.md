---
name: src-overlay-release-source-and-version-alignment
description: 分层「上游 src + 自建 overlay」仓库要改自己的发行渠道和版本口径时，成套改版本注入源、Release 仓库常量、tag 正则与附件名，避免页面版本和升级检查自相矛盾
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [src-overlay, 版本号, github-release, ldflags, 正则, 静默失败, 嵌入式, openwrt]
---
## 适用场景
仓库用「上游 src/ 原样 + overlay/ 自建」结构，自己另发整包（版本口径和上游不同，如 0.1.0.13-r23-6.18 对应上游 0.1.0-r23），但页面「当前安装」和「检查更新」还在读上游那套。
## 关键做法
1. 先找版本号真实注入点：Go 那侧多半是 build 脚本里的 `-X main.version=$(cat .../VERSION)`，容易仍指向上游的 `src/plugin/VERSION`，要改成仓库根自己的 VERSION。
2. 发布源是成套的：仓库 URL、releases API、tag 正则（段数要跟自己的 tag 一致）、选包时的附件名判定，四处必须一起改；只改 URL 不改正则会直接 no_release。
3. 改上游 Go 源码走 overlay 补丁 + manifest 登记，构建时打不上就失败退出，不静默漏功能。
4. 验证看接口回读，不看编译通过：`ubus call <svc> status` 的 version 与升级检查返回的 current/latest/url 是否都指向本项目 tag。
## 踩过的坑
- 只换了页面链接文案不算改，后端常量未动时检查结果仍报上游版本，且 status 会变成「ahead」这种看似正常的错误态。
- tag 段数变了但 versionParts 切分长度没跟着改，比较会静默失败。
- 在线安装链若依赖别的包格式（如 apk verify/add），换成 tar.gz 主包后不能复用，先把安装按钮关掉，只留检查+下载，别让用户点到必然失败的流程。
## 可复用的产物或模板
（无）