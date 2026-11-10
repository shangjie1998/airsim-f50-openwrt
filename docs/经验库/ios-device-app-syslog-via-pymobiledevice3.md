---
name: ios-device-app-syslog-via-pymobiledevice3
description: 需要在 Mac 上抓取已连接 iPhone 真机某个 App 的系统日志时，绕开 macOS log stream 不支持真机的限制
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-07
 tags: [ios, syslog, pymobiledevice3, 真机日志, macos, 诊断]
---
## 适用场景
要从 Mac 上实时抓取通过 USB 连接的 iPhone 真机上某个 App 输出的 print/NSLog 日志，用来定位只写进设备控制台的运行期行为。

## 关键做法
1. 先 `xcrun devicectl list devices` 或 `pymobiledevice3 usbmux list` 拿 UDID，确认设备已被识别为 USB 连接。
2. macOS 自带 `log stream` 只能读本机日志，传 `--device <UDID>` 会直接报 unrecognized option，不要在这条路上继续试。
3. 当前 Python 可能禁用 `pip --user`，改用虚拟环境：`python3 -m venv /tmp/ioslog && /tmp/ioslog/bin/pip install pymobiledevice3`。
4. 后台流式抓取并落盘：`/tmp/ioslog/bin/python -m pymobiledevice3 syslog live -pn <进程名> -o <输出文件>`，进程名用 App 的可执行名而非 bundle id。
5. 确认抓取生效：启动几秒后看输出文件是否在增长（正常几秒内就有上百 KB），再让用户去复现操作。

## 踩过的坑
- `log stream --device` 不支持外接 iOS 设备，会退化成 usage 提示，容易误判成设备没连上。
- `pip install --user` 在沙箱化 Python 中会报 "User site-packages are disabled"，必须走 venv。
- 只按 bundle id 过滤可能匹配不到，用 `-pn` 传可执行名（如 AirSIM），未知时先不加过滤抓几秒看进程名。
- 需用户配合点亮 App 复现，日志里只有 print 级内容，别指望拿到未打日志的分支状态。

## 可复用的产物或模板
`python3 -m venv /tmp/ioslog && /tmp/ioslog/bin/pip install pymobiledevice3`
`nohup /tmp/ioslog/bin/python -m pymobiledevice3 syslog live -pn AirSIM -o /tmp/ios.log >/dev/null 2>&1 &`
`/tmp/ioslog/bin/python -m pymobiledevice3 usbmux list`