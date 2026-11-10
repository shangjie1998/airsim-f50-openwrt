---
name: src-overlay-go-change-via-patch-layer
description: 在"上游源码 src/ + 个性化 overlay/"分层仓库里必须改上游 Go 源码时，怎么既保持 src/ 原样又把改动可靠编进二进制
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [src-overlay, overlay, patch, go, 交叉编译, 上游同步, 静默失败, 构建脚本]
---
## 适用场景
仓库分成"上游原样 `src/` + 只叠加个性化改动的 `overlay/`"两层，但某项功能（如号码格式规整、必须编译期生效的字段）无法在 overlay 壳层（rpcd 脚本、页面、uci）实现，只能改上游 Go 源码；同时又不想让 `src/` 变成"改过的副本"，以后跟上游合并时冲突成片。

## 关键做法
- 先判能不能不改源码：能在 overlay 壳层实现的一律走壳层（例如上游 daemon 没有某条 HTTP 路由，但状态文件就在盘上，就用 rpcd 脚本读/写它，页面对接壳层对象）。私有路由加进上游 `main.go` 这类改动，收益小、合并冲突长期存在，不划算。
- 确实必须改源码的，把改动做成补丁而不是改 `src/`：在 overlay 下建 `patches/` 目录，一个补丁一条改动，补丁头写成 `a/<相对 src 树根的路径>` / `b/…`，构建时按 `-p1` 打在"src 拷贝出来的构建树"上。
- 在叠加脚本里加补丁层，必须非交互且**打不上就失败退出**（`patch -p1 -N --batch -i <补丁>`，返回码非 0 直接 exit），否则上游改了上下文后会静默漏掉这条改动，编出来的包看着正常、功能其实没进去。
- 编完回读二进制确认改动真的编进去了（`strings <产物> | grep -c <新增函数名或新串>`），别只看"编译成功"。
- 语言/命名用中文注释写清"为什么这么改、上游对应补丁是哪个"，方便以后合并上游时对照。

## 踩过的坑
- 直接把改动写进 `src/` 最省事，但下一次同步上游时整文件冲突，而且 `src/` 与"上游原样"的等价性就丢了。
- 补丁层不加断言的话，上游改了被补丁文件的行数或上下文，`patch` 提示 `Reversed or previously applied` 或 fuzz 成功，构建照样过，改动实际没生效。
- 交互式 `patch` 在非交互环境会被 `Assume -R? [y]` 卡住，必须一开始就用 `-N --batch`。
- 只验证"编译通过"不够：改的是显示类字段（如号码格式），必须回到产物或运行中的接口上看最终值。

## 可复用的产物或模板
```python
# apply.py 里的一段：补丁必须干净打上
for pf in sorted((HERE / 'patches').glob('*.patch')):
    r = subprocess.run(['patch', '-p1', '-N', '--batch', '-i', str(pf)],
                       cwd=str(ROOT), capture_output=True, text=True)
    if r.returncode != 0:
        sys.stderr.write(f'补丁打不上: {pf.name}\n{r.stdout}{r.stderr}')
        sys.exit(1)
```
```sh
diff -u src/<树>/<文件> 改好的副本/<文件> \
  | sed -e '1s|.*|--- a/<树>/<文件>|' -e '2s|.*|+++ b/<树>/<文件>|' > overlay/<档>/patches/<名>.patch
strings <编出的二进制> | grep -c '<新增函数名>'
```