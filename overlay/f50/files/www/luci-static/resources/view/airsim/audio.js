'use strict';
'require view';
'require rpc';
'require view.airsim.ui as ui';
'require view.airsim.glass as glass';

/* 音频独立页：配件包 + 刷机后自检 + 切换内核三件事放一起。
 * 只跟 airsim-audio 打交道；没装音频接口的机器上这一页直接说清楚为什么是空的。
 * 右上角版本号是**音频配件包**自己的版本（跟「检查更新」图标同一组），不是主包版本；
 * 主包版本在维护页看，这一页不重复写。 */

var audProbe = rpc.declare({ object: 'airsim-audio', method: 'audio_opt_probe', expect: {}, reject: true });
var audInstall = rpc.declare({ object: 'airsim-audio', method: 'audio_opt_install', expect: {}, reject: true, nobatch: true });
var audLatest = rpc.declare({ object: 'airsim-audio', method: 'audio_opt_latest', params: ['force'], expect: {}, reject: true, nobatch: true });
var scRun = rpc.declare({ object: 'airsim-audio', method: 'selfcheck_run', expect: {}, reject: true, nobatch: true });
var scFix = rpc.declare({ object: 'airsim-audio', method: 'selfcheck_fix', expect: {}, reject: true, nobatch: true });
var kbList = rpc.declare({ object: 'airsim-audio', method: 'kernel_bundles', expect: {}, reject: true });

var KERNEL_CHOICES = ['5.4', '6.18', '7.3'];

function card(title, body, wide) {
 return E('section', { 'class': 'as-card' + (wide ? ' as-wide' : '') }, [E('h3', {}, title)].concat(body));
}

/* ---------------- 音频配件包 ---------------- */
function optCard(lc) {
 var chip = E('span', { 'class': 'as-pill as-pill-neutral' }, '读取中');
 var msg = E('p', { 'class': 'as-caption' }, '正在读取……');
 var note = E('p', { 'class': 'as-caption' }, '');
 var busy = false;

 var rowKernel = E('dd', {}, '—');
 var rowMode = E('dd', {}, '—');
 var rowKo = E('dd', {}, '—');
 var rowOpt = E('dd', {}, '—');

 function setNotice(el, kind, title, detail) {
  el.className = 'as-caption' + (kind === 'error' ? ' as-mini-red' : (kind === 'ok' ? ' as-mini-green' : ''));
  el.textContent = (title || '') + (detail ? '：' + detail : '');
 }

 function sync(info) {
  info = info || {};
  var installed = info.installed === true, working = info.busy === true;
  busy = working;
  lc.setCurrent(info.opt_version || (info.version || '').replace(/^audio-opt-v/, ''));

  if (installed) { chip.textContent = '已安装'; chip.className = 'as-pill as-pill-green'; }
  else { chip.textContent = working ? '安装中' : '未安装'; chip.className = 'as-pill ' + (working ? 'as-pill-neutral' : 'as-pill-red'); }

  rowKernel.textContent = (info.kernel || '—') + (info.vendor_kernel ? '（厂商内核）' : '');
  rowMode.textContent = info.vendor_kernel ? '厂商自带，不用装配件包'
   : (info.matched ? '配件包里有这套（' + (info.match_dir || '') + '）'
    : (info.available ? '本包支持 ' + info.available : '本包不支持这个内核'));
  rowKo.textContent = info.vendor_kernel ? ('missing=' + (info.audio_missing || '0'))
   : ((info.ko_count || 0) + ' / ' + (info.want_ko || 0) + ' 件');
  rowOpt.textContent = (info.opt_version || '—') + (info.dsp_present ? '（DSP 在）' : '');

  if (working) setNotice(msg, 'info', '正在装', (info.message || info.state || ''));
  else if (installed && info.vendor_kernel) setNotice(msg, 'ok', '这台是厂商内核', '自带语音模块，页面这边不用再装配件包');
  else if (installed && info.modules_ok) setNotice(msg, 'ok', '已装齐', '语音那套就位');
  else if (installed) setNotice(msg, 'error', '装了但没齐', '缺 ' + ((info.want_ko || 0) - (info.ko_count || 0)) + ' 件，点重装');
  else setNotice(msg, 'info', info.message || '未安装', info.hint || '需要的话点下面的按钮装');

  var inopt = info.kernel_ok === true;
  installBtn.disabled = working || !inopt;
  installBtn.title = inopt ? '' : '当前内核不在这个配件包的覆盖范围（' + (info.opt_kernels || '') + '），装了也起不来';
  note.textContent = '配件包按内核编二进制：' + (info.opt_kernels || '—') + '。内核对不上的要换配件包，不是硬装。';
 }

 var installBtn = E('button', { 'type': 'button', 'class': 'as-button', 'disabled': true, 'click': function() {
  if (busy) return;
  installBtn.disabled = true;
  setNotice(msg, 'info', '已开始安装', '后台在跑，这页会自动刷新状态');
  audInstall().then(function(r) {
   var v = r && r[0] ? r[0] : r;
   if (v && v.ok === false) setNotice(msg, 'error', '没装成', v.error || '');
   poll();
  }).catch(function(e) { setNotice(msg, 'error', '接口没反应', String((e && e.message) || e)); });
 } }, '安装 / 重装');

 function poll() {
  audProbe().then(function(r) { sync(r && r[0] ? r[0] : r); }).catch(function() {
   chip.textContent = '不可用'; chip.className = 'as-pill as-pill-red';
   setNotice(msg, 'error', '这台机器没有音频接口', '主包版本较旧，或 airsim-audio 没装上');
  });
 }
 setTimeout(poll, 0);
 setInterval(function() { if (!document.hidden && busy) poll(); }, 3000);
 setTimeout(poll, 5000);

 return card('', [
  E('div', { 'class': 'as-card-head' }, [
   E('h3', {}, '音频配件包'),
   E('div', { 'class': 'as-head-meta' }, [lc.ver, lc.btn])
  ]),
  E('p', { 'class': 'as-copy' }, '5.4.254 是厂商内核，语音模块内核自带，这一块不用装；6.18 / 7.3 要装按内核编的这一套。'),
  lc.mini,
  E('div', { 'class': 'as-row' }, [E('dt', {}, '状态'), E('dd', {}, [chip])]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '本机内核'), rowKernel]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '覆盖情况'), rowMode]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '模块件数'), rowKo]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '配件包版本'), rowOpt]),
  msg,
  E('div', { 'class': 'as-buttons' }, [installBtn]),
  note
 ], true);
}

