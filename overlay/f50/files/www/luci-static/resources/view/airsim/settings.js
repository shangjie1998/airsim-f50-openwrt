'use strict';
'require view';
'require rpc';
'require view.airsim.ui as ui';
'require view.airsim.glass as glass';

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
   (ui.menu ? ui.menu('settings') : ''),
   ui.stylesheet(),
   E('style', {}, glass.css()),
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
    E('section', { 'class': 'as-card' }, [
     E('h3', {}, '关于'),
     E('p', { 'class': 'as-copy', style: 'margin-top:8px' }, 'AirSIM for F50 是装在中兴 UMS9620（展锐 marlin3，aarch64）这台随身 Wi-Fi 上的自编译插件。它把这台机器本来散在系统各处的东西收到一处：电话网关与客户端配对、上网通道切换、音频、蓝牙、短信，再加一个刷机后自检。'),
     E('dl', { 'style': 'margin-top:10px' }, [
      E('div', { 'class': 'as-row' }, [E('dt', {}, '插件版本'), E('dd', {}, version)]),
      E('div', { 'class': 'as-row' }, [E('dt', {}, '产品'), E('dd', {}, 'AirSIM for F50')]),
      E('div', { 'class': 'as-row' }, [E('dt', {}, '机型'), E('dd', {}, '中兴 UMS9620 · 展锐 marlin3')]),
      E('div', { 'class': 'as-row' }, [E('dt', {}, '运行内核'), E('dd', {}, '主包不绑内核；5.4 / 6.18.60 / 7.3.1 都能跑，实际版本看「维护」页自检')]),
      E('div', { 'class': 'as-row' }, [E('dt', {}, '设备标识'), E('dd', {}, deviceId)]),
      E('div', { 'class': 'as-row' }, [E('dt', {}, '外观设置'), E('dd', {}, '仅保存在当前浏览器')])
     ]),
     E('p', { 'class': 'as-caption', style: 'margin-top:12px' }, '换电脑或换浏览器，外观要重新选一次；设备本身的状态不受影响。'),
     E('p', { 'class': 'as-caption' }, '这是第三方自编译系统上的插件，与厂商无关。改内核、刷机都有风险；每台机器都留了回退路径，具体命令写在仓库文档里。'),
     E('p', { 'class': 'as-caption' }, '源码与发布：github.com/shangjie1998/airsim-f50-openwrt')
    ])
   ]),
   E('footer', { 'class': 'as-footer' }, 'AirSIM for F50 · 设置')
  ]);
 },
 handleSaveApply: null,
 handleSave: null,
 handleReset: null
});
