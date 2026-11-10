---
name: upstream-patch-delta-into-patched-tree
description: 维护"上游源码+自家累积补丁"的工作副本时，只把上游新版里某几个文件的改动摘过来，直接打补丁大量冲突，怎么还原基线再干净落地
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [补丁移植, patch, git, 上游同步, 冲突, 嵌入式]
---
## 适用场景
工作目录不是上游原始源码，而是"固定上游 commit + 一份自定义累积补丁"的输出（本地被 patch 改过），此时上游发新版，只想摘其中若干文件的改动，不做整体合并。

## 关键做法
1. 先取两版补丁正文比对：`git show <旧commit>:patches/X.patch > /tmp/old.diff`、`git show <新commit>:patches/X.patch > /tmp/new.diff`，按 `diff --git` 切段，逐文件统计行数变化，得出"新增文件 / 被改文件"清单，只挑目标文件的段。
2. 把这些段单独拼成一个最小补丁文件。
3. 工作副本既然是旧补丁的输出，直接用 `patch -pN` 打新段会大面积冲突。先对同一文件反向打旧段 `patch -pN -R < 旧段`，把它还原成上游基线（先用副本操作，别动正式目录）。
4. 再正向打新段，`patch` 报告逐文件 patched、新增文件生成，即为干净落地。
5. `-pN` 的 N 按补丁里的路径前缀与实际所在目录层级对齐。

## 踩过的坑
- 直接正向打新段时，被改文件的多数 hunk 全挂（本例 22 个 hunk 挂 14 个）；报错只提示 "N out of M hunks failed"，容易误判成补丁坏了。
- 判断"反向能还原"的证据：先 `patch -R --dry-run` 确认无冲突，说明本地文件确实等于旧补丁的成品，才可放心反打。
- 只反打目标文件的旧段，不要整份旧补丁全反，否则会顺带抹掉本地其他改动。
- 反打与正打之间不要插入其他修改，避免基线再次偏移。

## 可复用的产物或模板
```
git show <old>:patches/X.patch > /tmp/old.diff   # 旧补丁全文
git show <new>:patches/X.patch > /tmp/new.diff   # 新补丁全文
# 按 diff --git 切段，按行数差异找出 新增/改动 文件
patch -pN --dry-run < /tmp/new.diff              # 先看冲突量
patch -pN -R < /tmp/old-file.diff                # 只反打目标文件的旧段
patch -pN    < /tmp/new-file.diff                # 再打新段
```