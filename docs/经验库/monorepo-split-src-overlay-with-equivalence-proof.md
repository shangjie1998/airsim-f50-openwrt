---
name: monorepo-split-src-overlay-with-equivalence-proof
description: 把「直接改上游源码」的单体仓库迁成 src+overlay 分层时，怎么搬文件并用 diff 与重编二进制证明产物没变
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [git, 目录重构, src-overlay, 等价性验证, 交叉编译, shell]
---
## 适用场景
需要把「上游源码和自建改动混在一棵树里」的仓库改造成 `src/`（上游原版）+ `overlay/<机型>/`（自建改动）分层，且构建脚本不能依赖 git 历史（上游只发剥了 .git 的源码包）。
## 关键做法
先给迁移前工作树拍快照（`rsync -a --exclude .git --exclude out --exclude dist --exclude build ./ /tmp/premerge/`），后续所有对比都以它为准。上游原版用 `git archive <上游tag> | tar -x -C src` 导出（不要 checkout，避免混入未提交内容），再 `cp -a src/. upstream/r<NN>/` 留原件存档。自建改动用 `git diff --name-only <上游tag> -- . ':(exclude)dist'` 逐条复制到 overlay 对应路径。撤掉根上副本时用 `git rm --cached` 让 git 识别成 rename、保住历史。合成脚本只做两件事：`cp -R src/. OUT` 再 `cp -R overlay/. OUT`。两重验证缺一不可：一是 `diff -r -q /tmp/premerge <合成树>` 只看预期差异；二是从合成树重新交叉编译主二进制，sha256 与现役装机那个逐字节一致，才算结构换了产物没变。
## 踩过的坑
`git diff --name-only` 不列 untracked 和空目录，旧树里靠空目录保证安装时目录存在的路径（如 `m8-pkg/luci/usr/lib/airsim`）会静默丢失，只有 `diff -r` 才暴露，需在 overlay 手工补同名空目录。shell 里 `echo "…$OUT（…）"` 这类中文全角括号紧贴变量名会被当成变量名一部分，配合 `set -u` 直接报 unbound variable 并中断脚本，变量一律写成 `${OUT}`。
## 可复用的产物或模板
```sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT=${1:-$ROOT/build}
rm -rf "$OUT"; mkdir -p "$OUT"
cp -R "$ROOT/src/." "$OUT/"
cp -R "$ROOT/overlay/m8/." "$OUT/"
echo "合并完成：${OUT}（VERSION=$(cat "$OUT/VERSION")）"
```