'use strict';
'require view';
'require rpc';
'require view.airsim.ui as ui';

var getStatus = rpc.declare({ object: 'airsim', method: 'status', expect: {} });
/* 蓝牙开关走 airsim-bt：逻辑在设备上，机型差异只在 /etc/config/airsim-bt */
var btGet = rpc.declare({ object: 'airsim-bt', method: 'bt_get', expect: {} });
var btSet = rpc.declare({ object: 'airsim-bt', method: 'bt_set', params: ['enable'], expect: {}, reject: true, nobatch: true });

function segmented(options, current, onPick) {
 var group = E('div', { 'class': 'as-segmented', 'role': 'group' });
 var buttons = [];
 options.forEach(function(opt) {
  var btn = E('button', { 'type': 'button', 'class': 'as-seg', 'click': function() {
   onPick(opt.value);
   buttons.forEach(function(b) { b.setAttribute('aria-pressed', b.dataset.value === opt.value ? 'true' : 'false'); });
  } }, opt.label);
  btn.dataset.value = opt.value;
  btn.setAttribute('aria-pressed', opt.value === current ? 'true' : 'false');
  buttons.push(btn);
  group.appendChild(btn);
 });
 return group;
}

/* 蓝牙卡：状态与开关都走 airsim-bt，前端不认识机型细节。
 * 设备没装 airsim-bt 配置时整张卡不出现（F50 接上去只要放一份配置就有）。 */
/* 蓝牙卡：开关、名字、状态、已配对设备、地址、可见性都走 airsim-bt。
 * 前端不认识机型细节；设备没装 airsim-bt 时整张卡不出现。 */
function btCard(bt) {
 bt = bt || {};

 var busy = false;

 /* 状态行：把「挂上了没有」说清楚，别让用户对着“未挂载”猜原因 */
 function stateText(s) {
  if (!s || s.ok === false) return '读取失败';
  if (!s.supported) return '不支持';
  if (!s.enabled) return '已关闭';
  if (s.attached && s.up) return s.bluetoothd ? '正常' : '已挂载（蓝牙服务未就绪）';
  if (s.attached) return '已挂载，初始化中';
  return '未挂载';
 }

 function summary(s) {
  if (!s || s.ok === false) return s && s.error ? '操作失败：' + s.error : '蓝牙状态读取失败，可刷新页面再试。';
  if (!s.supported) return '这台设备没装蓝牙配置。';
  if (!s.enabled) return '蓝牙已关闭，控制器不会挂载，也省一点待机功耗。';
  if (s.attached && s.up) return (s.name || '控制器') + ' 已挂载' + (s.address ? '，地址 ' + s.address : '') + (s.bluetoothd ? '，可被发现。' : '，蓝牙服务还没识别。');
  if (s.attached) return '控制器已经挂上了，正在初始化，再等十几秒就绪。';
  return s.hint || '正在挂载控制器，稍等一下。';
 }

 /* 可见性两行：手机能不能搜到（可被发现）、能不能发起配对 */
 function visText(s) {
  if (!s || !s.attached) return '未挂载';
  return (s.discoverable ? '可被发现' : '不可被发现') + ' · ' + (s.pairable ? '可配对' : '不可配对');
 }

 /* 已配对设备：没有就直说没有，别留一片空白让人以为坏了 */
 function devList(s) {
  var list = (s && s.devices) || [];
  if (!s || !s.attached) return E('p', { 'class': 'as-caption' }, '控制器没挂上，暂时读不到已配对设备。');
  if (!list.length) return E('p', { 'class': 'as-caption' }, '还没有已配对设备。手机、耳机搜到「' + (s.name || 'F50') + '」后点连接就会出现在这里。');
  return E('ul', { 'class': 'as-devs', style: 'margin:6px 0 0;padding-left:18px;line-height:1.7' }, list.map(function(d) {
   return E('li', {}, [
    E('strong', {}, d.name || '未命名'),
    E('span', { 'class': 'as-caption' }, ' ' + d.address + (d.connected ? ' · 已连接' : '')),
   ]);
  }));
 }

 /* 这四个值做成活节点：开关一动就整体重画，不留旧值 */
 var stateValue = E('dd', {}, stateText(bt));
 var info = E('p', { 'class': 'as-caption', style: 'margin-top:12px' }, summary(bt));
 var nameValue = E('dd', {}, (bt && bt.name) || '—');
 var addrValue = E('dd', {}, (bt && bt.address) || '—');
 var visValue = E('dd', {}, visText(bt));
 var devValue = E('div', {}, devList(bt));

 var group = segmented([
  { value: '1', label: '开' }, { value: '0', label: '关' }
 ], bt.enabled ? '1' : '0', function(value) { toggle(value === '1'); });

 function paint(enable) {
  group.querySelectorAll('button').forEach(function(b) {
   b.setAttribute('aria-pressed', b.dataset.value === (enable ? '1' : '0') ? 'true' : 'false');
  });
 }

 function apply(s) {
  stateValue.textContent = stateText(s);
  info.textContent = summary(s);
  nameValue.textContent = (s && s.name) || '—';
  addrValue.textContent = (s && s.address) || '—';
  visValue.textContent = visText(s);
  devValue.textContent = '';
  devValue.appendChild(devList(s));
 }

 function lock(flag) {
  busy = flag;
  group.querySelectorAll('button').forEach(function(b) { b.disabled = flag; });
 }

 /* 开关只切开关；名字、地址、配对列表都是读出来的，不由前端改 */
 function toggle(enable) {
  if (busy) return;
  lock(true);
  btSet(enable ? '1' : '0').then(function(res) {
   var s = res || {};
   lock(false);
   paint(!!s.enabled);
   apply(s);
   ui.toast(enable ? (s.attached ? '蓝牙已打开' : '蓝牙正在启动，稍等十几秒') : '蓝牙已关闭');
  }).catch(function(err) {
   lock(false);
   paint(!enable);
   info.textContent = '操作失败：' + (err && err.message ? err.message : err);
  });
 }

 var rows = [
  E('div', { 'class': 'as-row' }, [E('dt', {}, '状态'), stateValue]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '名称'), nameValue]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '蓝牙地址'), addrValue]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '可见性'), visValue]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '蓝牙服务'), E('dd', {}, bt.service || '—')]),
 ];

 return [
  E('h3', {}, bt.label || '蓝牙'),
  E('p', { 'class': 'as-copy', style: 'margin-top:8px' }, '关掉会释放射频和一点待机功耗；打开后控制器会挂上，并默认保持可被发现、可配对，手机和耳机在蓝牙列表里能直接搜到这台设备。'),
  E('div', { 'class': 'as-field-label' }, '开关'),
  group,
  info,
  rows.length ? E('dl', { style: 'margin-top:10px' }, rows) : null,
  E('div', { 'class': 'as-field-label', style: 'margin-top:14px' }, '已配对设备'),
  devValue
 ].filter(function(item) { return item !== null; });
}

