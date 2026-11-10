---
name: airsim-bridge-pcm-localport-auth-handshake
description: 想往 AirSIM 本地音频桥（127.0.0.1:7580）注入测试音却握手失败时，判定是握手协议不对还是会话已被网关占用
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-07
 tags: [airsim, pcm, 音频桥, 握手, ubus, 本地端口, 测试注入]
---
## 适用场景
需要在 AirSIM 设备上往本地音频桥端口（默认 127.0.0.1:7580）写测试音或读下行，验证通话音频通路时。

## 关键做法
先分清这个端口的两类连接方：
1. 网关（daemon 自己）向桥发起连接时，握手是 `AIRSIMPCM1\n`，这是**仅供内部上游使用**的简单握手。
2. 局域网/外部客户端连接时，网关要求握手串为四段、以 `AIRSIMVWL1` 开头，并用配对密钥签名认证；格式不对或签名不过，连接会被直接拒绝，表现为“握手失败”或读不到 `AIRSIMREADY`。

所以自己写脚本注入测试音时，直接用 `AIRSIMPCM1` 往往握手失败。排查顺序：先用 `netstat -tnp | grep 7580` 看是否已有一条由 daemon 持有的 ESTABLISHED 连接——有就说明桥已被一个实时会话独占，第二路必然失败；此时应让该会话自然结束（或重启服务）再测，而不是反复改脚本。

## 踩过的坑
看到 `7580` 在 LISTEN 就直接连，报 “handshake failed” 后误以为是脚本写错或桥坏了，实际是端口已被 daemon 自己的网关会话占着，而且握手协议分两套。另外 `ps w` 里看不到 `aplay`/`arecord` 子进程时，不代表没在播——要先看 `/status` 的音频计数和 `last_error`。

## 可复用的产物或模板
```
netstat -tnp | grep 7580          # 看桥是否已被某会话独占
grep -rn "AIRSIMPCM1\|AIRSIMVWL1" # 在源码里确认两套握手的归属
```
（无）