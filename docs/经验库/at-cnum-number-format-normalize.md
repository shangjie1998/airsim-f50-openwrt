---
name: at-cnum-number-format-normalize
description: 多卡设备从 AT+CNUM 取本机号码时，不同运营商返回格式不一（带 +、带 86、裸 11 位），只按单一前缀解析会让某张卡静默显示错误号码
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [at命令, cnum, 本机号码, 双卡, 号码格式化, 静默失败, openwrt]
---
## 适用场景
双卡或多运营商设备上，从 `AT+CNUM` 或类似 modem 自报字段取本机号码，用于网页展示、短信/通话推送或 API 输出，出现"一张卡带国家码、另一张不带"或被原样吐出。

## 关键做法
`AT+CNUM` 的号码格式随卡和运营商变化：有的回 `+8613…`，有的回 `8613…`，也有回裸的 11 位本地号。不要按单一前缀分支解析，统一走一个归一化函数：
- `+` 开头：原样返回
- `86` 开头且长度 ≥12：补 `+`
- 裸的 11 位、`1` 开头：补 `+86`
- 其它（`10010` 这类短号、非中国号段）：不动

每张卡取到号码后立刻过一遍，并且让号码源头（生成 info/状态的采集脚本）、API 出口和展示层都调用同一个函数，避免修一处漏一处。shell 侧用 case 实现，Python 侧再兜一层，防止读到旧缓存文件里的旧格式。

## 踩过的坑
原实现只写了 `case 86*` 一个分支。卡一返回 `<设备号码>` 恰好命中，显示正常；卡二返回裸 `19282491150` 未命中，原样输出。页面不报错，只是"少了个国家码"，属于静默失败——只有把两张卡的号码并排比对才能发现。
教训：不要在展示层补，号码源头和消费端要一起改；只改页面的话，换到短信或推送渠道又会漏。

## 可复用的产物或模板
```sh
intl_number() {
	case "${1:-}" in
	'') printf '' ;;
	+*) printf '%s' "$1" ;;
	86*) [ "${#1}" -ge 12 ] && printf '+%s' "$1" || printf '%s' "$1" ;;
	1*)  [ "${#1}" -eq 11 ] && printf '+86%s' "$1" || printf '%s' "$1" ;;
	*)   printf '%s' "$1" ;;
	esac
}
```
```python
def intl_number(value):
    value = (value or "").strip()
    if not value:
        return ""
    if value.startswith("+"):
        return value
    if value.startswith("86") and len(value) >= 12:
        return "+" + value
    if value.startswith("1") and len(value) == 11:
        return "+86" + value
    return value
```