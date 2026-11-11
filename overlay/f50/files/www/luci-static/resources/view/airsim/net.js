'use strict';
'require view';
'require rpc';
'require view.airsim.ui as ui';
'require view.airsim.glass as glass';

/* 联网（上网通道切换 + Wi-Fi 管理）
 *
 * 这页只跟 airsim-net 打交道，真正的门在设备上的 /usr/bin/airsim-netmode：
 * 模式判断只看 uci 三个值（network.wan.disabled / wireless.ap0.disabled / wireless.wwan0.disabled），
 * 页面不另存状态，每次刷新都回读设备。
 *
 * 两个别改坏的约定：
 * 1. 切模式要 8~30 秒才稳（wpa_supplicant 重启 + 认证 + DHCP），点完不能立刻刷状态，得轮询。
 * 2. 密码只在「连接」那一下发出去，读回来永远没有密码 —— 后端不给。 */

var netScreen    = rpc.declare({ object: 'airsim-net', method: 'screen', expect: {} });
var netProbe     = rpc.declare({ object: 'airsim-net', method: 'probe', expect: {} });
var netStatus    = rpc.declare({ object: 'airsim-net', method: 'status', expect: {} });
var netModeSet   = rpc.declare({ object: 'airsim-net', method: 'mode_set', params: ['mode'], expect: {}, nobatch: true });
var netScan      = rpc.declare({ object: 'airsim-net', method: 'wifi_scan', expect: {} });
var netSaved     = rpc.declare({ object: 'airsim-net', method: 'wifi_saved', expect: {} });
var netConnect   = rpc.declare({ object: 'airsim-net', method: 'wifi_connect', params: ['ssid', 'key', 'encryption'], expect: {}, reject: true, nobatch: true });
var netForget    = rpc.declare({ object: 'airsim-net', method: 'wifi_forget', expect: {}, reject: true, nobatch: true });
var netAdminSt   = rpc.declare({ object: 'airsim-net', method: 'admin_status', expect: {} });
var netAdminSet  = rpc.declare({ object: 'airsim-net', method: 'admin_set', params: ['enable'], expect: {}, reject: true, nobatch: true });

/* 只有两种状态，一个开关就够：开 = 连外部 Wi-Fi，关 = 用流量并开热点。
 * 单射频机器上「连 Wi-Fi」和「开热点」本来就互斥，拆成三档反而让人猜哪档对。 */
var MODE_TEXT = {
 wifi: '连外部 Wi-Fi 上网，蜂窝数据断开、热点关闭。',
 ap:   '用流量上网，同时开热点，手机电脑可以连 F50 的 Wi-Fi。',
 cell: '用流量上网，热点关着。'
};

function pill(text, kind) {
 return E('span', { 'class': 'as-pill' + (kind ? ' as-pill-' + kind : '') }, text);
}

