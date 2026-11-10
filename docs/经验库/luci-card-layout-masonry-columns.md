---
name: luci-card-layout-masonry-columns
description: 自建 LuCI 页面用卡片布局、同排卡片高低不齐导致大片空白或内容被硬撑时，改用 CSS 多列瀑布流并处理整行卡与单卡页
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [luci, css, 布局, 响应式, columns, 瀑布流]
---
## 适用场景
自建 Web 页面（尤其 LuCI 插件页）用卡片承载内容，卡片内容长短不一。要求卡片按浏览器宽度排成 1/2/3 栏并铺满页面时，用 grid 会出现同排卡片被拉到最高卡的高度、旁边空出一大块；内容少的卡被硬撑，看着又空又乱。

## 关键做法
1. 布局容器改用多列瀑布流：`columns:<卡片最小宽>; column-gap:<间距>`。列数由浏览器按可用宽度自动决定，宽度变则栏数变，天然铺满。
2. 每个子卡片必须 `break-inside:avoid; width:100%`，否则卡片会被从中间断开、跨到下一列。卡片间距用子元素自带 `margin-bottom:<间距>` 造（多列布局不认 `gap`），并给最后一个子元素清零。
3. 需要占满整行的卡（说明、整块表单）标 `column-span:all`。
4. 整页只有一张卡时，多列会把它压在左半栏；加 `:only-child{column-span:all}` 让它占满。
5. 卡片最小宽决定在什么视口下出几栏，要按目标视口实测调：设 360 在 1050 宽仍是 1 栏，调到 300 才稳定两栏。
6. 窄屏补一条 `@media(max-width:720px){columns:1}`，避免手机上出现窄列。

## 踩过的坑
- 只把 `grid-template-columns` 调成 `auto-fit/minmax` 解决不了「同排拉平留空」，因为 grid 的行对齐机制本来就会这样；必须换成多列。
- 多列布局里 `gap` 不生效，只能靠子元素 margin；漏了就卡片贴在一起。
- 漏 `break-inside:avoid` 时卡片会被劈开跨列，表现为半张卡在上一列底部、半张在下一列顶部。
- 窄的卡看着「空」不一定是布局错，是内容真的少；先把内容拆到合理粒度（一卡一件事）再谈排版。

## 可复用的产物或模板
```css
.as-layout{columns:var(--as-card-min);column-gap:var(--as-gap)}
.as-layout>*{break-inside:avoid;margin:0 0 var(--as-gap);width:100%}
.as-layout>*:last-child{margin-bottom:0}
.as-wide{column-span:all}
.as-layout>:only-child{column-span:all}
@media(max-width:720px){.as-layout{columns:1}}
```