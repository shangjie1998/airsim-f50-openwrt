---
name: luci-theme-menu-cache-stale-tab
description: LuCI 里新增或改动菜单项后页签/导航仍不出现时，如何确认服务端菜单已生效并定位到前端主题缓存
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [luci, openwrt, menu.d, 主题缓存, sessionStorage, aurora, 静默失败]
---
## 适用场景
在 OpenWrt/LuCI 里新增或调整 `menu.d` 菜单项（新页面、新 tab），刷新浏览器后菜单或页签始终不出现，但页面本身能直接访问。

## 关键做法
先证明服务端已经认了新节点，再怀疑前端：请求 `/cgi-bin/luci/admin/menu`（返回整棵菜单树 JSON），确认新节点在里面且 `satisfied` 为真；再在设备上读 LuCI 生成的菜单缓存文件（`/tmp/luci-indexcache*.json`）查同一节点。两处都在、页面直连也能打开，说明不是菜单文件、acl 或 order 的问题，而是前端缓存。此时清空浏览器侧的 `sessionStorage` 与 `localStorage`（Aurora 等主题把菜单树存在 `sessionStorage` 的 `aurora.nav` 里反复复用），刷新后页签才会出现。验证方式是看页签数量/顺序是否与 `menu.d` 的 order 一致。

## 踩过的坑
一开始中招两次：先怀疑「新建独立 `menu.d/*.json` 文件不参与 tab 生成」，把节点并进主菜单文件，无效；又怀疑 acl/order，实际服务端早已正确。根因是主题把菜单树缓存在 `sessionStorage`（并按 `navStamp` 复用），服务器怎么改都不影响已打开的页面。只清 `/tmp/luci-*`（设备侧）和重启 `uhttpd` 不够，必须清浏览器侧 storage 或换新标签页。

## 可复用的产物或模板
（无）