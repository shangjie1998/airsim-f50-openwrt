---
name: overlay-rename-rules-with-miss-assertion
description: 上游源码树保持原样、个性化改动做成 overlay 时，文案改名用规则表加"任一条未命中即报错"的断言，避免上游改措辞后静默漏改
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [上游同步, overlay, 字符串替换, 静默失败, 交叉编译, 嵌入式]
---
## 适用场景
维护"上游源码原样 + 自己叠加改动"的仓库，且改动中有一类是**换显示文案**（品牌改名、机型名替换），需要在上游发新版时反复重放这套改动。

## 关键做法
1. 把改名做成一张规则表（每行 `旧串<TAB>新串`，注释行以 `#` 开头），与整文件替换的 `files/` 目录并列放在 overlay 里。
2. 应用脚本只扫目标子树（如 `plugin/**/*.go`）里非 vendor、非 `_test.go` 的文件，逐条统计命中次数。
3. **每条规则命中 0 次就整体报错退出**，并列出未命中的规则名；编出来的二进制再做一次反向检查（`strings` 里应出现新名、不应出现旧名）。
4. 构建脚本先 `rsync` 上游树到临时构建目录再叠 overlay，不直接改 `src/`；上游构建脚本若依赖 git 历史（`git archive <钉住的提交>`），换成直接编译的等价命令。

## 踩过的坑
- 上游只发剥掉 `.git` 的 sources 包，`git cat-file -e <base>^{commit}` 直接 `fatal: Not a valid object name`，构建链在第一步就断；不要试图补建 git 历史，改成手动同参数编译更快。
- 上游补丁若已合进手上的树，再 `patch -p1` 会满屏 `Reversed (or previously applied)` 并反复提示 `Assume -R? [y]`；此时跳过打补丁步骤即可。
- 纯字符串替换最容易的失败是**静默漏改**：上游把文案措辞改了一两个字，替换规则一条都不命中，编出来的包看着正常但界面还是旧名。用"命中计数为零即失败"把这种失败变成显式报错。

## 可复用的产物或模板
```python
hits = {old: 0 for old, _ in pairs}
for f in targets:
    t = f.read_text(encoding='utf-8'); o = t
    for old, new in pairs:
        c = t.count(old)
        if c: hits[old] += c; t = t.replace(old, new)
    if t != o: f.write_text(t, encoding='utf-8')
missing = [old for old, c in hits.items() if c == 0]
if missing: print('未命中:', missing, file=sys.stderr); sys.exit(1)
```