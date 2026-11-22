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

/* AGDSP 主固件：专有未授权镜像，配件包/主包/公开仓都不内置，只能从上游作者 release 自备。 */
var FW_URL = 'https://github.com/dikeckaan/mu300-audio-firmware/releases/download/v1.0/agdsp.bin';
var FW_PATH = '/lib/firmware/agdsp.bin';

function card(title, body, wide) {
 return E('section', { 'class': 'as-card' + (wide ? ' as-wide' : '') }, [E('h3', {}, title)].concat(body));
}

/* ---------------- 音频配件包 ---------------- */
function optCard(lc) {
 var chip = E('span', { 'class': 'as-pill as-pill-neutral' }, '读取中');
 var msg = E('p', { 'class': 'as-caption', 'role': 'status', 'aria-live': 'polite' }, '正在读取……');
 var note = E('p', { 'class': 'as-caption' }, '');
 var busy = false;

 var rowKernel = E('dd', {}, '—');
 var rowMode = E('dd', {}, '—');
 var rowKo = E('dd', {}, '—');
 /* 固件在位状态由独立「AGDSP 主固件」卡片展示，这里不再重复提示。 */
 var rowOpt = E('dd', {}, '—');
 var rowMatch = E('dd', {}, '—');
 var rowCfg = E('dd', {}, '—');

 /* 后端 match_status 五档 → 人话。ok 之外的档位说清缺的是哪一环。 */
 function matchStatusText(s) {
  switch (s) {
   case 'ok': return '目录与模块对得上';
   case 'no_dir': return '当前内核没有对应驱动目录';
   case 'no_reserve': return '目录在，但缺 audio_reserve.ko';
   case 'bad_elf': return 'reserve.ko 不是合法可加载模块（损坏或非本架构）';
   case 'vermagic_mismatch': return '模块 vermagic 不认当前内核（别处编的）';
   default: return '—';
  }
 }

 function setNotice(el, kind, title, detail) {
  el.className = 'as-caption' + (kind === 'error' ? ' as-mini-red' : (kind === 'ok' ? ' as-mini-green' : ''));
  el.textContent = (title || '') + (detail ? '：' + detail : '');
 }

 function sync(info) {
  info = info || {};
  var installed = info.installed === true, working = info.busy === true;
  var ready = info.ready === true, toolsOk = info.tools_ok === true;
  var linksOk = info.links_ok !== false, amixerOk = info.amixer_ok !== false;
  busy = working;
  lc.setCurrent(info.opt_version || (info.version || '').replace(/^audio-opt-v/, ''));

  if (working) { chip.textContent = '安装中'; chip.className = 'as-pill as-pill-neutral'; }
  else if (ready) { chip.textContent = '已装齐'; chip.className = 'as-pill as-pill-green'; }
  else if (installed) { chip.textContent = '缺件'; chip.className = 'as-pill as-pill-red'; }
  else { chip.textContent = '未安装'; chip.className = 'as-pill ' + (info.kernel_ok === false ? 'as-pill-red' : 'as-pill-neutral'); }

  rowKernel.textContent = (info.kernel || '—') + (info.vendor_kernel ? '（厂商内核）' : '');
  rowMode.textContent = info.vendor_kernel ? '厂商自带，不用装配件包'
   : (info.kernel_ok === false ? '本包不支持这个内核（覆盖：' + (info.opt_kernels || '—') + '）'
    : (info.matched ? '配件包已装这套（' + (info.match_dir || '') + '）'
     : '支持当前内核，尚未安装'));
  rowKo.textContent = info.vendor_kernel ? ('missing=' + (info.audio_missing || '0'))
   : ((info.ko_count || 0) + ' / ' + (info.want_ko || 0) + ' 件');
  rowOpt.textContent = (info.opt_version || '—') + (info.dsp_present ? '（DSP 在）' : '');
  rowMatch.textContent = info.vendor_kernel ? '厂商内核自带，不校验配件目录' : matchStatusText(info.match_status);
  rowCfg.textContent = info.config_sha256 ? info.config_sha256.slice(0, 12) + '…（仅记录对照，不参与判定）' : '—';

  if (working) setNotice(msg, 'info', '正在装', (info.message || info.state || ''));
  else if (installed && info.vendor_kernel) setNotice(msg, 'ok', '这台是厂商内核', '自带语音模块，页面这边不用再装配件包');
  else if (ready) setNotice(msg, 'ok', '已装齐', '模块、工具链、amixer 都在，主服务能起');
  else if (installed) {
   var lack = [];
   if (!toolsOk) lack.push('工具链' + (info.tools_missing ? '（' + info.tools_missing + '）' : ''));
   if (!linksOk) lack.push('/usr/bin 链接');
   if (!amixerOk) lack.push('amixer');
   if ((info.ko_count || 0) < (info.want_ko || 0)) lack.push('模块 ' + ((info.want_ko || 0) - (info.ko_count || 0)) + ' 件');
   setNotice(msg, 'error', '装了但还不能用', '缺 ' + (lack.join('、') || '未知') + '，点「补齐缺件」会自动下载');
  }
  else setNotice(msg, 'info', info.message || '未安装', info.hint || '需要的话点下面的按钮装');

  var inopt = info.kernel_ok === true;
  installBtn.disabled = working || !inopt;
  installBtn.textContent = ready ? '重装' : (installed ? '补齐缺件' : '安装 / 重装');
  installBtn.title = inopt ? '' : '当前内核不在这个配件包的覆盖范围（' + (info.opt_kernels || '') + '），装了也起不来';
  note.textContent = '配件包按内核编二进制：' + (info.opt_kernels || '—') + '。内核对不上的要换配件包，不是硬装。缺工具链时按钮会自动从配件包仓库补。';

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
   setNotice(msg, 'error', '这台机器没有音频接口', '主包版本较旧，或 airsim-audio 没装上；按钮仍可点，会再试一次');
   installBtn.disabled = false;
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
  E('div', { 'class': 'as-row' }, [E('dt', {}, '模块校验'), rowMatch]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '配件包版本'), rowOpt]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '内核配置指纹'), rowCfg]),
  msg,
  E('div', { 'class': 'as-buttons' }, [installBtn]),
  note
 ], true);
}

