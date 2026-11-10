---
name: modemmanager-misses-inbound-sms-native-stack-owns
description: 设备上 ModemManager 只列得出已发送短信、收到的短信一条都看不到时，判断收短信归原生栈并按其存储镜像
metadata:
    type: work_experience
    source: unknown
    created_at: "2026-11-07"
    tags:
        - modemmanager
        - sms
        - 短信接收
        - openwrt
        - at-cpms
        - 镜像
---
## 适用场景
要让上游程序（如网关 Agent）读到设备收到的短信，但 `mmcli -m any --messaging-list-sms` 只有自己发出去的记录，收到的短信一律看不到。
## 关键做法
先分清"没收到"和"读不到"：`AT+CPMS?` 报存储里有 N 条而 `AT+CMGL` 只回 OK 无 PDU，说明短信已到卡里但被别的进程消费了。查 `/usr/libexec` 与 `/etc` 下是否有原生短信服务（如 `e5-sms-receive`、`/etc/e5-sms/inbox.json`），有则以该原生存储为镜像源，取其中 `direction: in` 的记录，而不是 mmcli 列表。镜像时用状态文件记住已写过哪些 id，别用"池里有没有文件"判断，避免被确认删除的条目复活。
## 踩过的坑
按 ModemManager 做镜像源时列表永远是空、pool 一直 count=0，误以为没收到短信。另外池文件 `length` 若按字符数写，中文会被判无效，必须用 UTF-8 字节数。调试中 `ln -sf /dev/null /dev/null` 会把 `/dev/null` 变成自指软链导致 rc.common/procd 全线报 Symbolic link loop，须用 `rm -f /dev/null && mknod /dev/null c 1 3 && chmod 666 /dev/null` 恢复。
## 可复用的产物或模板
判断三步：`AT+CPMS?` 有数 → `AT+CMGL` 空 → 找 `/etc/*-sms/inbox*` 之类的原生存储，改读该文件。