'use strict';
'require view';
'require rpc';
'require view.airsim.ui as ui';

/* 蓝牙独立页：从「外观与设置」里拎出来的，放在它后面。
 * 这一页只跟 airsim-bt 打交道，机型差异全在设备上的 /etc/config/f50-bt。
 * 没装蓝牙栈的机器上这一页连菜单都不出现，见 install.sh 里的移除逻辑。 */
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

var btGet = rpc.declare({ object: 'airsim-bt', method: 'bt_get', expect: {} });
var btSet = rpc.declare({ object: 'airsim-bt', method: 'bt_set', params: ['enable'], expect: {}, reject: true, nobatch: true });
var btUnpair = rpc.declare({ object: 'airsim-bt', method: 'bt_unpair', params: ['address'], expect: {}, reject: true, nobatch: true });

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

 /* 已配对设备：没有就直说没有，别留一片空白让人以为坏了。
  * 每台后面跟一个「删除」——删之前先问一句，删完重新拉一次状态。 */
 function devList(s, onRemove) {
  var list = (s && s.devices) || [];
  if (!s || !s.attached) return E('p', { 'class': 'as-caption' }, '控制器没挂上，暂时读不到已配对设备。');
  if (!list.length) return E('p', { 'class': 'as-caption' }, '还没有已配对设备。手机、耳机搜到「' + (s.name || 'F50') + '」后点连接就会出现在这里。');
  return E('ul', { 'class': 'as-devs', style: 'margin:6px 0 0;padding-left:18px;line-height:1.9' }, list.map(function(d) {
   return E('li', { style: 'display:flex;align-items:center;gap:10px;justify-content:space-between' }, [
    E('span', {}, [
     E('strong', {}, d.name || '未命名'),
     E('span', { 'class': 'as-caption' }, ' ' + d.address + (d.connected ? ' · 已连接' : '')),
    ]),
    E('button', {
     'type': 'button',
     'class': 'as-btn as-btn-ghost',
     'style': 'flex:none;font-size:12px;padding:4px 10px',
     'click': function(ev) {
      ev.preventDefault();
      if (!onRemove) return;
      if (!window.confirm('删除「' + (d.name || d.address) + '」的配对？\n删了以后，这台设备下次连接要重新配对。')) return;
      onRemove(d);
     }
    }, '删除')
   ]);
  }));
 }

 /* 这四个值做成活节点：开关一动就整体重画，不留旧值 */
 var stateValue = E('dd', {}, stateText(bt));
 var info = E('p', { 'class': 'as-caption', style: 'margin-top:12px' }, summary(bt));
 var nameValue = E('dd', {}, (bt && bt.name) || '—');
 var addrValue = E('dd', {}, (bt && bt.address) || '—');
 var visValue = E('dd', {}, visText(bt));
 var devValue = E('div', {}, devList(bt, removeDev));

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
  devValue.appendChild(devList(s, removeDev));
 }

 /* 删一台已配对设备：后端先断开再 remove，删完重拉一遍整页状态 */
 function removeDev(d) {
  if (busy) return;
  lock(true);
  info.textContent = '正在删除「' + (d.name || d.address) + '」…';
  btUnpair(d.address).then(function(res) {
   var r = res || {};
   if (r.ok === false) {
    lock(false);
    info.textContent = '删除失败：' + (r.error || '蓝牙服务没响应，刷新后再试一次');
    return;
   }
   return L.resolveDefault(btGet(), {}).then(function(s) {
    lock(false);
    paint(!!(s && s.enabled));
    apply(s || {});
    info.textContent = r.message || ('已删除「' + (d.name || d.address) + '」');
    ui.toast('已删除配对：' + (d.name || d.address));
   });
  }).catch(function(err) {
   lock(false);
   info.textContent = '删除失败：' + (err && err.message ? err.message : err);
  });
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
  return L.resolveDefault(btGet(), {});
 },
 render: function(bt) {
  bt = bt || {};
  ui.install();
  var version = bt.version ? 'v' + bt.version : '';

  return E('div', { 'class': 'as-root' }, [
   ui.stylesheet(),
   E('header', { 'class': 'as-header' }, [
    E('div', {}, [
     E('div', { 'class': 'as-eyebrow' }, 'AirSIM · 蓝牙'),
     E('h2', {}, '蓝牙在这儿。'),
     E('p', {}, '开关、名字、已配对设备都在这页；关掉重启仍是关。')
    ])
   ]),
   E('div', { 'class': 'as-layout' }, [
    bt.supported === false
     ? E('section', { 'class': 'as-card as-wide' }, [
        E('h3', {}, '蓝牙'),
        E('p', { 'class': 'as-caption', style: 'margin-top:10px' }, '这台设备没有蓝牙服务（/usr/libexec/rpcd/airsim-bt 不在位），装好蓝牙那一套再回来看。')
       ])
     : E('section', { 'class': 'as-card as-wide' }, btCard(bt)),
    E('section', { 'class': 'as-card as-wide' }, [
     E('h3', {}, '说明'),
     E('p', { 'class': 'as-caption', style: 'margin-top:10px' }, '名字按 F50-<蓝牙地址后 6 位> 自动生成，每台机器唯一、重启不变；手机蓝牙列表里看到的就是它。'),
     E('p', { 'class': 'as-caption' }, '控制器挂上后默认保持可被发现、可配对，不用每次手动点；配对由手机或耳机那边发起，配完会出现在「已配对设备」里，不要了就在那一条后面点「删除」。'),
     E('p', { 'class': 'as-caption' }, '删除会先断开连接再删记录；删完「可被发现 / 可配对」会自动复位，不用再开一次开关。')
    ])
   ])
  ]);
 }
});
