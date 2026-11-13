'use strict';
'require baseclass';

/* 外观状态（主题 / 密度 / 透明度）全在这一处，存在 localStorage，四页共用。
 * 页面只要 require 这个模块并在 render 开头调 ui.install()，就会自动套用
 * 上次选好的外观；设置页通过 set* 改，改完通知所有监听者更新按钮态。 */

var STORE = 'airsim.ui.appearance';
var DEFAULTS = { mode: 'auto', density: 'comfort', transparency: 'glass' };
var MODES = ['auto', 'light', 'dark'];
var DENSITIES = ['comfort', 'compact'];
var TRANSPARENCIES = ['glass', 'solid'];

var listeners = [];
var mq = null;
var saved = null;

function parse() {
 if (saved) return saved;
 var raw = null;
 try { raw = JSON.parse(window.localStorage.getItem(STORE) || '{}'); } catch (e) { raw = {}; }
 var out = {
  mode: MODES.indexOf(raw.mode) >= 0 ? raw.mode : DEFAULTS.mode,
  density: DENSITIES.indexOf(raw.density) >= 0 ? raw.density : DEFAULTS.density,
  transparency: TRANSPARENCIES.indexOf(raw.transparency) >= 0 ? raw.transparency : DEFAULTS.transparency
 };
 saved = out;
 return out;
}

function persist(next) {
 saved = next;
 try { window.localStorage.setItem(STORE, JSON.stringify(next)); } catch (e) { /* 无痕模式下忽略 */ }
}

function isDark(state) {
 if (state.mode === 'dark') return true;
 if (state.mode === 'light') return false;
 try { return !!(mq && mq.matches); } catch (e) { return false; }
}

function paint() {
 var state = parse();
 var dark = isDark(state);
 var html = document.documentElement;
 html.setAttribute('data-theme', dark ? 'dark' : 'light');
 html.setAttribute('data-density', state.density);
 html.setAttribute('data-transparency', state.transparency);
 // 告诉浏览器我们有深色配色，滚动条、输入控件跟着变
 try { html.style.colorScheme = dark ? 'dark' : 'light'; } catch (e) { /* 老浏览器忽略 */ }
 listeners.forEach(function(fn) { try { fn(state); } catch (e) { /* 单个监听失败不影响其它 */ } });
 return state;
}

function subscribe(fn) {
 listeners.push(fn);
 return function() { var at = listeners.indexOf(fn); if (at >= 0) listeners.splice(at, 1); };
}

function setMode(mode) {
 if (MODES.indexOf(mode) < 0) return parse();
 var next = parse(); next.mode = mode; persist(next); return paint();
}
function setDensity(density) {
 if (DENSITIES.indexOf(density) < 0) return parse();
 var next = parse(); next.density = density; persist(next); return paint();
}
function setTransparency(transparency) {
 if (TRANSPARENCIES.indexOf(transparency) < 0) return parse();
 var next = parse(); next.transparency = transparency; persist(next); return paint();
}

function install() {
 if (!mq) {
  try {
   mq = window.matchMedia('(prefers-color-scheme: dark)');
   var onSystemChange = function() { if (parse().mode === 'auto') paint(); };
   if (mq.addEventListener) mq.addEventListener('change', onSystemChange);
   else if (mq.addListener) mq.addListener(onSystemChange);
  } catch (e) { mq = null; }
 }
 return paint();
}

/* 页面统一用这个挂样式表，路径只写一次。
 * ui.css 改了要跟着改样式版本号，否则浏览器会一直吃旧缓存（这坑踩过两次）。 */
var CSSVERSION = '20261113m';
function stylesheet(name) {
 return E('link', { 'rel': 'stylesheet', 'href': L.resource('view/airsim/' + (name || 'ui.css')) + '?v=' + CSSVERSION });
}