/* ---------------- 刷机后自检 ---------------- */
function selfCheckCard() {
 var chip = E('span', { 'class': 'as-pill as-pill-neutral' }, '没查');
 var msg = E('p', { 'class': 'as-caption', 'role': 'status', 'aria-live': 'polite' }, '刷机、升级、换内核之后点一次：内核、模块树、DSP 内存预留、开机脚本、身份元数据、主服务，六项一起看。');
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
  fixBtn.title = fixBtn.disabled ? '没有能就地补的缺件（缺的多半属配件包，要装对应包）' : '补开机预留脚本、身份元数据（只用本机已有副本）';
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
  E('div', { 'class': 'as-buttons' }, [runBtn, fixBtn]),
  /* 这页最容易把人带沟里的一步：补齐当场看还以为没补上。补的是模块和脚本，
   * 内核不会自己去加载，得重启才生效——不写清楚就会以为补失败了。 */
  E('p', { 'class': 'as-caption' }, '补齐之后先别拿这一屏当结论：补进去的是驱动模块和脚本，内核不会自己去加载，得重启一次才生效。重启前这里照样报缺是正常的，不是又坏了。重启回来刷新这页，再点一次自检，六项都绿了才算真齐。'),
 ], true);
}

/* ---------------- 切换内核（只拼命令，不在页面里执行） ---------------- */
function kernelCard() {
 var kernRow = E('dd', {}, '—');
 var instRow = E('dd', {}, '—');
 var pick = KERNEL_CHOICES[0];
 var rel = E('input', { 'id': 'as-audio-release', 'type': 'text', 'class': 'as-input', 'placeholder': 'v2026.11.02.1' });
 var bundleSel = E('select', { 'id': 'as-audio-bundle', 'class': 'as-input' });
 var cmd = E('pre', { 'class': 'as-out' }, '（选好内核和包，命令会显示在这里）');
 /* 这个系列一个包都没有时，光说「没有」没意义 —— 得把下载命令直接摆出来，
  * 不然新人拿到干净机器根本不知道去哪儿找包。本地有包就整条藏起来。 */
 var dlBox = E('pre', { 'class': 'as-out' }, '');
 dlBox.style.display = 'none';
 /* 没包时那句话的落点：只在下包场景出现，有包就清空 */
 var warn = E('p', { 'class': 'as-caption' }, '');

 /* 下拉只列跟目标内核同系列的包。全平铺看着「选择多」，实际只会让人拿错包——
  * 这个下拉的作用就是给 mu300-update 指「拿哪个包」，系列不对等于装错内核。 */
 var allBundles = [];
 function refill(k) {
  bundleSel.textContent = '';
  var hits = allBundles.filter(function(b) { return (b.kver || '').indexOf(k + '.') === 0; });
  if (!hits.length) {
   bundleSel.appendChild(E('option', { 'value': '' }, '（这台机器上没有 ' + k + ' 系列的内核包）'));
   bundleSel.disabled = true;
  } else {
   bundleSel.disabled = false;
   hits.forEach(function(b) {
    var mb = b.size ? (Math.round(b.size / 104857.6) / 10) + ' MB' : '';
    var o = E('option', { 'value': b.path }, b.name + (b.kver ? '（内核 ' + b.kver + '）' : '') + (mb ? '（' + mb + '）' : ''));
    /* 把包的内核版本挂在 option 上，paint 靠它比对 */
    o.dataset.kver = b.kver || '';
    bundleSel.appendChild(o);
   });
  }
  paint();
 }
 var tip = E('p', { 'class': 'as-caption' }, '只给命令，不代跑：切内核要写 boot 分区并重启，重启这步必须你自己来。');
 /* 复制结果单独一行播报：原来直接改写上面那句常驻说明，复制一次说明就永久没了，回不来。 */
 var copyFeed = E('p', { 'class': 'as-caption', 'role': 'status', 'aria-live': 'polite' }, '');

 var group = E('div', { 'class': 'as-buttons' });
 KERNEL_CHOICES.forEach(function(k) {
  var b = E('button', { 'type': 'button', 'class': 'as-button', 'click': function() {
   pick = k;
   Array.prototype.forEach.call(group.children, function(x) {
    x.classList.toggle('is-on', x.dataset.k === k);
    /* 选中态要能被读屏认出来：光换颜色等于只给眼睛看。 */
    x.setAttribute('aria-pressed', x.dataset.k === k ? 'true' : 'false');
   });
   /* 换目标内核就重填下拉：只留同系列的包，别让人拿 7.3 的包去装 6.18 */
   refill(k);
  } }, k);
  b.dataset.k = k;
  if (k === pick) b.classList.add('is-on');
  b.setAttribute('aria-pressed', k === pick ? 'true' : 'false');
  group.appendChild(b);
 });

 function paint() {
  var bundle = (bundleSel.value || '').trim();
  var rv = (rel.value || '').trim() || 'v2026.11.02.1';
  /* 这个系列本地有没有包，决定给「切换命令」还是「下载命令」 */
  var has = allBundles.some(function(b) { return (b.kver || '').indexOf(pick + '.') === 0; });
  if (!has) {
   cmd.textContent = '（这台机器上没有 ' + pick + ' 系列的内核包，先下包）';
   dlBox.textContent = 'curl -L -o /mnt/mu300-disk/pkg/mu300-kernel-' + pick + '.tar.gz https://github.com/dikeckaan/mu300-linux/releases/download/' + rv + '/mu300-kernel-' + pick + '.tar.gz';
   dlBox.style.display = '';
   cmd.classList.remove('is-bad');
   cmd.style.color = '';
   warn.textContent = '下到 pkg 目录之后回这页刷新一次，下拉里就会多出这个包，再复制切换命令去跑。';
   return;
  }
  cmd.textContent = 'MU300_RELEASE=' + rv + ' MU300_KERNEL_BUNDLE=' + bundle + ' mu300-update kernel ' + pick;
  cmd.classList.remove('is-bad');
  cmd.style.color = '';
  dlBox.style.display = 'none';
  warn.textContent = '';
 }

 bundleSel.addEventListener('change', paint);
 rel.addEventListener('input', paint);

 var copyBtn = E('button', { 'type': 'button', 'class': 'as-button', 'click': function() {
  /* 没包的时候该复制的是下载那条，不是「先下包」这句提示 */
  var t = (dlBox.style.display !== 'none' && dlBox.textContent) ? dlBox.textContent : (cmd.textContent || '');
  if (!t) return;
  if (navigator.clipboard && navigator.clipboard.writeText) {
   navigator.clipboard.writeText(t).then(function() { copyFeed.textContent = '已复制，去 SSH 里粘。'; })
    .catch(function() { copyFeed.textContent = '复制被浏览器拦了，手动选中那行命令复制。'; });
  } else {
   copyFeed.textContent = '这个浏览器不让自动复制，手动选中那行命令复制。';
  }
 } }, '复制命令');

 setTimeout(function() {
  kbList().then(function(r) {
   var d = r && r[0] ? r[0] : r;
   kernRow.textContent = d.kernel || '—';
   instRow.textContent = (d.installed ? 'boot 里记的是 ' + d.installed : 'boot 里没记（' + (d.kernel_short || '?') + '）')
    + (d.installed_tag ? '，tag ' + d.installed_tag : '');
   if (d.installed_tag && !rel.value) rel.value = d.installed_tag;
   allBundles = d.bundles || [];
   /* 默认目标跟着 boot 里记的那一档走：本机跑的就是 6.18，一上来指着 5.4 等于给错方向 */
   var short = d.kernel_short || '';
   if (short && KERNEL_CHOICES.indexOf(short) >= 0) {
    pick = short;
    Array.prototype.forEach.call(group.children, function(x) {
     x.classList.toggle('is-on', x.dataset.k === pick);
     x.setAttribute('aria-pressed', x.dataset.k === pick ? 'true' : 'false');
    });
   }
   refill(pick);
  }).catch(function() { kernRow.textContent = '读不到（主包较旧）'; });
 }, 0);

 return card('切换内核', [
  E('p', { 'class': 'as-copy' }, '主包不绑内核，内核是独立一档装进 boot 分区的。现在支持三档：5.4（厂商默认）、6.18、7.3。切完必须重启才生效。'),
  E('div', { 'class': 'as-row' }, [E('dt', {}, '当前内核'), kernRow]),
  E('div', { 'class': 'as-row' }, [E('dt', {}, 'boot 记录'), instRow]),
  E('div', { 'class': 'as-field-label' }, '目标内核'),
  group,
  ui.field('上游 release 号', rel),
  ui.field('本机现成的内核包', bundleSel),
  dlBox,
  cmd,
  warn,
  E('div', { 'class': 'as-buttons' }, [copyBtn]),
  tip,
  copyFeed,
  E('p', { 'class': 'as-caption' }, '包不在机器上时先从上游下载到 /mnt/mu300-disk/pkg/ 或 /tmp，再回来选。切换命令要以 root 在设备上跑。')
 ], true);
}

