'use strict';
'require view';
'require rpc';
'require view.airsim.ui as ui';
'require view.airsim.glass as glass';

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
var btNearby = rpc.declare({ object: 'airsim-bt', method: 'bt_nearby', expect: {} });
var btScan = rpc.declare({ object: 'airsim-bt', method: 'bt_scan', params: ['on'], expect: {}, reject: true, nobatch: true });
var btPair = rpc.declare({ object: 'airsim-bt', method: 'bt_pair', params: ['address'], expect: {}, reject: true, nobatch: true });
var btConnect = rpc.declare({ object: 'airsim-bt', method: 'bt_connect', params: ['address'], expect: {}, reject: true, nobatch: true });
var btActionResult = rpc.declare({ object: 'airsim-bt', method: 'bt_action_result', expect: {} });
var btPairPending = rpc.declare({ object: 'airsim-bt', method: 'bt_pair_pending', expect: {} });
var btPairReply = rpc.declare({ object: 'airsim-bt', method: 'bt_pair_reply', params: ['decision'], expect: {}, reject: true, nobatch: true });

 /* 待应答轮询：btCard 每次重画都会新建一遍闭包，所以定时器只在模块级起一次，
  * 每次通过钩子指向当前那一次渲染出来的 pollPending。 */
var _pollHook = null;
var _pollStarted = false;