/* ---------------- 顶部菜单（我们自己接管） ----------------
 * 为什么不交给主题：aurora 的横向页签在窄了以后会把后面几项裁掉，越加页越糟。
 * 这里自绘一条菜单：宽屏一行平铺、放不下就折行；窄屏（<=520）整条换成下拉。
 * 主题那边同名的 .sidebar-submenu / .mobile-nav-submenu 在 ui.css 里按页面前缀关掉了。
 */
var PAGES = [
 { id: 'overview',    title: '设备概览' },
 { id: 'connect',     title: '连接' },
 { id: 'net',         title: '联网' },
 { id: 'call',        title: '来电转发' },
 { id: 'sms',         title: '短信' },
 { id: 'maintenance', title: '维护' },
 { id: 'settings',    title: '外观与设置' },
 { id: 'bluetooth',   title: '蓝牙' },
 { id: 'audio',       title: '音频' }
];

/* 链接一律交给 L.url 生成：它知道 /cgi-bin/luci 这层前缀和实际脚本名。
 * 早先自己从 pathname 截一段，遇到 /admin/services/airsim（不带页名）就截成
 * /admin/services/，于是点蓝牙直接 404 —— 别再自己拼。 */
function menu(current) {
 current = current || '';
 var base = L.url('admin/services/airsim');
 var items = [];

 PAGES.forEach(function(page) {
  var active = page.id === current;
  items.push(E('a', {
   'class': 'as-menu-item' + (active ? ' is-active' : ''),
   'href': base + '/' + page.id,
   'aria-current': active ? 'page' : null
  }, page.title));
 });

 /* 菜单一挂上，就给 body 打标记；ui.css 靠这个标记才去关主题那排页签。
  * 万一浏览器还在用旧版 ui.js（没有 menu 函数），页面只传 null，主题页签照旧可用，不会整页没有导航。 */
 try { document.body.classList.add('as-menu-on'); } catch (e) { /* 忽略 */ }

 var picker = E('select', { 'class': 'as-input as-menu-select', 'aria-label': '切换到其它页面' },
  PAGES.map(function(page) {
   return E('option', { 'value': base + '/' + page.id, 'selected': page.id === current ? '' : null }, page.title);
  })
 );
 picker.addEventListener('change', function() {
  if (picker.value) window.location.href = picker.value;
 });

 return E('nav', { 'class': 'as-menu', 'aria-label': 'AirSIM 页面' }, [
  E('div', { 'class': 'as-menu-strip' }, items),
  /* 窄屏只保留下拉。以前这里还摆了一个 .as-menu-current 显示当前页名，
   * 加上下拉里选中的也是同一页，屏幕上就是「蓝牙 蓝牙」两遍。
   * 下拉自带 aria-label 和选中项，够用了。*/
  E('div', { 'class': 'as-menu-narrow' }, [
   picker
  ])
 ]);
}

/* 字段封装：视觉标签和真实 <label for> 一次生成。
 * 以前各页自己写 `<div class="as-field-label">`，标签只是画上去的，
 * 点它不会聚焦输入框、读屏也念不出字段名。这里统一收口：
 * node 必须有 id，没有就按顺序补一个（同一页多个字段不会撞号）；
 * 有 help 时把说明挂成 aria-describedby，读屏读字段时会带上。 */
var fieldSeq = 0;
var dialogSeq = 0;
function field(labelText, node, help) {
 if (!node) return null;
 if (!node.id) { fieldSeq += 1; node.id = 'as-f-' + fieldSeq; }
 var label = E('label', { 'for': node.id, 'class': 'as-field-label' }, labelText);
 var kids = [label, node];
 if (help) {
  var helpId = node.id + '-help';
  node.setAttribute('aria-describedby', helpId);
  kids.push(E('p', { 'class': 'as-caption', 'id': helpId }, help));
 }
 return E('div', { 'class': 'as-setting' }, kids);
}

/* 确认框：替代 vendor 模块的 confirmBox，样式走 ui.css 的 .as-modal。
 * 返回 Promise<boolean>，取消 / 点遮罩 / Esc 都为 false。 */
