---
name: busybox-rpcd-sidecar-fifo-and-jshn-gotchas
description: 在 OpenWrt/busybox 上用 rpcd 驱动一个常驻外部进程（pty/FIFO 方式）给 ubus 供数据时，避开 FIFO 阻塞、start-stop-daemon 起不来、jshn 缺函数这几个静默坑
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [openwrt, rpcd, ubus, busybox, fifo, jshn, python3, 静默失败, 超时]
---
## 适用场景
设备上没有 dbus 客户端、也没有 python 的 dbus 绑定，需要让 ubus/rpcd 拿到一个常驻外部进程（如 `bluetoothctl` 的 pty 会话）产出的实时数据，并把命令下给它。

## 关键做法
- 后台协进程由 rpcd 按需拉起：脚本自带 shebang、chmod 755，直接 `"$SCRIPT" >/dev/null 2>&1 &` 后台跑，再 `sleep 1` 用 `pgrep -f '[脚]本名'` 确认拉起。不要依赖 start-stop-daemon。
- rpcd 向 FIFO 写命令必须用读写模式拿两端：`( exec 3<> "$FIFO" && printf '%s\n' "$cmd" >&3 )`，写完即关，这样没有读端时也不会阻塞。
- 协进程把数据落成「一设备一行」的 TSV（地址<TAB>名字<TAB>时间戳），shell 侧 `while IFS= read` 直接读，避免在 busybox 里拆 JSON；rpcd 按时间戳过滤过期条目。
- 加心跳文件（协进程每 2 秒覆写时间戳），rpcd 每次调用先比对心跳，超过约 5 秒没动就视为假活（进程在、内部子进程已死），杀掉清理目录后重起。

## 踩过的坑
- 对 FIFO 直接 `printf > fifo`：没有读端时 open 会阻塞，正好表现为 ubus `Request timed out`（30 秒），而且看不出是哪一步卡住。
- busybox 的 `start-stop-daemon` 不认 `-- python3 -- 脚本` 这种带参数形式，实测 rc=1 静默起不来；换直接 `&` 后正常。
- jshn 里没有 `json_add_number`（会报 `not found` 并把该字段从输出里整段丢掉），数字一律用 `json_add_int`；返回字段凭空消失时先去单独测 jshn 函数是否存在。
- 外部程序停止扫描时会把已知设备全标 `[DEL]`，照着删会让刚扫完的列表立刻变空；改为不删，靠时间戳新鲜度过滤。
- 解析外部交互式输出时别把 `[CHG]` 属性行（Class/UUIDs/RSSI）当名字，只有新设备行和 `Name:` 行可信。

## 可复用的产物或模板
（无）