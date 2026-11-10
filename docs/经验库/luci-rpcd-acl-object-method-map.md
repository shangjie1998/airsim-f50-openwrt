---
name: luci-rpcd-acl-object-method-map
description: 给 LuCI 加自建 ubus 对象的页面后报 -32002 Access denied 时，判定是 acl.d 里 ubus 段结构写错，以及怎样用真实 rpc 路径验证而不是被本地 ubus call 误导
metadata:
  type: work_experience
  source: harness
  created_at: 2026-11-07
  tags: [luci, rpcd, ubus, acl, access-denied, openwrt]
---
## 适用场景
在 OpenWrt 的 JS 版 LuCI（26.x，menu.d + rpcd）里给自建 ubus 对象加页面，浏览器上点开报 `-32002: Access denied`。

## 关键做法
1. `/usr/share/rpcd/acl.d/<app>.json` 里 `ubus` 段必须是 **对象 → 方法数组** 的映射，read/write 分开列方法：
   `"read": {"ubus": {"airsim": ["status","sms_list"]}}`、`"write": {"ubus": {"airsim": ["sms_send","service"]}}`。
2. 改完 `/etc/init.d/rpcd restart`。
3. 验证一定要走 LuCI 真实的 PRC 入口：POST `/cgi-bin/luci/admin/ubus`，body 为
   `{"jsonrpc":"2.0","method":"call","params":["<sessionid>","airsim","status",{}],"id":1}`，
   `result[0]` 为 0 才算 ACL 通过，为 5 是权限拒绝。sessionid 可用 `ubus call session login '{"username":...,"timeout":600}'` 取。
4. 改完 ACL 让用户**退出重新登录**（或刷新会话），旧会话仍带旧权限表。

## 踩过的坑
- 写成 `"ubus": ["airsim"]`（数组）不会在 rpcd 启动时报错，只在页面运行时 `-32002 Access denied`，静默失败，容易误判成后端脚本问题。
- 直接在设备上 `ubus call airsim status` 是走 root 全权限，**永远是通的**，完全暴露不了 ACL 问题；必须用带 session 的 HTTP rpc 路径验证。
- 页面 JS 若被 scp 传成 600，uhttpd 读不到会报 403，`chmod 644` 才正常；这条不影响 ACL，但容易和 ACL 问题混在一起排查。

## 可复用的产物或模板
ACL 模板：
```json
{ "luci-app-xxx": {
    "description": "...",
    "read":  { "ubus": { "myobj": [ "m1", "m2" ] } },
    "write": { "ubus": { "myobj": [ "m3", "m4" ] } } } }
```
验证命令：
```sh
SID=$(ubus call session login '{"username":"root","password":"<pw>","timeout":600}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["ubus_rpc_session"])')
curl -s -X POST http://<dev>/cgi-bin/luci/admin/ubus -H 'Content-Type: application/json' \
 -d '{"jsonrpc":"2.0","method":"call","params":["'$SID'","myobj","m1",{}],"id":1}' | head -c 120
```