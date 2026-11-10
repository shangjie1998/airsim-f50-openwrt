---
name: release-sources-tar-build-without-git-history
description: 上游只发布剥掉 .git 的 sources tar、而项目构建脚本又依赖 git 归档某个钉住的提交时，怎么在本机把二进制编出来并确认编的是新版
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [go, 交叉编译, 源码包, git-archive, release, 构建脚本, 版本核对]
---
## 适用场景
从上游 release 的 sources tar（不含 `.git`）复原构建，而仓库自带的 `build.sh` 里调用了 `git archive <pinned-commit>`、`git apply` 之类依赖历史对象的步骤，一跑就报 `Not a valid object name ...^{commit}`。

## 关键做法
- 不要为了让它跑通去伪造 git 历史。先看 `build.sh` 到底做了哪几步：通常是「取钉住的基线 → 打自家补丁 → `go build`」。
- 手工照做同样的参数：把源码树拷到临时目录，直接 `patch -p1 < patches/xxx.patch`（源码包里往往已经是打过补丁的状态，此时 patch 会提示 `Reversed (or previously applied)` 并问 y，用 `--silent` 一口气答过去即可，关键是随后能编过），再按 `build.sh` 里同样的 `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -buildvcs=false -trimpath -ldflags="-s -w -X main.version=..."` 编。
- 编完必须证明「编的确实是新版」，否则拿旧二进制冒充新版本：`file` 看 ELF 架构，`strings <bin> | grep <新版本号/新品牌串>` 命中即可；同时把新旧二进制 sha256 对比一次。

## 踩过的坑
- 直接跑 `build.sh` 会停在 `git cat-file -e <commit>`，sources tar 里没有 `.git`，补不出这个提交；不要因此去 clone 全仓库或改脚本里那条断言。
- 手工 patch 时会被交互提示卡住（`Assume -R? [y]` 一串），非交互环境里要一次性放行，并事后确认产物能编出来，而不是只看 patch 退出码。
- 只凭「编译成功」不算数：改名、版本号这类字符串编进去以后要回读二进制确认，否则很容易装上一份没改名或没升版的产物。

## 可复用的产物或模板
```sh
cp -R src/AirSIM /tmp/bld/AirSIM && cd /tmp/bld/AirSIM
patch -p1 --silent < patches/xxx.patch
cd module/module-agent && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go build -buildvcs=false -trimpath -ldflags="-s -w -X main.agentVersion=<ver>" -o /tmp/bld/agent .
file /tmp/bld/agent; strings /tmp/bld/agent | grep -c "<新版本串>"
```