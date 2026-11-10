---
name: luci-view-e-nested-array-render
description: 自建 LuCI 页面把一个 E() 的 children 数组构造成含子数组的嵌套结构时，页面会静默渲染成 "[object HTMLDivElement]" 文本而不报错，需先摊平成一层再传入
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [luci, openwrt, javascript, E, 渲染, 静默失败, 页面拆卡]
---
## 适用场景
在自建 LuCI 视图里用 `E(tag, attrs, children)` 搭页面，并且 children 是由变量、函数返回值或条件分支拼出来的（典型是把原先一张大卡拆成多张卡、再交给布局容器）时使用。

## 关键做法
先得到一个只含 HTML 元素的一维数组，再作为 children 传入：用 `var cards = []; cards.push(E(...)); cards.push(E(...));` 逐条推入，最后 `E('div', { 'class': 'as-layout' }, cards)`。子卡内部的段落也照样用 `.concat(子数组)` 或展开展开成一层，不要让数组套数组落在 children 里。

## 踩过的坑
拆卡时把 `return [ E(...), E(...) ]` 这样的数组当作单个条目塞进 layout 的 children，页面不抛异常、控制台无任何报错，但那一段位置直接显示「[object HTMLDivElement],[object HTMLDivElement]」——因为嵌套数组被当成文本节点字符串化了。排查时不能只信控制台干净，要么读截图、要么读页面文本/DOM 确认实际渲染内容；`node --check` 也只能验语法，验不出这个问题。

## 可复用的产物或模板
```js
var cards = [];
cards.push(E('section', { 'class': 'as-card as-wide' }, [E('h3', {}, '标题'), E('p', { 'class': 'as-caption' }, '说明')));
cards.push(E('section', { 'class': 'as-card' }, [E('h3', {}, '另一张卡')].concat(subItems)));
return E('div', { 'class': 'as-root' }, [ ui.stylesheet(), E('div', { 'class': 'as-layout' }, cards) ]);
```