function dialog(title, message, opts) {
 opts = opts || {};
 return new Promise(function(resolve) {
  var settled = false;
  /* 对话框得有个名字：读屏进弹层时先念标题，否则只说「对话框」，不知道弹出的是什么。 */
  var titleId = 'as-dlg-title-' + (dialogSeq += 1);
  var ok = E('button', { 'type': 'button', 'class': 'as-button' + (opts.danger ? ' as-danger' : '') }, opts.okText || '确定');
  var cancel = E('button', { 'type': 'button', 'class': 'as-button' }, opts.cancelText || '取消');
  var mask = E('div', { 'class': 'as-modal-mask' }, [
   E('div', { 'class': 'as-modal', 'role': 'dialog', 'aria-modal': 'true', 'aria-labelledby': titleId }, [
    E('h3', { 'id': titleId }, title),
    E('p', { 'class': 'as-copy as-modal-copy' }, message || ''),
    E('div', { 'class': 'as-modal-actions' }, opts.okOnly ? [ok] : [cancel, ok])
   ])
  ]);
  function close(value) {
   if (settled) return;
   settled = true;
   document.removeEventListener('keydown', onKey, true);
   if (mask.parentNode) mask.parentNode.removeChild(mask);
   resolve(value);
  }
  function onKey(ev) {
   if (ev.key === 'Escape') { ev.stopPropagation(); close(false); }
   else if (ev.key === 'Enter') { ev.stopPropagation(); close(true); }
  }
  ok.addEventListener('click', function() { close(true); });
  if (!opts.okOnly) cancel.addEventListener('click', function() { close(false); });
  mask.addEventListener('click', function(ev) { if (ev.target === mask) close(false); });
  document.addEventListener('keydown', onKey, true);
  document.body.appendChild(mask);
  setTimeout(function() { try { ok.focus(); } catch (e) { /* 忽略 */ } }, 30);
 });
}

/* 轻提示：返回句柄，可 update 文案或 close 掉 */
function toast(text, opts) {
 opts = opts || {};
 var timer = null;
 var el = E('div', { 'class': 'as-toast', 'role': 'status' }, text);
 function arm(ms) {
  if (timer) clearTimeout(timer);
  if (ms) timer = setTimeout(function() { close(); }, ms);
 }
 function close() {
  if (timer) { clearTimeout(timer); timer = null; }
  if (el.parentNode) el.parentNode.removeChild(el);
 }
 el.setAttribute('data-kind', opts.type || 'info');
 document.body.appendChild(el);
 arm(opts.timeout === undefined ? 2600 : opts.timeout);
 return {
  update: function(next, kind) { el.textContent = next; if (kind) el.setAttribute('data-kind', kind); arm(opts.timeout === undefined ? 2600 : opts.timeout); },
  close: close,
  el: el
 };
}

/* 按钮忙碌态：只切 disabled 与半透明，不改文案 */
function busy(button, flag) {
 if (!button) return;
 button.disabled = !!flag;
 if (flag) button.classList.add('is-busy');
 else button.classList.remove('is-busy');
}

/* ---------------- 配件包「检查更新」小部件（音频/蓝牙两个页面共用） ----------------
 * 卡片右上角一组：左边版本号，右边圆形图标按钮。图标随状态变：
 * ↻ 可查（灰） / 旋转 正在查 / ✓ 已是最新 / ↑ 有新版本（绿） / ⚠ 查不到（红）。
 * 不挂常亮箭头：常亮箭头等于承诺"随时能升"，会误导。
 * 用法：ui.latestChecker({ opt:'audio-opt', installLabel:'安装音频模块', check: audioLatest })
 */
function stampEpoch(epoch) {
 var n = Number(epoch || 0);
 if (!n) return '';
 var d = new Date(n * 1000), p2 = function(x) { return (x < 10 ? '0' : '') + x; };
 return p2(d.getMonth() + 1) + '-' + p2(d.getDate()) + ' ' + p2(d.getHours()) + ':' + p2(d.getMinutes());
}