return view.extend({
 load: function() {
  /* 一次调用拿齐四张卡片的数据：原来分四次，每次都要过一遍 rpcd 再起一个 shell，
   * 这颗 SoC 上光进程开销就是 60 ms 一级。 */
  return L.resolveDefault(netScreen(), {});
 },

 render: function(d) {
  ui.install();
  var probe = (d && d.probe) || {};
  var st = (d && d.status) || {};
  var saved = (d && d.saved) || {};
  var mode = st.mode || '';

  /* 能力受限的提示：这不是错误，是机器本身的边界，写清楚免得用户以为功能坏了 */
  var limits = [];
  if (probe.single_radio) limits.push('这台机器只有一个 5G 射频，Wi-Fi 与热点不能同时开：连外部 Wi-Fi 时热点会断，手机断网几秒到十几秒。');
  if (probe.band === '5g') limits.push('当前无线只在 5G 工作，扫描只列 ' + (probe.channels || '36 40 44 48') + ' 信道上的热点；2.4G 的热点扫得到也连不上，所以不列。');
  if (probe.noscan === true) limits.push('热点固定 36 信道、不做信道外扫描，这是出厂设置，不要改。');

  var statusLines = [
   E('div', { 'class': 'as-row' }, [E('dt', {}, '当前状态'), E('dd', {}, mode === 'wifi' ? 'Wi-Fi 上网' : mode === 'ap' ? '流量 + 热点' : mode === 'cell' ? '流量（热点关）' : '状态没读出来')]),
   E('div', { 'class': 'as-row' }, [E('dt', {}, '出口'), E('dd', {}, (st.route_dev || '—') + (st.sta_ip ? ' · ' + st.sta_ip : ''))]),
   E('div', { 'class': 'as-row' }, [E('dt', {}, '已连 Wi-Fi'), E('dd', {}, st.sta_ssid || '（未连接）')])
  ];

  /* ---- 卡 1：上网方式（一个滑块两态）----
   * 为什么不用 input[type=checkbox]：主题给复选框挂了伪元素渐变和勾，
   * 我们用 !important 也压不干净（真机上看到的是"蓝方块里一个勾"）。
   * 换成 button + 圆点，样式全在自己手里，主题插不进来。 */
  var feed = E('p', { 'class': 'as-feedback' }, '');
  var uplinkOn = mode === 'wifi';
  var uplinkSwitch = glass.switch(uplinkOn, function(next) {
   uplinkSwitch.disabled = true;
   feed.setAttribute('class', 'as-feedback as-feedback-pending');
   feed.textContent = next
    ? '正在连外部 Wi-Fi…先连上再断流量，最慢半分钟，期间别关页面。'
    : '正在关 Wi-Fi、开流量和热点…最慢半分钟，期间别关页面。';
   netModeSet(next ? 'wifi' : 'ap').then(function(res) {
    if (res && res.ok) {
     feed.setAttribute('class', 'as-feedback as-feedback-success');
     feed.textContent = (next ? '已切到 Wi-Fi 上网。' : '已切成流量 + 热点。') + (res.message ? ' ' + res.message : '');
     window.setTimeout(function() { window.location.reload(); }, 1500);
    } else {
     uplinkSwitch.disabled = false;
     glass.switchSet(uplinkSwitch, !next);
     feed.setAttribute('class', 'as-feedback as-feedback-error');
     feed.textContent = '没切成：' + ((res && (res.message || res.error)) || '设备没有返回原因');
    }
   }).catch(function(err) {
    uplinkSwitch.disabled = false;
    glass.switchSet(uplinkSwitch, !next);
    feed.setAttribute('class', 'as-feedback as-feedback-error');
    feed.textContent = '切换请求失败：' + (err && err.message ? err.message : err) + '（多半是半分钟内没连上 Wi-Fi，已退回原状态）';
   });
  });

  /* ---- 卡 2：Wi-Fi 列表 ---- */
  var listBox = E('div', { 'class': 'as-devs' }, [E('p', { 'class': 'as-caption' }, '还没扫描。')]);
  var scanBtn = E('button', { 'class': 'as-btn', 'type': 'button' }, '扫描');
  var manualSsid = E('input', { 'class': 'as-input', 'type': 'text', 'placeholder': '隐藏网络：直接输 Wi-Fi 名称' });
  var manualKey = E('input', { 'class': 'as-input', 'type': 'password', 'placeholder': '密码（开放网络留空）' });

  function connect(ssid, encryption, key) {
   feed.removeAttribute('class');
   feed.textContent = '正在连 ' + ssid + '…最多等 30 秒。';
   feed.className = 'as-feedback as-feedback-pending';
   netConnect(ssid, key || '', encryption || 'psk2').then(function(res) {
    if (res && res.ok) {
     feed.className = 'as-feedback as-feedback-success';
     feed.textContent = '已连上 ' + ssid + '。';
     window.setTimeout(function() { window.location.reload(); }, 1500);
    } else {
     feed.className = 'as-feedback as-feedback-error';
     feed.textContent = '连不上：' + ((res && res.message) || '密码错或信号太弱') + '（失败会自动退回原来的设置）';
    }
   }).catch(function(err) {
    feed.className = 'as-feedback as-feedback-error';
    feed.textContent = '连接请求失败：' + err;
   });
  }

  function apRow(ap) {
   var keyInput = E('input', { 'class': 'as-input', 'type': 'password', 'placeholder': '这个 Wi-Fi 的密码' });
   var panel = E('div', { 'class': 'as-details', 'style': 'display:none' }, [
    keyInput,
    E('div', { 'class': 'as-actions' }, [
     E('button', { 'class': 'as-btn', 'type': 'button', 'click': function() {
      var k = (keyInput.value || '').trim();
      if (!k && ap.encryption !== 'NONE') { keyInput.focus(); return; }
      connect(ap.ssid, ap.encryption === 'NONE' ? 'none' : 'psk2', k);
     } }, '连接')
    ])
   ]);
   var open = function() {
    var visible = panel.style.display !== 'none';
    panel.style.display = visible ? 'none' : '';
    if (!visible && ap.encryption !== 'NONE') keyInput.focus();
   };
   var locked = ap.encryption && ap.encryption !== 'NONE';
   return E('div', { 'class': 'as-dev-item', 'click': function(ev) { if (ev.target === keyInput || ev.target.tagName === 'BUTTON') return; open(); } }, [
    E('div', { 'class': 'as-inline', style: 'justify-content:space-between' }, [
     E('div', {}, [
      E('strong', {}, ap.ssid),
      E('div', { 'class': 'as-caption' }, (ap.band || '') + ' · 信道 ' + ap.channel + ' · ' + ap.signal + (locked ? ' · 加密' : ' · 开放'))
     ]),
     ap.supported === false ? pill('不支持', 'red') : (locked ? pill('加密', 'neutral') : pill('开放', 'green'))
    ]),
    panel
   ]);
  }

  function paintScan(res) {
   listBox.innerHTML = '';
   if (!res || !res.ok) {
    listBox.appendChild(E('p', { 'class': 'as-caption' }, (res && res.error) || '扫描失败。'));
    return;
   }
   if (!res.aps || !res.aps.length) {
    listBox.appendChild(E('p', { 'class': 'as-caption' }, '这个频段上没扫到可连的热点。'));
   } else {
    res.aps.forEach(function(ap) { listBox.appendChild(apRow(ap)); });
   }
   if (res.filtered) {
    listBox.appendChild(E('p', { 'class': 'as-caption' }, '另有 ' + res.filtered + ' 个在 2.4G 或其它信道上，本机连不上，没列出。'));
   }
  }

  scanBtn.addEventListener('click', function() {
   scanBtn.disabled = true;
   listBox.innerHTML = '<p class="as-caption">扫描中…</p>';
   netScan().then(paintScan).catch(function(err) {
    listBox.innerHTML = '';
    listBox.appendChild(E('p', { 'class': 'as-caption' }, '扫描请求失败：' + err));
   }).then(function() { scanBtn.disabled = false; });
  });

  /* ---- 卡 3：已保存的 Wi-Fi ---- */
  var savedBox;
  if (saved && saved.ssid) {
   savedBox = E('div', {}, [
    E('div', { 'class': 'as-inline', style: 'justify-content:space-between' }, [
     E('div', {}, [
      E('strong', {}, saved.ssid),
      E('div', { 'class': 'as-caption' }, (saved.encryption === 'psk2' ? 'WPA2' : saved.encryption === 'sae' ? 'WPA3' : '开放') + (saved.has_key ? ' · 密码已保存' : ''))
     ]),
     pill('当前' , 'green')
    ]),
    E('div', { 'class': 'as-actions' }, [
     E('button', { 'class': 'as-btn', 'type': 'button', 'click': function() {
      if (!window.confirm('删除保存的「' + saved.ssid + '」？删完要重新输密码。')) return;
      netForget().then(function(res) {
       feed.className = 'as-feedback ' + (res && res.ok ? 'as-feedback-success' : 'as-feedback-error');
       feed.textContent = (res && (res.message || res.error)) || '';
       if (res && res.ok) window.setTimeout(function() { window.location.reload(); }, 1200);
      });
     } }, '删除')
    ]),
    E('p', { 'class': 'as-caption' }, '正在用这个 Wi-Fi 上网时不能删，先切回「手机流量」再删 —— 不然删完就没网了。')
   ]);
  } else {
   savedBox = E('p', { 'class': 'as-caption' }, '还没有保存的 Wi-Fi。连上一次就会记住，下次开机自动连。');
  }

  /* ---- 卡 4：管理页面访问（安全开关）----
   * 这一页权限等同 root，能不能从 Wi-Fi 那边打开它是个安全决定，所以单列一张卡，
   * 默认关、说清楚代价。规则由后端绑在 wlan0 上：蜂窝口永远不放。 */
  var adm = (d && d.admin) || {};
  var admOn = adm.enabled === true;
  var admLive = adm.live === true;
  var admFeed = E('p', { 'class': 'as-feedback' }, '');
  admFeed.style.display = 'none';   // 没有话说的时候不要留一个空框
  var admSwitch = glass.switch(admOn, function(nextOn) {
   var next = nextOn ? 1 : 0;
   admSwitch.disabled = true;
   admFeed.style.display = '';
   admFeed.className = 'as-feedback as-feedback-pending';
   admFeed.textContent = next ? '正在打开…' : '正在关掉…';
   netAdminSet(next ? '1' : '0').then(function(res) {
    if (res && res.ok) {
     admFeed.style.display = '';
     admFeed.className = 'as-feedback as-feedback-success';
     admFeed.textContent = (res.message || '') || (next ? '已允许。' : '已禁止。');
     window.setTimeout(function() { window.location.reload(); }, 1500);
    } else {
     admSwitch.disabled = false;
     glass.switchSet(admSwitch, !nextOn);
     admFeed.style.display = '';
    admFeed.className = 'as-feedback as-feedback-error';
     admFeed.textContent = '没设成：' + ((res && (res.error || res.message)) || '设备没有返回原因');
    }
   }).catch(function(err) {
    admSwitch.disabled = false;
    glass.switchSet(admSwitch, !nextOn);
    admFeed.style.display = '';
    admFeed.className = 'as-feedback as-feedback-error';
    admFeed.textContent = '请求失败：' + err;
   });
  });

  return E('div', { 'class': 'as-root' }, [
   (ui.menu ? ui.menu('net') : ''),
   ui.stylesheet(),
   E('style', {}, glass.css()),
   E('header', { 'class': 'as-header' }, [
    E('div', {}, [
     E('div', { 'class': 'as-eyebrow' }, 'AirSIM · 联网'),
     E('h2', {}, '上网走哪条路。'),
     E('p', {}, '打开就上网走外部 Wi-Fi，关掉就上网走流量并开热点。USB 管理地址从头到尾不变。')
    ]),
    E('span', { 'class': 'as-version' }, probe.band_label ? 'Wi-Fi ' + probe.band_label : '')
   ]),
   E('div', { 'class': 'as-layout' }, [
    E('section', { 'class': 'as-card' }, [
     E('h3', {}, '上网方式'),
     E('div', { 'class': 'as-inline', style: 'justify-content:space-between' }, [
      E('div', {}, [
       E('strong', {}, uplinkOn ? '正在用 Wi-Fi 上网' : (mode === 'ap' ? '正在用流量上网，热点开着' : '正在用流量上网')),
       E('div', { 'class': 'as-caption' }, uplinkOn ? '蜂窝数据已断，热点已关' : '热点开着，手机电脑可以连 F50 的 Wi-Fi')
      ]),
      uplinkOn ? pill('Wi-Fi', 'green') : pill('流量', 'red')
     ]),
     E('div', { 'class': 'as-switch-row', 'click': function(ev) {
      if (ev.target === uplinkSwitch || uplinkSwitch.contains(ev.target)) return;
      glass.switchToggle(uplinkSwitch);
     } }, [
      E('span', { 'class': 'g-cap' }, '开'),
      uplinkSwitch,
      E('span', { 'class': 'g-cap' }, '关'),
      E('span', { 'class': 'as-caption', style: 'margin-left:4px' }, 'Wi-Fi 上网')
     ]),
     feed,
     E('dl', { 'class': 'as-facts' }, statusLines),
     E('p', { 'class': 'as-caption' }, '开：连外部 Wi-Fi 上网，流量和热点都关。关：用流量上网，并把热点开起来。顺序是固定的 —— 连 Wi-Fi 时先确认拿到地址再断流量，反过来会先丢网；失败会自动退回切换前的状态。')
    ]),

    E('section', { 'class': 'as-card' }, [
     E('h3', {}, '连接 Wi-Fi'),
     E('p', { 'class': 'as-caption' }, '扫描会占用射频：热点开着的时候扫描，手机那边会断网几秒。建议插 USB 线用电脑操作。'),
     E('div', { 'class': 'as-actions' }, [scanBtn]),
     listBox,
     E('div', { 'class': 'as-details' }, [
      E('div', { 'class': 'as-field-label' }, '隐藏网络 / 手动输入'),
      manualSsid,
      manualKey,
      E('div', { 'class': 'as-actions' }, [
       E('button', { 'class': 'as-btn', 'type': 'button', 'click': function() {
        var s = (manualSsid.value || '').trim();
        if (!s) { manualSsid.focus(); return; }
        connect(s, (manualKey.value || '').trim() ? 'psk2' : 'none', (manualKey.value || '').trim());
       } }, '连接这个网络')
      ])
     ])
    ]),

    E('section', { 'class': 'as-card' }, [
     E('h3', {}, '已保存的 Wi-Fi'),
     savedBox
    ]),

    E('section', { 'class': 'as-card' }, [
     E('h3', {}, '管理页面访问'),
     E('div', { 'class': 'as-inline', style: 'justify-content:space-between' }, [
      E('div', {}, [
       E('strong', {}, admOn ? 'Wi-Fi 那边可以打开管理页' : '只允许 USB 口打开管理页'),
       E('div', { 'class': 'as-caption' }, '只放行 80/443 · 绑定 ' + (adm.device || 'wlan0') + ' 无线口')
      ]),
      admOn ? pill(admLive ? '已生效' : '没生效', admLive ? 'red' : 'neutral') : pill('已关', 'green')
     ]),
     E('div', { 'class': 'as-switch-row', 'click': function(ev) {
      if (ev.target === admSwitch || admSwitch.contains(ev.target)) return;
      glass.switchToggle(admSwitch);
     } }, [
      admSwitch,
      E('span', {}, admOn ? '允许（从 Wi-Fi 打开）' : '禁止（只走 USB 口）')
     ]),
     admFeed,
     E('div', { 'class': 'as-notice as-notice-warn' }, '这一页权限等同 root：能改联网、能装包、能看短信。打开后，同一个上游 Wi-Fi（比如 PDCN）里的任何设备都能看到登录框并一直试密码；80 端口是明文，共享网络里有人抓包就看得见登录动作。只在自己家的 Wi-Fi 下打开，用完关掉。'),
     E('p', { 'class': 'as-caption' }, '规则只绑无线口 wlan0，切到「手机流量」后蜂窝公网进不来 —— 就算忘了关，也不会把管理页暴露在运营商网络上。USB 口的管理入口不受这个开关影响。')
    ]),

    E('section', { 'class': 'as-card' }, [
     E('h3', {}, '这台机器的限制'),
     E('div', { 'class': 'as-notice as-notice-warn' }, limits.length ? limits.join(' ') : '没有额外限制。'),
     E('p', { 'class': 'as-caption' }, 'Wi-Fi 上网时 USB 电脑只能管理 F50，不能通过 F50 上网；要供网就把开关关掉（流量 + 热点）。')
    ])
   ]),
   E('footer', { 'class': 'as-footer' }, 'AirSIM for F50 · 联网')
  ]);
 },

 handleSaveApply: null,
 handleSave: null,
 handleReset: null
});
