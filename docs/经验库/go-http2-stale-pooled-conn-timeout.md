---
name: go-http2-stale-pooled-conn-timeout
description: 嵌入式设备上的 Go 客户端往云端 POST 卡在 http2 等响应头超时或 malformed HTTP response 时，判定是 HTTP/2 残留并完整改走 HTTP/1.1
metadata:
    type: work_experience
    source: harness
    created_at: "2026-11-08"
    tags:
        - go
        - http2
        - alpn
        - 连接池
        - 超时
        - 嵌入式
        - 推送中继
---
## 适用场景
嵌入式/弱网设备上用 Go 的 http.Client 周期性往远端 POST（推送中继、心跳、设备注册），报 `http2: timeout awaiting response headers`，或改成 HTTP/1.1 后报 `malformed HTTP response` 且内容是 h2 帧字节。

## 关键做法
1. 不要先怀疑超时值太小。等不到响应头说明请求发出后连接已经死了，不是等得不够久。
2. 想让这条链路走 HTTP/1.1，三样必须同时设：`ForceAttemptHTTP2 = false`、`TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}`、以及 TLS 层的 ALPN —— 从 `TLSClientConfig` 克隆后把 `NextProtos` 设成 `[]string{"http/1.1"}`，原有 config 不为空时先 Clone 再改。
3. 报错会逐层退化，把报错变化当诊断阶梯用：`http2: timeout awaiting response headers` → `TLS handshake timeout` 或 `malformed HTTP response "\\x00\\x00\\x12\\x04..."` → 正常。中间那个 `\\x00\\x00\\x12\\x04` 是 h2 SETTINGS 帧，说明 ALPN 还在协商 h2、服务端按 h2 回帧而我们按 HTTP/1.1 解析。
4. 去掉 h2 之后要重新量这条链路本身的握手耗时（分 IPv4/IPv6 各测），再按实测最坏值定 `TLSHandshakeTimeout`、`ResponseHeaderTimeout` 和整体 client timeout，不要沿用默认值。

## 踩过的坑
- 只改 `ResponseHeaderTimeout` 无效。
- **只设 `ForceAttemptHTTP2 = false` + 清空 `TLSNextProto` 不够**：ALPN 仍会协商 h2，服务端照 h2 回帧，客户端报 `malformed HTTP response` 并带一段 `\\x00\\x00\\x12\\x04` 字节。必须再钉 `TLSClientConfig.NextProtos`。
- 强制 HTTP/1.1 只解决"卡住干等"，链路本身握手慢或丢包仍会超时；两件事要分开改、分开验。
- 同一台设备上 IPv4 与 IPv6 到同一主机的 TLS 握手耗时可差一个数量级（实测 IPv4 5～8 秒、IPv6 0.68 秒）。只测一种地址族就定超时值会误判。

## 可复用的产物或模板
（无）