return view.extend({
 load: function() {
  return Promise.all([ L.resolveDefault(getStatus(), {}), L.resolveDefault(btGet(), {}) ]);
 },
 render: function(data) {
  var current = (data && data[0]) || {};
  var bt = (data && data[1]) || {};
  ui.install();
  var state = { mode: ui.getMode(), density: ui.getDensity(), transparency: ui.getTransparency() };

  var modeGroup = segmented([
   { value: 'auto', label: '跟随系统' }, { value: 'light', label: '浅色' }, { value: 'dark', label: '深色' }
  ], state.mode, function(value) { ui.setMode(value); });

  var densityGroup = segmented([
   { value: 'comfort', label: '舒适' }, { value: 'compact', label: '紧凑' }
  ], state.density, function(value) { ui.setDensity(value); });

  var transparencyGroup = segmented([
   { value: 'glass', label: '玻璃模糊' }, { value: 'solid', label: '纯色背景' }
  ], state.transparency, function(value) { ui.setTransparency(value); });

  var version = current.version ? 'v' + current.version : '—';
  var deviceId = current.agent_id ? '…' + String(current.agent_id).slice(-8) : '—';

  return E('div', { 'class': 'as-root' }, [
   ui.stylesheet(),
   E('header', { 'class': 'as-header' }, [
    E('div', {}, [E('div', { 'class': 'as-eyebrow' }, 'AirSIM · 设置'), E('h2', {}, '按你的习惯来。'), E('p', {}, '外观与显示偏好，只影响这台设备的网页界面。')]),
    E('span', { 'class': 'as-version' }, version)
   ]),
   E('div', { 'class': 'as-layout' }, [
    E('section', { 'class': 'as-card' }, [
     E('h3', {}, '外观'),
     E('p', { 'class': 'as-copy', style: 'margin-top:8px' }, '选择「跟随系统」时，iPhone、iPad、电脑切换深色，这里会一起变。'),
     E('div', { 'class': 'as-field-label' }, '模式'),
     modeGroup,
     E('p', { 'class': 'as-caption', style: 'margin-top:12px' }, '选择会记住，下次打开仍然是这个模式。')
    ]),
    E('section', { 'class': 'as-card' }, [
     E('h3', {}, '显示'),
     E('div', { 'class': 'as-field-label' }, '界面密度'),
     densityGroup,
     E('div', { 'class': 'as-field-label' }, '卡片背景'),
     transparencyGroup,
     E('p', { 'class': 'as-caption', style: 'margin-top:12px' }, '玻璃模糊在低配置设备或老浏览器上可能掉帧，掉帧就切「纯色背景」。')
    ]),
    bt.supported === false ? null : E('section', { 'class': 'as-card as-wide' }, btCard(bt)),
    E('section', { 'class': 'as-card as-wide' }, [
     E('h3', {}, '关于'),
     E('dl', { 'style': 'margin-top:10px' }, [
      E('div', { 'class': 'as-row' }, [E('dt', {}, '插件版本'), E('dd', {}, version)]),
      E('div', { 'class': 'as-row' }, [E('dt', {}, '设备标识'), E('dd', {}, deviceId)]),
      E('div', { 'class': 'as-row' }, [E('dt', {}, '产品'), E('dd', {}, 'AirSIM for F50')]),
      E('div', { 'class': 'as-row' }, [E('dt', {}, '外观设置'), E('dd', {}, '仅保存在当前浏览器')])
     ]),
     E('p', { 'class': 'as-caption', style: 'margin-top:12px' }, '换电脑或换浏览器，外观需要重新选一次；设备本身的状态不受影响。')
    ])
   ]),
   E('footer', { 'class': 'as-footer' }, 'AirSIM for F50 · 设置')
  ]);
 },
 handleSaveApply: null,
 handleSave: null,
 handleReset: null
});
