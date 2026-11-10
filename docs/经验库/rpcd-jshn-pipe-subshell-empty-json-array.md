---
name: rpcd-jshn-pipe-subshell-empty-json-array
description: 用 jshn 在 shell 里组装 JSON 数组返回给 ubus 时，循环挂在管道后会静默返回空数组，需先把数据源落临时文件再重定向读取
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [openwrt, rpcd, ubus, jshn, shell, 静默失败, json]
---
## 适用场景
在 OpenWrt 上用 `/usr/libexec/rpcd/<name>` 这类纯 shell 脚本（jshn）读一个列表文件或 jsonfilter 结果，再把它们组装成 JSON 数组回给 ubus 时。

## 关键做法
- 组装走 `json_init` / `json_add_array` / 循环里 `json_add_object ... json_close_object` / `json_close_array` / `json_dump`。
- 循环的读取源必须用重定向接在 `done` 后面：`done < "$tmp"`；数据源先 `jsonfilter -e '@[*]' < src > "$tmp"` 落成临时文件。
- 循环里的计数器（序号、命中数）同样会随子 shell 丢失，需要计数就另用 awk 一次算完，别在 while 里自增。
- 对已知非空的源调用一次并回读元素数；返回 `[]` 而调用方无报错，基本就是这个坑。

## 踩过的坑
第一版写成 `jsonfilter -e '@[*]' < clients.json | while IFS= read -r key; do json_add_object ...; done`，函数照常返回、ubus 调用也报成功，但结果是 `{"clients":[]}`，一条都没有。jshn 的累积状态在管道后的子 shell 里，随子 shell 结束一起丢掉。改成先落临时文件、循环用 `done < "$tmp"` 后立刻返回 2 条。这个坑没有任何报错，只能靠对非空输入回读校验发现。

## 可复用的产物或模板
```sh
tmp=$(mktemp /tmp/rpcd-list.XXXXXX 2>/dev/null || echo /tmp/rpcd-list.$$)
jsonfilter -e '@[*]' < "$SRC" > "$tmp" 2>/dev/null
json_init
json_add_array items
idx=0
while IFS= read -r key; do
	[ -n "$key" ] || continue
	idx=$((idx + 1))
	json_add_object
	json_add_int index "$idx"
	json_add_string value "$key"
	json_close_object
done < "$tmp"
json_close_array
rm -f "$tmp"
json_dump
```