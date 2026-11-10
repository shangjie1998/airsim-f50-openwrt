---
name: shell-var-adjacent-cjk-punct
description: 写中文提示的 shell 脚本里变量紧挨全角标点时会被并进变量名、报出莫名其妙 unbound variable 错误，怎么改
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [shell, sh, 中文提示, 变量展开, 构建脚本, 静默报错]
---
## 适用场景
在 sh/bash 脚本里 output 中文提示、且提示串中变量后面紧跟全角标点（`（`、`）`、`：`、`，`）的时候。

## 关键做法
变量一律写成 `${VAR}` 再紧跟中文标点；或者把中文和变量拆开、用两次输出。改完用 `sh -n` 查语法、单跑一次看输出是否带中文括号正常。

## 踩过的坑
`echo "合并完成：$OUT（VERSION=…）"` 报 `OUT？: unbound variable`，`echo "  $V（两处一致）"` 报 `V？: unbound variable`：报错里那个变量名带着中文标点，正是扩展时全角括号被当成了变量名的一部分。这种报错指向一个脚本里根本没定义的变量，很容易误以为是真的少定义，反复往上翻。同一项目里两次踩到，说明不是偶发。

## 可复用的产物或模板
```sh
echo "  ${V}（两处一致）"
echo "合并完成：${OUT}（VERSION=${V}）"
```