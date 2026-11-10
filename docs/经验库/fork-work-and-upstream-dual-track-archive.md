---
name: fork-work-and-upstream-dual-track-archive
description: 维护他人开源项目的自建 fork、且改动部分不进 git（被 gitignore）时，怎么建立能真正恢复的本地归档
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [git, git-bundle, fork, 归档, 备份, gitignore, 上游, 嵌入式]
---
## 适用场景
基于别人的开源仓库做长期二次开发，担心上游仓库被删或失联；同时自己的关键改动不落在 git 里（补丁式工作副本、`out/` 被 ignore、编译产物源码树在仓库外）。

## 关键做法
双轨归档，缺一轨都恢复不全：

1. **上游仓库冷备**：在仓库里执行 `git bundle create <目标>/<项目>-all-<日期>.bundle --all`，再用 `git bundle verify <bundle>` 确认输出包含 “records a complete history”。`--all` 会同时带上所有 remote 的分支与标签。文件名带日期，旧的保留不覆盖。恢复：`git clone <bundle> <新目录>`，标签和提交都在。
2. **仓库外改动快照**：在项目内存 `<项目>/saves/<日期>-<主题>/`，放 `code/`（改动过的文件原文）、`diff-vs-<基线>.patch`（用 `diff -u` 对基线副本生成）、`README.md`（改了什么、为什么、以后什么条件下该重估）。
3. **README 里写清恢复命令和维护规矩**，让下一次的自己照着做，而不是靠记忆。

判断该不该双轨：先查 `.gitignore` 是否排除了改动所在目录（`grep -n out .gitignore`），排除了就必须有第 2 轨，否则“备份了仓库”是假安全。

## 踩过的坑
- 只存 `git bundle` 看着完整，实际漏掉最关键的改动：本次仓库 `.gitignore` 第 1 行是 `/out/`，agent 源码整体不在版本控制里，bundle 里根本没有它们。
- 只存改动文件、不存上游基线，等于改动成了孤儿：上游一删，patch 上下文对不上，想重放也不知道基线在哪。
- bundle 不做 verify 就存，可能拿到残缺包，事后才发现恢复不出来。
- 「临时想留个底」的一次性拷贝不写日期、不写恢复方法，几个月后自己都看不懂哪个是最新的。

## 可复用的产物或模板
```
# 生成 + 校验（文件名带日期，旧的不要覆盖）
git -C <repo> bundle create <dest>/<proj>-all-$(date +%F).bundle --all
git -C <repo> bundle verify <dest>/<proj>-all-$(date +%F).bundle

# 仓库外的改动生成 diff（对基线副本）
diff -u <基线副本>/<file> <当前>/<file> >> saves/<日期>-<主题>/diff-vs-<基线>.patch
```