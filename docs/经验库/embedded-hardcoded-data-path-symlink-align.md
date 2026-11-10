---
name: embedded-hardcoded-data-path-symlink-align
description: 设备上某个上游程序把数据源目录写死、而厂商服务实际存在别的路径导致接口恒报错时，用软链对齐路径免重编恢复，并把软链固化成开机脚本
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [嵌入式, openwrt, 路径写死, 软链接, init脚本, 静默失败, 免重编]
---
## 适用场景
设备接口（HTTP/ubus）恒定返回某类“数据不可用”错误，但底层厂商服务明明在正常写数据时，先怀疑是路径/目录契约不一致，而不是数据缺失或服务没起。

## 关键做法
1. 从接口错误串反查代码里写死的路径（如 agent 写死 `pool: "/etc/xxx/pool/msg"`）。
2. 到设备上核对这条路径是否真实存在，再找厂商脚本里路径是怎么算出来的（厂商常把 uci 配置归一化成自己目录，与上游假设不同）。
3. 若两边只是目录层错位，用绝对软链把写死路径指到真实目录；后台进程按周期扫描时会自愈，无需重启服务，也无需重编二进制。
4. 把软链做成开机脚本（`#!/bin/sh /etc/rc.common`，`start()` 里幂等 `ln -s`），并加进装机脚本的 enable/start 段，避免重刷后丢失。变量紧贴中文标点时注意别并进变量名。

## 踩过的坑
- 只信接口报错“不可用”会误判成数据丢失或服务未就绪，实际是路径对不上，日志里厂商仍在正常落盘。
- 目录不存在时软链目标要用相对名（`ln -s sms /etc/xxx`），用绝对路径在挂载或重刷后容易失效。
- 脚本首行写成 `#!/sh /etc/rc.common` 会导致无法正常执行，必须 `#!/bin/sh /etc/rc.common`。

## 可复用的产物或模板
```sh
#!/bin/sh /etc/rc.common
START=97
start() {
	[ -d /etc/vendor/data ] || return 0
	[ -e /etc/upstream/pool ] && return 0
	ln -s ../vendor/data /etc/upstream/pool 2>/dev/null || true
}
```