/* ---------------- 刷机后自检 ---------------- */
function selfCheckCard() {
 var chip = E('span', { 'class': 'as-pill as-pill-neutral' }, '没查');
 var msg = E('p', { 'class': 'as-caption' }, '刷机、升级、换内核之后点一次：内核、模块树、DSP 内存预留、开机脚本、身份元数据、蓝牙三件、hci0、主服务，八项一起看。');
 var list = E('ul', { 'class': 'as-instructions' }, []);
 var runBtn = E('button', { 'type': 'button', 'class': 'as-button', 'disabled': true }, '运行自检');
 var fixBtn = E('button', { 'type': 'button', 'class': 'as-button', 'disabled': true }, '补齐能补的');
 var scBusy = false;

 function render(res) {
  res = res || {};
  if (res.ok !== true) {
   chip.textContent = '不可用'; chip.className = 'as-pill as-pill-red';
   msg.textContent = res.error || '自检没跑起来（主包可能较旧）。';
   runBtn.disabled = false; fixBtn.disabled = true;
   return;
  }
  var items = res.items || [], okc = res.ok_count || 0, tot = res.total || items.length;
  chip.textContent = okc + '/' + tot + ' 项正常';
  chip.className = 'as-pill ' + (okc === tot ? 'as-pill-green' : (okc >= tot - 2 ? 'as-pill-neutral' : 'as-pill-red'));
  msg.textContent = '内核 ' + (res.kernel || '未知') + (res.fixed ? '；刚补了：' + res.fixed : '') + (okc === tot ? '；这一套齐了。' : '；下面标 ✗ 的就是缺的。');
  while (list.firstChild) list.removeChild(list.firstChild);
  items.forEach(function(it) {
   list.appendChild(E('li', {}, (it.ok ? '✓ ' : '✗ ') + it.name + ' —— ' + (it.detail || '')));
  });
  runBtn.disabled = false;
  fixBtn.disabled = (res.fixable_count || 0) < 1;
  fixBtn.title = fixBtn.disabled ? '没有能就地补的缺件（缺的多半属配件包，要装对应包）' : '补开机预留脚本、身份元数据、蓝牙三件（只用本机已有副本）';
 }

 function run(fix) {
  if (scBusy) return;
  scBusy = true; runBtn.disabled = true; fixBtn.disabled = true;
  msg.textContent = fix ? '正在补能补的缺件……' : '正在检查……';
  (fix ? scFix() : scRun()).then(function(r) {
   scBusy = false;
   render(r && r[0] ? r[0] : r);
  }).catch(function(e) {
   scBusy = false; runBtn.disabled = false;
   msg.textContent = '自检接口没反应：' + String((e && e.message) || e);
  });
 }

 runBtn.addEventListener('click', function() { run(false); });
 fixBtn.addEventListener('click', function() { run(true); });
 setTimeout(function() { run(false); }, 0);

 return card('刷机后自检', [
  E('p', { 'class': 'as-copy' }, '刷机丢的往往不是主包，是散在配件包里的那几件。这里一次查完，缺什么、归谁、能不能就地补，都写清楚。'),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '体检结果'), E('dd', {}, [chip])]),
  msg, list,
  E('div', { 'class': 'as-buttons' }, [runBtn, fixBtn])
 ], true);
}

