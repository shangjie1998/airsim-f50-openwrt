'use strict';
'require view';
'require rpc';
'require view.airsim.ui as ui';

var getStatus = rpc.declare({ object: 'airsim', method: 'status', expect: {} });

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


return view.extend({
 load: function() {
  return L.resolveDefault(getStatus(), {});
 },
 render: function(data) {
  var current = (data) || {};
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
