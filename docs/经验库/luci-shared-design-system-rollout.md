---
name: luci-shared-design-system-rollout
description: 自建 LuCI 插件多页面各用自家 CSS、要引入一套共享设计系统并逐页替换时，怎么铺开而不丢样式、怎么在没有视口控制的环境下验证手机档
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [luci, openwrt, css, 设计系统, 响应式, 浏览器验证, browser-use]
---
## 适用场景
自建 LuCI 插件有若干独立页面，每个页面自带一份 CSS，同一组件在不同页长得不一样；要抽出一份共享的 token + 组件样式，并逐批把页面切过去。

## 关键做法
1. 共享层拆两件事：一份 CSS 只放 `:root` / `html[data-theme="dark"]` 两套 token 加组件块，页面文件里不写颜色、间距、圆角；一份 JS 做外观状态机（三态 + 密度 + 透明度写 localStorage、`auto` 监听 `prefers-color-scheme`），页面只在 render 开头调 `install()`、样式链接换成共享表一行，业务与 rpc 逻辑一行不动——这样单页验证时不至于把功能回归和样式回归混在一起。
2. 切页面前先机械核对：把该页 JS 里出现的 `前缀-xxx` 类名与共享 CSS 里已定义的选择器取差集，遗漏的类补进共享表。不核对就会静默丢样式：页面能打开、不报错，只是块变平。
3. 改完在真机上验证三件事：深浅切换即时生效且刷新仍记住、页面数据与改前一致、窄屏无横向溢出。

## 踩过的坑
- 差集脚本会把形如 `as-sms-filter` 的 **id** 也算成 class，出现大片虚假缺失；要用「是否作为 `class:` 值出现」过滤，或逐个回看上下文再补。
- 想用 `Emulation.setDeviceMetricsOverride` 把浏览器调到 390px 验证手机档时，报 `CDP protocol error -32001: Session with given id not found`，`resync()` 后依然报；`window.resizeTo` 也不生效。改用同源 iframe：在页面上插一个 `width:390px;height:844px` 的 iframe 指向同页，读 `contentDocument.documentElement` 的 `scrollWidth/clientWidth` 判断是否溢出，同时截图看实际排版。
- 新增页面文件要确认安装脚本的拷贝列表里有它，漏一个就是页面上 404 或白屏，而且这种错在设备上看不到明显日志。
- 改完 `menu.d`/新增视图后菜单文字仍是旧的，别只清 `/tmp/luci-indexcache*`：先 `touch` 一遍新拷过去的 `menu.d/*.json` 和视图文件（scp 会保留旧 mtime，LuCI 的 indexcache 按 mtime 判断失效，旧时间戳等于缓存永远有效），再清 `/tmp/luci-indexcache*`；若设备用的是 aurora 之类自带 router 的主题，导航还被缓存在浏览器 `sessionStorage` 的 `aurora.nav`（以及 `luci-session-store`）里，`sessionStorage.clear()` 之后刷新才换成新菜单。
- 新样式页首次打开会停在「正在载入视图…」十几秒（弱性能设备 + 刚清缓存时更明显），刷新一次即正常；判断是否真卡死要看 `scripts`/网络请求是否 200，别急着改代码。

## 可复用的产物或模板
差集核对（把前缀换成自己的组件前缀）：
```python
import re
need=set(); [need.update(re.findall(r'class[\'"]?\s*[:=]\s*[\'"]([^\"\']*[\'"]', open(f).read())) for f in pages]
```
（实际按项目写法取 `class:` 值里切出的类名，再与 CSS 的 `.类名` 集合相减。）
同源 iframe 测窄屏：
```js
var f=document.createElement('iframe');
f.style.cssText='position:fixed;left:0;top:0;width:390px;height:844px;border:0;z-index:999999';
f.src=location.href; document.body.appendChild(f);
// 之后读 f.contentDocument.documentElement.scrollWidth / clientWidth
```- 改了共享 CSS 却没生效、`document.styleSheets` 里只有主题那份：`L.resource('view/x/ui.css')` 拼的是固件版本号，固件不换 URL 就不换，浏览器一直吃旧副本。解法是在共享 JS 里放一个自管版本常量，`stylesheet()` 返回的 href 末尾拼 `?v=<常量>`，并且**以后每次改 CSS 都同步改这个常量**（本项目 `ui.js` 的 `CSSVERSION`）。
- 想给视图加一行 `console.log` 探针排查时，别插在文件开头：LuCI 的 `'require xxx'` 声明必须是从第一行起的连续语句，前面插任何代码都会让加载器不再注入依赖，表现为 `ReferenceError: rpc is not defined` 并卡在「正在载入视图…」。探针要加就加在 require 段之后。
- 改了 `acl.d` 后 `/etc/init.d/rpcd restart` 会让 LuCI 登录态掉线（需用户手动重登）；偶尔这一次重启后 ubus 调用会永久挂住——页面停在「正在载入视图…」、`network_requests()` 里那条 `/cgi-bin/luci/admin/ubus` 一直 inflight、控制台无报错，设备侧 `ubus call` 却秒回。再 `restart` 一次 rpcd 并重登即恢复，别去改页面代码。
