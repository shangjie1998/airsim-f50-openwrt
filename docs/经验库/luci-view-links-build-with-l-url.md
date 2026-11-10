---
name: luci-view-links-build-with-l-url
description: 自建 LuCI 插件页面里生成站内页面链接时，用 L.url 而不是 window.location.pathname 截前缀，避免落在组根页后菜单全部指向不存在的地址
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
tags: [luci, openwrt, javascript, 页面链接, 路由, 静默失败]
---
## 适用场景
在自建 LuCI 插件（`view.xxx` 模块）里生成站内跳转链接：自绘顶部菜单、面包屑、返回按钮、下拉页选择器。页面在具体页和"组根页"（`/admin/services/<插件名>`，不带最后的页名）两种地址下都会渲染。

## 关键做法
链接 base 交给 LuCI 自己算：`var base = L.url('admin/services/airsim');` 再拼 `base + '/' + pageId`。`L.url()` 知道 `/cgi-bin/luci` 这层脚本前缀和实际脚本名，两种地址下结果都一样。

## 踩过的坑
用 `window.location.pathname` 去掉最后一段来当 base：具体页 `/admin/services/airsim/overview` 上正确，但落到组根页 `/admin/services/airsim` 时截出的是 `/admin/services/`，于是所有菜单项变成 `/admin/services/bluetooth`、`/admin/services/sms`，页面报 `No page is registered at ...`。从侧栏进插件、或直接在组根地址刷新，就会触发，看起来像"插件没装好"。
确认做对的办法：读出各菜单项的 href，应同时包含脚本前缀和完整页名路径；再在组根页与某个具体页各点一次菜单项。

## 可复用的产物或模板
```js
var PAGE = L.url('admin/services/airsim');           // 组根地址，两种入口下都正确
E('a', { 'href': PAGE + '/' + id }, title);          // 具体页
```