function latestChecker(o) {
 var ver = E('span', { 'class': 'as-ver' }, o.opt + ' v' + (o.current || '—'));
 var glyph = E('span', { 'class': 'as-glyph' }, '↻');
 var btn = E('button', { 'type': 'button', 'class': 'as-icon-btn', 'title': '检查更新（6 小时内不重复联网）', 'aria-label': '检查更新（6 小时内不重复联网）' }, [glyph]);
 var mini = E('p', { 'class': 'as-caption as-opt-mini' }, '');
 var busy = false, painted = false;

 /* 图标按钮里只有符号没有文字，title 只能给鼠标看，读屏要的是 aria-label。
  * 两个一起改，免得状态换了名称还是老的。 */
 function setHint(text) {
  btn.title = text;
  btn.setAttribute('aria-label', text);
 }

 function paint(res, forced) {
  painted = true;
  res = res || {};
  var latest = res.latest_ver || '', cur = res.current_ver || o.current || '';
  if (res.has_update === true && latest) {
   ver.textContent = o.opt + ' v' + cur + ' → v' + latest;
   glyph.textContent = '↑';
   btn.className = 'as-icon-btn as-icon-on';
   setHint('有新版本 ' + (res.latest_tag || '') + '：点下面的按钮会直接拉新版');
   mini.className = 'as-caption as-opt-mini as-mini-green';
   mini.textContent = '有新版本 ' + (res.latest_tag || ('v' + latest)) + '，点「' + o.installLabel + '」会直接装这个新版。';
   return;
  }
  if (res.ok === true) {
   ver.textContent = o.opt + ' v' + (cur || latest);
   glyph.textContent = '✓';
   btn.className = 'as-icon-btn';
   mini.className = 'as-caption as-opt-mini';
   mini.textContent = '已是最新' + (latest ? '（v' + latest + '）' : '')
    + (stampEpoch(res.checked_at) ? '，' + stampEpoch(res.checked_at) + ' 查的' : '')
    + (res.cooldown === true ? '；冷却中，这次没联网' : '')
    + (res.error ? '。' + res.error : '');
   return;
  }
  glyph.textContent = '⚠';
  btn.className = 'as-icon-btn';
  mini.className = 'as-caption as-opt-mini as-mini-red';
  mini.textContent = (res.error || '查不到最新版本')
   + (stampEpoch(res.checked_at) ? '；上次查到的：v' + (latest || '—') + '（' + stampEpoch(res.checked_at) + '）' : '')
   + (forced ? '；可能是脱网或 GitHub 暂时拒绝，过会儿再点' : '');
 }

 function run(force) {
  if (busy) return Promise.resolve();
  busy = true; btn.disabled = true; glyph.className = 'as-glyph as-spin';
  return o.check(force ? 1 : 0).then(function(r) {
   paint(r, force);
  }).catch(function(e) {
   glyph.textContent = '⚠';
   mini.className = 'as-caption as-opt-mini as-mini-red';
   mini.textContent = '接口没反应：' + String((e && e.message) || e);
  }).then(function() {
   busy = false; btn.disabled = false; glyph.className = 'as-glyph';
  });
 }

 btn.addEventListener('click', function() { run(true); });

 function setCurrent(v) {
  if (!v || painted) return;
  ver.textContent = o.opt + ' v' + v;
 }

 return { ver: ver, btn: btn, mini: mini, run: run, paint: paint, setCurrent: setCurrent };
}

return baseclass.extend({
 install: install,
 paint: paint,
 subscribe: subscribe,
 stylesheet: stylesheet,
 menu: menu,
 dialog: dialog,
 field: field,
 toast: toast,
 busy: busy,
 getMode: function() { return parse().mode; },
 getDensity: function() { return parse().density; },
 getTransparency: function() { return parse().transparency; },
 setMode: setMode,
 setDensity: setDensity,
 setTransparency: setTransparency,
 isDark: function() { return isDark(parse()); },
 stampEpoch: stampEpoch,
 latestChecker: latestChecker
});