/* ---------------- AGDSP 主固件（自备说明） ---------------- */
function fwCard() {
 var statusEl = E('p', { 'class': 'as-caption' }, '正在读取本机固件状态……');
 var urlBox = E('p', { 'class': 'as-technical' }, FW_URL);
 var pathBox = E('p', { 'class': 'as-technical' }, FW_PATH);
 var feed = E('p', { 'class': 'as-caption', 'role': 'status', 'aria-live': 'polite' }, '');

 function copyText(text, okMsg) {
  if (!text) return;
  if (navigator.clipboard && navigator.clipboard.writeText) {
   navigator.clipboard.writeText(text).then(function() { feed.textContent = okMsg; })
    .catch(function() { feed.textContent = '复制被浏览器拦了，手动选中那行复制。'; });
  } else {
   feed.textContent = '这个浏览器不让自动复制，手动选中那行复制。';
  }
 }

 var copyUrlBtn = E('button', { 'type': 'button', 'class': 'as-button', 'click': function() { copyText(FW_URL, '下载链接已复制，去浏览器或 SSH 里粘。'); } }, '复制下载链接');
 var copyPathBtn = E('button', { 'type': 'button', 'class': 'as-button', 'click': function() { copyText(FW_PATH, '落地路径已复制。'); } }, '复制落地路径');

 setTimeout(function() {
  audProbe().then(function(r) {
   var info = r && r[0] ? r[0] : (r || {});
   if (info.fw_present === true) {
    statusEl.className = 'as-caption as-mini-green';
    statusEl.textContent = '本机已有这个固件：' + FW_PATH;
   } else {
    statusEl.className = 'as-caption as-mini-red';
    statusEl.textContent = '本机没有这个固件；先从下面链接取一份落到设备上，通话才有声。';
   }
  }).catch(function() { statusEl.textContent = '本机固件状态读不到（主包较旧）。'; });
 }, 0);

 return card('AGDSP 主固件（语音 DSP）', [
  E('p', { 'class': 'as-copy' }, '这是语音 DSP 跑起来必需的专有固件，文件名叫 agdsp.bin。'),
  statusEl,
  E('div', { 'class': 'as-field-label' }, '从哪里获取（上游作者仓库 v1.0 release）'),
  urlBox,
  E('div', { 'class': 'as-field-label' }, '放到设备哪个目录'),
  pathBox,
  E('div', { 'class': 'as-buttons' }, [copyUrlBtn, copyPathBtn]),
  feed,
  E('p', { 'class': 'as-caption' }, '为什么咱不带：它是 Unisoc/展锐专有镜像，上游作者只做了搬运、没有给出再分发许可，所以配件包、主包和公开仓库都不内置这份文件，只能你自己从上面链接取。'),
  E('p', { 'class': 'as-caption' }, '缺了它会怎样：驱动和声卡能起，但 DSP 不上电，双向通话无声。落位后由 mu300-audio-dsp 校验大小、AUDCP 头和 SHA-256，已有固件不覆盖。')
 ], true);
}