function startPendingPoll() {
 if (_pollStarted) return;
 _pollStarted = true;
 setTimeout(function() { if (_pollHook) _pollHook(); }, 400);
 setInterval(function() { if (!document.hidden && _pollHook) _pollHook(); }, 3000);
}

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
  return E('ul', { 'class': 'as-devs' }, list.map(function(d) {
   return E('li', { 'class': 'as-dev-item' }, [
    E('span', {}, [
     E('strong', {}, d.name || '未命名'),
     E('span', { 'class': 'as-caption' }, ' ' + d.address + (d.connected ? ' · 已连接' : '')),
    ]),
    E('button', {
     'type': 'button',
     'class': 'as-button as-mini as-danger-quiet',
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

 /* 有人往这台机器发起配对：重点是「30 秒内不点就自动拒绝」得写清楚，
  * 不然用户一犹豫就被默默拒掉了。 */
 var pendingBox = E('div', {}, []);
 var pendingTimer = null;

 function renderPending(p) {
  pendingBox.textContent = '';
  if (!p || !p.pending) return;
  var who = (p.name || '一台设备') + (p.address ? '（' + p.address + '）' : '');
  var extra = p.passkey ? '配对验证码 ' + p.passkey + '，请和对方屏幕上的一致；' : '';
  var left = p.seconds_left > 0 ? p.seconds_left : 0;
  pendingBox.appendChild(E('div', { 'class': 'as-card as-notice' }, [
   E('h3', {}, '有人要配对'),
   E('p', { 'class': 'as-copy as-lead' }, who + ' 想和这台机器配对。' + extra + (left ? '还剩 ' + left + ' 秒，超时就自动拒绝。' : '已经超时，会自动拒绝。')),
   E('div', { 'class': 'as-inline' }, [
    E('button', { 'type': 'button', 'class': 'as-button as-primary', 'click': function() { replyPending('yes'); } }, '同意配对'),
    E('button', { 'type': 'button', 'class': 'as-button as-secondary', 'click': function() { replyPending('no'); } }, '拒绝')
   ])
  ]));
 }

 function replyPending(decision) {
  btPairReply(decision).then(function() {
   pendingBox.textContent = '';
   ui.toast(decision === 'yes' ? '已同意配对' : '已拒绝这次配对');
   return L.resolveDefault(btGet(), {}).then(function(s) { apply(s || {}); });
  }).catch(function(err) {
   pendingBox.appendChild(E('p', { 'class': 'as-caption' }, '应答失败：' + (err && err.message ? err.message : err)));
  });
 }

 function pollPending() {
  btPairPending().then(function(p) {
   renderPending(p || {});
  }).catch(function() { /* 读不到就当没有请求，别弹报错 */ });
 }

 /* 附近设备：扫一次 15 秒，边扫边刷，完了把按钮放开 */
 /* 这里别再写一遍「点扫描找设备」——卡片抬头已经说了，两句话叠一起是重复。
  * 内容由 renderNearby 填，没扫过就是空着。*/
 var nearBox = E('div', {});
 var scanBtn = E('button', { 'type': 'button', 'class': 'as-btn', 'click': function() { startScan(); } }, '扫描附近的设备');

 function renderNearby(n) {
  var list = (n && n.nearby) || [];
  nearBox.textContent = '';
  if (!list.length) {
   nearBox.appendChild(E('p', { 'class': 'as-caption' }, '还没扫到设备。让对方的蓝牙可被发现，再点一次扫描。'));
   return;
  }

  /* 附近经常一堆 BLE 信标，名字就是地址，全列出来没法用。
  * 排序：已配对 > 有名字的 > 名字就是地址的。*/
  var macName = function(d) {
   return /^[0-9A-F]{2}-[0-9A-F]{2}-[0-9A-F]{2}-[0-9A-F]{2}-[0-9A-F]{2}-[0-9A-F]{2}$/.test(d.name || '');
  };
  list = list.slice().sort(function(a, b) {
   if (!!a.paired !== !!b.paired) return a.paired ? -1 : 1;
   if (macName(a) !== macName(b)) return macName(a) ? 1 : -1;
   return (a.name || '').localeCompare(b.name || '');
  });
  var shown = list.slice(0, 12);
  shown.forEach(function(d) {
   var right = d.paired
    ? E('button', { 'type': 'button', 'class': 'as-button as-mini as-secondary',
        'click': function() { doConnect(d); } }, d.connected ? '重新连接' : '连接')
    : E('button', { 'type': 'button', 'class': 'as-button as-mini as-primary',
        'click': function() { doPair(d); } }, '配对');
   nearBox.appendChild(E('div', { 'class': 'as-dev-item' }, [
    E('span', {}, [
     E('strong', {}, d.name || '未知设备'),
     E('span', { 'class': 'as-caption' }, ' ' + d.address + (d.paired ? (d.connected ? ' · 已连接' : ' · 已配对') : ''))
    ]),
    right
   ]));
  });
  if (list.length > shown.length)
   nearBox.appendChild(E('p', { 'class': 'as-caption as-lead' }, '附近还有 ' + (list.length - shown.length) + ' 台没列出来（多半是蓝牙信标），要用的话再扫一次。'));
 }

 function refreshNearby() {
  btNearby().then(function(n) { renderNearby(n || {}); }).catch(function() {});
 }

 function startScan() {
  if (busy) return;
  scanBtn.disabled = true;
  scanBtn.textContent = '正在扫描…';
  btScan('on').then(function() {
   var ticks = 0;
   var t = setInterval(function() {
    refreshNearby();
    if (++ticks >= 6) {
     clearInterval(t);
     scanBtn.disabled = false;
     scanBtn.textContent = '再扫一次';
    }
   }, 2500);
   refreshNearby();
  }).catch(function(err) {
   scanBtn.disabled = false;
   scanBtn.textContent = '扫描附近的设备';
   ui.toast('扫描失败：' + (err && err.message ? err.message : err));
  });
 }

 /* 主动配对：发起后等结果（最长 35 秒），成了就把这台 trust 上、
  * 再试着连一次，失败就把后端那句原话给用户看。 */
 function waitResult(action, address, cb) {
  var started = Math.floor(Date.now() / 1000) - 1;
  var tries = 0;
  var t = setInterval(function() {
   btActionResult().then(function(r) {
    if (r && r.done && r.action === action && r.address === address) {
     clearInterval(t);
     cb(r);
    } else if (++tries > 24) {
     clearInterval(t);
     cb({ ok: false, done: false, message: '等了 35 秒没有结果，对方可能没点确认' });
    }
   }).catch(function() {});
  }, 1500);
 }

 function doPair(d) {
  if (busy) return;
  lock(true);
  info.textContent = '正在和「' + (d.name || d.address) + '」配对，让对方点一下确认…';
  btPair(d.address).then(function() {
   waitResult('pair', d.address, function(r) {
    lock(false);
    if (r.ok) {
     info.textContent = '已和「' + (d.name || d.address) + '」配对，正在连接…';
     doConnect(d, true);
    } else {
     info.textContent = '配对没成：' + (r.message || '对方没响应');
    }
    refreshNearby();
   });
  }).catch(function(err) {
   lock(false);
   info.textContent = '配对失败：' + (err && err.message ? err.message : err);
  });
 }

 function doConnect(d, silent) {
  if (busy) return;
  lock(true);
  if (!silent) info.textContent = '正在连接「' + (d.name || d.address) + '」…';
  btConnect(d.address).then(function() {
   waitResult('connect', d.address, function(r) {
    lock(false);
    info.textContent = r.ok ? '已连接「' + (d.name || d.address) + '」。' : '连接没成：' + (r.message || '对方没响应或者不在范围');
    return L.resolveDefault(btGet(), {}).then(function(s) { apply(s || {}); });
   });
  }).catch(function(err) {
   lock(false);
   info.textContent = '连接失败：' + (err && err.message ? err.message : err);
  });
 }

 /* 这四个值做成活节点：开关一动就整体重画，不留旧值 */
 var stateValue = E('dd', {}, stateText(bt));
 var info = E('p', { 'class': 'as-caption', 'role': 'status', 'aria-live': 'polite', style: 'margin-top:12px' }, summary(bt));
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

 /* 每次重画把钩子指向本次的 pollPending；定时器模块级只起一次 */
 _pollHook = pollPending;
 startPendingPoll();

 var rows = [
  E('div', { 'class': 'as-row' }, [E('dt', {}, '状态'), stateValue]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '名称'), nameValue]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '蓝牙地址'), addrValue]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '可见性'), visValue]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '蓝牙服务'), E('dd', {}, bt.service || '—')]),
 ];

 /* 一张卡只装一件事：开关、本机信息、已配对、附近设备各归各卡 */
 return {
  pending: pendingBox,
  switchCard: [
   E('p', { 'class': 'as-copy' }, '关掉会释放射频和一点待机功耗；打开后控制器会挂上，并默认保持可被发现、可配对，手机和耳机在蓝牙列表里能直接搜到这台设备。'),
   E('div', { 'class': 'as-field-label' }, '开关'),
   group,
   info
  ],
  infoCard: [
   E('p', { 'class': 'as-caption' }, '名字按 F50-<蓝牙地址后 6 位> 自动生成，每台机器唯一、重启不变。'),
   rows.length ? E('dl', { 'class': 'as-facts' }, rows) : null
  ].filter(function(item) { return item !== null; }),
  devCard: [ devValue ],
  nearCard: [
   E('p', { 'class': 'as-caption' }, '点「扫描」找周围能连的设备（15 秒）。已配对的点「连接」，没配过的点「配对」。'),
   E('div', { 'class': 'as-buttons' }, [scanBtn]),
   nearBox
  ]
 };
}
return view.extend({
 load: function() {
  return L.resolveDefault(btGet(), {});
 },
 render: function(bt) {
  bt = bt || {};
  ui.install();
  var version = bt.version ? 'v' + bt.version : '';

  /* 卡片先攒成一个平数组再交给布局；嵌套数组会被 E 当文本渲染成 [object HTMLDivElement] */
  var cards = [];

  if (bt.supported === false) {
   cards.push(E('section', { 'class': 'as-card as-wide' }, [
    E('h3', {}, '蓝牙'),
    E('p', { 'class': 'as-caption as-lead' }, '这台设备没有蓝牙服务（/usr/libexec/rpcd/airsim-bt 不在位），装好蓝牙那一套再回来看。')
   ]));
  }
  else {
   var c = btCard(bt);
   if (c.pending) cards.push(c.pending);
   cards.push(E('section', { 'class': 'as-card as-wide' }, [E('h3', {}, bt.label || '蓝牙')].concat(c.switchCard)));
   cards.push(E('section', { 'class': 'as-card' }, [E('h3', {}, '本机蓝牙')].concat(c.infoCard)));
   cards.push(E('section', { 'class': 'as-card' }, [E('h3', {}, '已配对设备')].concat(c.devCard)));
   cards.push(E('section', { 'class': 'as-card' }, [E('h3', {}, '附近的设备')].concat(c.nearCard)));
  }

  cards.push(E('section', { 'class': 'as-card as-wide' }, [
   E('h3', {}, '说明'),
   E('p', { 'class': 'as-caption as-lead' }, '名字按 F50-<蓝牙地址后 6 位> 自动生成，每台机器唯一、重启不变；手机蓝牙列表里看到的就是它。'),
   E('p', { 'class': 'as-caption' }, '控制器挂上后默认保持可被发现、可配对，不用每次手动点；配对由手机或耳机那边发起，不要了就在「已配对设备」那一条后面点「删除」，删除会先断开再删记录，可被发现、可配对自动复位。'),
   E('p', { 'class': 'as-caption' }, '两个方向都能配：这台机器主动去配别人，就在「附近的设备」里点「配对」；别人发起配对时，页面顶上会弹「有人要配对」，点同意才继续，30 秒不点就当拒绝。')
  ]));

  return E('div', { 'class': 'as-root' }, [
   (ui.menu ? ui.menu('bluetooth') : ''),
   ui.stylesheet(),
   E('style', {}, glass.css()),
   E('header', { 'class': 'as-header' }, [
    E('div', {}, [
     E('div', { 'class': 'as-eyebrow' }, 'AirSIM · 蓝牙'),
     E('h2', {}, '蓝牙在这儿。'),
     E('p', {}, '开关、名字、已配对设备、扫描配对都在这页；关掉重启仍是关。')
    ])
   ]),
   E('div', { 'class': 'as-layout' }, cards)
  ]);
 },
 handleSaveApply: null,
 handleSave: null,
 handleReset: null
});
