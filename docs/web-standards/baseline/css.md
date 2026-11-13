# CSS 能力卡

> 判定权归 `targets.md` 的 T1（iPad Safari）。没在 T1 真机跑过的不许进 A 档。
> 每条固定三行：**能不能用 / 怎么用 / 不能用时怎么写**。

## A 档 · 已在本项目实测可用

直接写，不用犹豫。

| 特性 | 怎么用 | 备注 |
|---|---|---|
| `flex` / `grid` / `gap` | 一维用 flex，二维用 grid，间距一律 gap，不用 margin 堆 | 卡片网格用 `repeat(auto-fit, minmax(220px,1fr))` |
| 自定义属性 `var()` | 颜色、圆角、模糊值全部走 token | 深浅两套 token 同结构 |
| `clamp()` / `min()` / `max()` | 字号与间距做流体 | 字号下限不得低于 13px（嵌入式页面远看多） |
| `backdrop-filter` | 毛玻璃只用在这一层：头部栏、弹层 | 层级 ≤ 2，不放进滚动容器 |
| `prefers-color-scheme` + 类切换 | 深色走两套 token，允许手动覆盖 | 两套都要过对比度 |
| `box-shadow` / `border-radius` | 克制：单层柔和阴影，圆角只用一种尺度 | 大圆角配单边边框会画成弧线，见 D 档注 |
| `transition`（属性白名单） | 只过渡 `opacity`、`transform`、`background-color` | 不过渡 `width/height/backdrop-filter` |

## B 档 · 平台成熟，首次使用须在 T1 跑一次并升 A

可以用，但第一次用必须截图为证。

| 特性 | 怎么用 | 不能用时怎么写 |
|---|---|---|
| `:has()` | 父级条件样式，省一层额外 class | 回退成显式 class 标记 |
| container queries | 组件内部响应，不依赖窗口宽度 | 回退成媒体查询 |
| `subgrid` | 多卡片内部元素对齐 | 回退成显式 `grid-template-rows` 固定行高 |
| `aspect-ratio` | 图片、开关、色块占位 | 回退成 `padding-top` 百分比占位 |
| `line-clamp` | 多行截断 | 回退成定高 + `overflow: hidden` |
| `color-mix()` | 从 token 派生浅色/深色 | 回退成预计算好的第二个 token |
| `light-dark()` | 减少一套深色 token | 回退成类切换两套 token（我们现在就是这套） |
| 逻辑属性 `inline-size` / `margin-inline` | 新代码优先 | 回退成物理属性 |
| `:focus-visible` | 键盘焦点圈，不污染鼠标点击 | 回退成 `:focus` |
| `text-wrap: balance` | 标题折行好看 | 回退不管，允许普通折行 |
| CSS 嵌套 | 新样式层局部使用 | 回退成扁平选择器 |
| 原生 `dialog` / `popover` | 弹层优先用原生，省手写遮罩 | 回退成现有自绘弹层 |

## C 档 · 谨慎，禁用于主线流程

能做但不是现在做；用之前必须单独验、单独写降级。

| 特性 | 风险 | 处理 |
|---|---|---|
| anchor positioning | T1 支持不稳，polyfill 不能引 | 不用；需要锚点跟随就用自绘定位 |
| view transitions | 跨页面切换在 Safari 表现不一致 | 不用；页面切换不加动效 |
| scroll-driven animations | 滚动性能不可控 | 不用 |
| `@container style queries` | 支持面窄 | 不用 |
| `backdrop-filter` 叠加 > 2 层 | 掉帧 | 降层级或改半透明纯色 |

## D 档 · 禁用

见到就改，没有例外。

| 禁用项 | 原因 | 替代 |
|---|---|---|
| `column-count` / `column-width` 多列 | Safari 上整块卡片不显示（本项目踩过） | grid 或 flex 分列 |
| `display: contents` | Safari 同样会让整块消失 | 用 grid 布局表达层级 |
| 外部 `@import` / 外部字体 / 任何 CDN | 设备可能无外网 | 全部内联；字体用系统字体栈 |
| polyfill（含条件加载） | 体积与无外网双重限制 | 用 T1 原生能力，不支持就降级 |
| `width/height` 参与 transition | 触发布局抖动 | 用 `transform: scale` |
| 单边 `border` 配 `border-radius` | 圆角会把单边线两端画成弧 | 要么四边都有边框，要么 `border-radius: 0` |

## 维护规则

- 新增特性：先加进 B 或 C，T1 验过再挪进 A，**不许直接写进 A**。
- 每次挪档，在文件末尾「变更记录」加一行：日期、特性、从哪档到哪档、证据（截图文件名）。

## 变更记录

| 日期 | 特性 | 变动 | 证据 |
|---|---|---|---|
| 2026-11-13 | 初版 | A/B/C/D 分档建立 | AirSIM F50 LuCI 视图现状（8 个视图零外链，glass.js 使用 backdrop-filter） |
