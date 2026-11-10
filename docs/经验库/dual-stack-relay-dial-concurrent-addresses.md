---
name: dual-stack-relay-dial-concurrent-addresses
description: 嵌入式设备连 CDN 后端自建中继时心跳/媒体建链报 DNS 或 TLS 超时、双栈某一侧实际不可达时，怎么改拨号逻辑避免串行等待
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [双栈, dns, 拨号, 超时, go, openwrt, 中继, 网络]
---
## 适用场景
设备（尤其 4G/5G 蜂窝出口）连 Cloudflare 这类 CDN 后的自建中继，出现心跳 TLS handshake timeout、WebSocket/媒体建链失败，错误里带 `lookup ...: i/o timeout`，且域名解析结果同时含 A 与 AAAA。

## 关键做法
1. 先分别实测两栈，不要凭 `nslookup` 输出或单次测量定顺序：`curl -6/-4 -o /dev/null -w "code=%{http_code} conn=%{time_connect} tls=%{time_appconnect}\n" https://<relay>/healthz`。实测常见结果是某一栈整条不通（code=000）、另一栈通但要 1~3 秒。
2. 拨号函数里改成：用系统解析器取全部 A/AAAA，对所有地址并发 `DialContext`，首个成功立刻取消其余；每个尝试单独给 4~8 秒预算。
3. 解析超时和连接超时分别设 context，不要共用同一个 deadline，否则解析慢了连接就没时间。
4. 自定义 DNS 服务器（1.1.1.1/8.8.8.8 之类）降级为并发之后仍失败的回退，不要放在首位。

## 踩过的坑
- 原实现先串行查多个自定义 IPv4 DNS（每个 4~5 秒），全超时再退系统解析，在"解析只回 IPv6、IPv6 不通、IPv4 握手 3 秒"的飘移链路上把预算一路吃满，媒体拨号必失败。
- 只把顺序倒成"先系统解析器"仍会踩：系统解析器常把 A 记录排在前面，照样卡在慢的 IPv4 一侧。
- 解析与连接共用同一个带超时的 context，看起来省事，实测会把连接阶段的可用时间挤掉。

## 可复用的产物或模板
```go
addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host) // 单独 6s 预算
raceCtx, cancel := context.WithCancel(ctx); defer cancel()
res := make(chan dialResult, len(addresses))
for _, a := range addresses {
    ip := a.IP
    go func() {
        actx, acancel := context.WithTimeout(raceCtx, 8*time.Second); defer acancel()
        c, e := dialer.DialContext(actx, network, net.JoinHostPort(ip.String(), port))
        res <- dialResult{c, e}
    }()
}
// 取首个 err==nil，成功后 cancel() 让其余立即放弃
```
限制：本轮只确认命令通道/心跳恢复，云端媒体建链尚未打通，因此该改法对媒体路径的效果仍需单独观察。