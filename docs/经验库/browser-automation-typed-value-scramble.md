---
name: browser-automation-typed-value-scramble
description: 浏览器自动化往输入框写长文本后，回读内容出现字符乱序或中英文交错时，如何用原生 value setter 修正并确认真的写对了
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [browser-use, 表单输入, 字符乱序, 静默失败, 发布]
---
## 适用场景
用浏览器自动化在网页表单里填标题、正文等长文本，尤其是含中文或标点的字段；填写后页面看似正常，实际内容可能已被打乱。

## 关键做法
1. 写完立刻回读输入框真实值（读取控件文本或 DOM 的 value），不要只看"已填入"就继续下一步。
2. 发现乱序（如 `AirS IM fan zhuang bao.1-r22-m801.0.` 这类词序颠倒、空格错位），不要反复重打，改用 JS 直接设值：取该元素的 `value` 属性原生 setter，调用后依次派发 `input` 和 `change` 事件（`bubbles: true`），让框架的状态同步更新。
3. 再回读一次确认值正确，然后再提交表单。

## 踩过的坑
一次性把长文本塞进输入框时，写入顺序会被打乱，中英文和数字段落错位；肉眼扫一眼看不出来，但提交后标题/正文就是错的。短字段不易触发，长文本和中文最容易中招。
用原生 setter 改完后必须回读校验，因为只派发事件不设值、或设值不派发事件，都可能让表单提交旧值。

## 可复用的产物或模板
```js
const el = document.querySelector('input[placeholder="Title"]');
const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
setter.call(el, '要写入的完整文本');
el.dispatchEvent(new Event('input', {bubbles:true}));
el.dispatchEvent(new Event('change', {bubbles:true}));
return el.value;   // 回读确认
```
textarea 用 `HTMLTextAreaElement.prototype` 的同名 setter。