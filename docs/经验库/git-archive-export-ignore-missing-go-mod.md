---
name: git-archive-export-ignore-missing-go-mod
description: 用 git archive 从上游仓库导出固定 commit 的 Go 子目录源码后交叉编译，报依赖包找不到、而目录里根本没有 go.mod/go.sum 时，判定为 .gitattributes 的 export-ignore 把它们挡掉并从同一 commit 补回
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [go, git-archive, gitattributes, 上游源码, 交叉编译, openwrt]
---
## 适用场景
为了轻量、可复现地构建上游项目某个子模块，用 `git archive <commit> <子目录>` 导出只读基线、再打补丁交叉编译。导出成功、补丁也应用成功，但编译阶段解析依赖失败。

## 关键做法
先确认现象：报错是 `main module (x) does not contain package x/<导出路径>/...` 或 `cannot find package <依赖>`，而 `ls` 导出的 Go 子目录里确实没有 `go.mod`（或没有 `go.sum`）。
判定为仓库根 `.gitattributes` 对该路径设了 `export-ignore`。
不要改用 `GO111MODULE=off` 硬扛：那样只要有一个第三方依赖就会继续失败，而且就算凑齐也绕开了版本锁定。
正确做法是在导出后、编译前，从同一个 commit 把这两个文件补回：
`git -C <上游> show <commit>:<子模块>/go.mod > <导出目录>/go.mod`
`git -C <上游> show <commit>:<子模块>/go.sum > <导出目录>/go.sum`
再以 `GO111MODULE=on -mod=mod` 编译。

## 踩过的坑
报错文案指向依赖包和路径，不指向缺 `go.mod`，容易误判成构建目录或 GOPATH 配错，先试了 `GO111MODULE=off`，结果暴露更多缺失包，反而偏离原因。
把补回动作写进准备脚本（如 `scripts/prepare-agent.sh`）的补丁之后，下次上游升级还能一键复现，不用手工救。

## 可复用的产物或模板
```sh
TASK_SOURCE=$(sh scripts/prepare-agent.sh)
git -C "$UPSTREAM" show "$BASE:module/module-agent/go.mod" > "$TASK_SOURCE/module/module-agent/go.mod"
git -C "$UPSTREAM" show "$BASE:module/module-agent/go.sum" > "$TASK_SOURCE/module/module-agent/go.sum"
cd "$TASK_SOURCE/module/module-agent" && \
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GO111MODULE=on GOFLAGS=-mod=mod \
  go build -buildvcs=false -trimpath -ldflags="-s -w" -o /tmp/agent .
```