---
name: luci-self-drawn-sticky-nav-under-theme-bar
description: 自建 LuCI 页面自绘的吸顶导航条滚走后不见时，按主题顶栏的 sticky 高度与 z-index 让位，而不是把 top 设 0
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [luci, css, sticky, 主题, 导航, 静默失败]
---
## 适用场景
自建 LuCI 插件在主题内自绘一排导航/工具栏，并希望它吸顶常驻；滚动后整条消失、或被页面上其它内容盖住时。

## 关键做法
部署后不要只看「设置了 position:sticky 就以为生效」，要读真实几何：量主题头部元素的 position、height、z-index，把自绘条的 `top` 设为头部高度、`z-index` 设得低于头部，让它贴在头部下沿；再滚动一段后读取自绘条内元素的实际 top 值确认它停在预期位置而不是 0。

## 踩过的坑
自绘条写 `top:0; z-index:14` 时，虽然 `getBoundingClientRect().top` 报 0、position 报 sticky、opacity 与 visibility 都正常，页面里却看不见——被主题自带的 sticky 顶栏（高度 57px、z-index 40）整块压住。只看属性值会误判为「已生效」，必须以滚动后的截图或内部子元素的 top 值作准。滚动后子元素 top ≈ 头部高度 + 内边距即为正确。

## 可复用的产物或模板
（无）