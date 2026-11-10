---
name: luci-overlay-modal-style-outside-root-scope
description: 在样式以根类做作用域隔离的 LuCI 自建页面上加浮层弹窗时，怎么让浮层挂到 body 后仍能正确显示
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [luci, openwrt, css, 弹层, 样式作用域, 静默失败]
---
## 适用场景
在 LuCI 自建管理页上加弹出式浮层（列表确认、批量删除、二次确认），而该页的 CSS 把所有选择器都写成以某个根类为前缀（例如 `.as-root .as-button{…}`）来做作用域隔离时。

## 关键做法
- 浮层容器用 `E('div', {class:'…-mask'})` 建好后 `document.body.appendChild`，不要塞进页面里的某个卡片，避免被主题容器的定位、裁剪、浮动影响。
- 浮层样式要单独写一套**不带根类前缀**的选择器（`.as-modal{…}`、`.as-modal .as-button{…}`）。挂在 body 下的节点不满足根类前缀，复用页面样式会失配。
- 浮层里的 `h3`、`button` 等要显式重置主题默认值（`h3` 的 border/padding、`button` 的 box-shadow/text-shadow），否则跟着主题走会变形。
- 验证就看渲染结果：打开浮层后截一张全屏图，确认遮罩铺满窗口、按钮有底色、标题居中，而不是只看 DOM 里节点存在。

## 踩过的坑
- 这类页面本身已经在和 LuCI 主题打架——CSS 里专门写过把 header/footer 从主题的全局定位里隔离出来的规则，说明主题会干预这些标签；浮层同样会中招，不能默认"页面样式能用"。
- 样式失配是静默的：不报错、F12 里节点也在，只是显示成白板或被主题挤变形，容易误判成逻辑没跑通。

## 可复用的产物或模板
（无）