/* ---------------- DSP 通话参数 profiles（自备说明） ---------------- */
var PROFILE_NAMES = ['audio_structure', 'dsp_vbc', 'cvs', 'dsp_smartamp'];

function profilesCard() {
 var statusEl = E('p', { 'class': 'as-caption' }, '正在读取本机参数状态……');
 var listBox = E('p', { 'class': 'as-technical' }, PROFILE_NAMES.join(' · '));

 setTimeout(function() {
  audProbe().then(function(r) {
   var info = r && r[0] ? r[0] : (r || {});
   if (info.profiles_present === true) {
    statusEl.className = 'as-caption as-mini-green';
    statusEl.textContent = '本机已有四个 profile 参数（/lib/firmware/）。';
   } else {
    statusEl.className = 'as-caption as-mini-red';
    statusEl.textContent = '本机缺：' + (info.profiles_missing || '部分 profile 参数') + '；不影响先打通电话。';
   }
  }).catch(function() { statusEl.textContent = '本机参数状态读不到（主包较旧）。'; });
 }, 0);

 return card('DSP 通话参数（profiles）', [
  E('p', { 'class': 'as-copy' }, '四个 blob 是 DSP 通话场景参数：音频结构、VBC 语音处理、CVS 后处理、智能功放。'),
  statusEl,
  E('div', { 'class': 'as-field-label' }, '文件名（落在 /lib/firmware/）'),
  listBox,
  E('p', { 'class': 'as-caption' }, '没有这四个也能正常打电话：缺省时通话走 legacy_pcm 直连兜底通路，不挡打通；装上后 DSP 才按通话场景套用路由、增益、降噪回声消除和智能功放参数。6.18.60 已实测装入后正常通话、双向有声。'),
  E('p', { 'class': 'as-caption' }, '为什么咱不带：这些是 Unisoc/展锐专有参数，未提供再分发许可，配件包、主包和公开仓库都不内置，需自备（从同 SoC Android 的 audio_params XML 编译）。')
 ], true);
}

/* ---------------- 说明与自备项（不阻塞直连通话） ---------------- */
function knownIssuesCard() {
 return card('说明与自备项', [
  E('p', { 'class': 'as-copy' }, '下面几条不挡 legacy_pcm 直连通话，先记下。'),
  E('ul', { 'class': 'as-instructions' }, [
   E('li', {}, 'AGDSP 主固件 agdsp.bin 属专有镜像，配件包不随包分发，需按上方「AGDSP 主固件」提示自备；固件不到位时 DSP 不上电、通话无声。'),
   E('li', {}, 'DSP profiles 四个参数 blob 已自备装入并实测通话通过；缺省仍可走直连兜底打通电话，详见上方「DSP 通话参数」卡片。')
  ])
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
   E('div', { 'class': 'as-layout' }, [optCard(lc), selfCheckCard(), fwCard(), profilesCard(), kernelCard(), knownIssuesCard()])
  ]);
 },
 handleSaveApply: null,
 handleSave: null,
 handleReset: null
});