/* ---------------- 切换内核（只拼命令，不在页面里执行） ---------------- */
function kernelCard() {
 var kernRow = E('dd', {}, '—');
 var instRow = E('dd', {}, '—');
 var pick = KERNEL_CHOICES[0];
 var rel = E('input', { 'type': 'text', 'class': 'as-input', 'placeholder': 'v2026.11.02.1' });
 var bundleSel = E('select', { 'class': 'as-input' });
 var cmd = E('pre', { 'class': 'as-out' }, '（选好内核和包，命令会显示在这里）');
 var tip = E('p', { 'class': 'as-caption' }, '只给命令，不代跑：切内核要写 boot 分区并重启，重启这步必须你自己来。');

 var group = E('div', { 'class': 'as-buttons' });
 KERNEL_CHOICES.forEach(function(k) {
  var b = E('button', { 'type': 'button', 'class': 'as-button', 'click': function() {
   pick = k;
   Array.prototype.forEach.call(group.children, function(x) { x.classList.toggle('is-on', x.dataset.k === k); });
   paint();
  } }, k);
  b.dataset.k = k;
  if (k === pick) b.classList.add('is-on');
  group.appendChild(b);
 });

 function paint() {
  var bundle = (bundleSel.value || '').trim();
  var release = (rel.value || '').trim() || '<上游 release 号>';
  var bundlePart = bundle || '<内核包路径>';
  cmd.textContent = 'MU300_RELEASE=' + release + ' MU300_KERNEL_BUNDLE=' + bundlePart + ' mu300-update kernel ' + pick;
 }

 bundleSel.addEventListener('change', paint);
 rel.addEventListener('input', paint);

 var copyBtn = E('button', { 'type': 'button', 'class': 'as-button', 'click': function() {
  var t = cmd.textContent || '';
  if (!t) return;
  if (navigator.clipboard && navigator.clipboard.writeText) {
   navigator.clipboard.writeText(t).then(function() { tip.textContent = '已复制，去 SSH 里粘。'; })
    .catch(function() { tip.textContent = '复制被浏览器拦了，手动选中那行命令复制。'; });
  } else {
   tip.textContent = '这个浏览器不让自动复制，手动选中那行命令复制。';
  }
 } }, '复制命令');

 setTimeout(function() {
  kbList().then(function(r) {
   var d = r && r[0] ? r[0] : r;
   kernRow.textContent = d.kernel || '—';
   instRow.textContent = (d.installed ? 'boot 里记的是 ' + d.installed : 'boot 里没记（' + (d.kernel_short || '?') + '）')
    + (d.installed_tag ? '，tag ' + d.installed_tag : '');
   if (d.installed_tag && !rel.value) rel.value = d.installed_tag;
   var bs = d.bundles || [];
   if (!bs.length) {
    bundleSel.appendChild(E('option', { 'value': '' }, '（这台机器上没有现成的内核包）'));
   } else {
    bs.forEach(function(b) {
     var mb = b.size ? (Math.round(b.size / 104857.6) / 10) + ' MB' : '';
     bundleSel.appendChild(E('option', { 'value': b.path }, b.name + (mb ? '（' + mb + '）' : '')));
    });
   }
   paint();
  }).catch(function() { kernRow.textContent = '读不到（主包较旧）'; });
 }, 0);

 return card('切换内核', [
  E('p', { 'class': 'as-copy' }, '主包不绑内核，内核是独立一档装进 boot 分区的。现在支持三档：5.4（厂商默认）、6.18、7.3。切完必须重启才生效。'),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '当前内核'), kernRow]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, 'boot 记录'), instRow]),
  E('div', { 'class': 'as-field-label' }, '目标内核'),
  group,
  E('div', { 'class': 'as-field-label' }, '上游 release 号'),
  rel,
  E('div', { 'class': 'as-field-label' }, '本机现成的内核包'),
  bundleSel,
  cmd,
  E('div', { 'class': 'as-buttons' }, [copyBtn]),
  tip,
  E('p', { 'class': 'as-caption' }, '包不在机器上时先从上游下载到 /mnt/mu300-disk/pkg/ 或 /tmp，再回来选。切换命令要以 root 在设备上跑。')
 ], true);
}

return view.extend({
 load: function() { return {}; },
 render: function() {
  ui.install();
  /* 右上角：音频配件包版本（左）+ 检查更新图标（右）。
     audio-opt 的版本号由 probe 回填（设备上实际装的那个），没有就显示 —。 */
  var lc = ui.latestChecker({ opt: 'audio-opt', installLabel: '安装/重装音频模块', check: audLatest });

  return E('div', { 'class': 'as-root' }, [
   (ui.menu ? ui.menu('audio') : ''),
   ui.stylesheet(),
   E('style', {}, glass.css()),
   E('header', { 'class': 'as-header' }, [
    E('div', {}, [
     E('div', { 'class': 'as-eyebrow' }, 'AirSIM · 音频'),
     E('h2', {}, '音频、自检、内核。'),
     E('p', {}, '配件包装没装齐、刷机后缺什么、换哪一档内核，都在这页说清。')
    ]),
    E('div', { 'class': 'as-head-meta' }, [lc.ver, lc.btn])
   ]),
   E('div', { 'class': 'as-layout' }, [optCard(lc), selfCheckCard(), kernelCard()])
  ]);
 }
});
