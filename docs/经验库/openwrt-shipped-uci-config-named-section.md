---
name: openwrt-shipped-uci-config-named-section
description: 自建 OpenWrt 安装包要预置一份 uci 默认配置，之后用 uci get file.section.key 读写时不生效/报 Invalid argument
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [openwrt, uci, 安装包, 配置文件, 静默失败]
---
## 适用场景
给 OpenWrt 自建包预置默认配置（install.sh 里 cp 一份 /etc/config/xxx），脚本或服务后续用 `uci -q get 文件.节名.选项` 读这份配置。

## 关键做法
配置文件里的节必须显式命名，写成 `config forward 'forward'`，不能写成匿名节 `config forward`。同理 `config 类型 '名字'` 里的名字要与脚本读取时用的节名完全一致；匿名节只能靠 `@类型[0]` 索引访问，`文件.类型.选项` 这种写法读不到，直接报 `uci: Invalid argument`。改完重新打包装机后，用 `uci -q get 文件.节名.选项` 读一次实际值来确认可寻址。

## 踩过的坑
匿名节时 `uci set`/`uci get` 全部返回 `uci: Invalid argument`，报错里不提节名问题，很容易误判成 uci 命令写错或配置没装进去。安装脚本里用 `[ ! -f /etc/config/xxx ]` 保护已有配置，测试时需先删掉旧配置再装，否则新默认值不会生效。

## 可复用的产物或模板
配置默认值：
```
config forward 'forward'
	option enabled '0'
	option url ''
	option method 'POST'
	option content_type 'application/json'
	option timeout '10'
```