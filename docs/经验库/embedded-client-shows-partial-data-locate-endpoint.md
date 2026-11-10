---
name: embedded-client-shows-partial-data-locate-endpoint
description: 嵌入式设备的客户端只显示一部分数据（如双卡只显示一个号），往自建接口补字段后客户端仍无变化，如何定位到客户端真正读取的是哪个端口和哪条接口
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [嵌入式, 客户端联调, 接口发现, 请求记录, netstat, 静默失败, openwrt]
---
## 适用场景
自建常驻服务（agent/daemon）被手机 App 通过本地网关或代理访问，客户端展示的数据比设备实际拥有的少。尤其当设备内部有多个对外端口、多个 HTTP 服务、网关有路径白名单时，"猜字段名"和"往一个接口加字段"往往无效。

## 关键做法
1. 不猜字段名。先在服务端加一段临时请求记录中间件，把 `Method + Path` 与 UA 去重计数，定时落到一个临时文件（如 `/tmp/xxx-req.log`），装到设备上重启服务。
2. 让用户打开客户端停十几秒，回读该文件，先拿到客户端真实请求的路径集合。
3. 若记录的 UA 全是内部代理（如 `Go-http-client`），说明请求是被转发进来的，再看 `netstat -an` 里客户端 IP 连的是哪个端口，据此判断请求落在哪个 HTTP 服务。
4. 到该服务的注册代码/网关白名单里核对路径，找到"客户端取该类数据的唯一接口"，就地补字段；补完直接对该内部端口 `wget` 回读，确认字段真在响应里。
5. 验证完成后摘除临时请求记录器。

## 踩过的坑
- 在 `/api/status` 加字段无效：客户端读的是另一个专取该数据的接口，`/api/status` 只是整包状态，客户端可能只取其中一部分或干脆不读。
- 只看请求日志还不够：记录的 UA 全是本地代理，无法区分是哪个客户端；必须结合 `netstat` 的客户端 IP 与端口，才能确认它落在哪个服务上。
- 设备内部常有多个 HTTP 服务（管理面、控制面、网关），同一功能可能只在网关白名单里放行某几条路径，忘记白名单会以为接口不存在。
- 启动时后台刷新那次 AT/IO 调用可能被其他循环抢走，导致字段一直为空；可在读取路径上做一次一次性兜底补采。

## 可复用的产物或模板
请求记录中间件（Go，挂在 handler 外层）：

```go
func (a *agent) logRequests(next http.Handler) http.Handler {
	reqLogOnce.Do(func() { go reqLogFlushLoop() })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqLogMu.Lock()
		key := r.Method + " " + r.URL.Path
		stat := reqLogSeen[key]
		if stat == nil {
			ua := r.UserAgent()
			if len(ua) > 70 { ua = ua[:70] }
			stat = &reqLogStat{UA: ua}
			reqLogSeen[key] = stat
		}
		stat.Count++
		reqLogMu.Unlock()
		next.ServeHTTP(w, r)
	})
}
```

定位端口：`netstat -an | grep ESTABLISHED | grep <客户端IP>`，对照服务监听端口。