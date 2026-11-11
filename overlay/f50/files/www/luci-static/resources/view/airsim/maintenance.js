'use strict';
'require view';
'require poll';
'require rpc';
'require view.airsim.ui as ui';
'require view.airsim.glass as glass';

var getStatus = rpc.declare({ object: 'airsim', method: 'status', expect: {}, reject: true });
var getDiagnostics = rpc.declare({ object: 'airsim', method: 'diagnostics', expect: {}, reject: true });
var getUpdates = rpc.declare({ object: 'airsim', method: 'updates', params: ['previews'], expect: {}, reject: true, nobatch: true });
var updateStatus = rpc.declare({ object: 'airsim', method: 'updates_status', expect: {}, reject: true });
var updatePolicy = rpc.declare({ object: 'airsim', method: 'updates_policy', params: ['config'], expect: {}, reject: true, nobatch: true });
var installUpdate = rpc.declare({ object: 'airsim', method: 'update_install', params: ['tag'], expect: {}, reject: true, nobatch: true });
var backupMake = rpc.declare({ object: 'airsim-backup', method: 'make', expect: {}, reject: true, nobatch: true });
var backupRestore = rpc.declare({ object: 'airsim-backup', method: 'restore', params: ['payload'], expect: {}, reject: true, nobatch: true });
var btProbe = rpc.declare({ object: 'airsim-bt', method: 'bt_opt_probe', expect: {}, reject: true });
var btInstall = rpc.declare({ object: 'airsim-bt', method: 'bt_opt_install', expect: {}, reject: true, nobatch: true });
var audioProbe = rpc.declare({ object: 'airsim-audio', method: 'audio_opt_probe', expect: {}, reject: true });
var audioInstall = rpc.declare({ object: 'airsim-audio', method: 'audio_opt_install', expect: {}, reject: true, nobatch: true });
var btLatest = rpc.declare({ object: 'airsim-bt', method: 'bt_opt_latest', params: ['force'], expect: {}, reject: true, nobatch: true });
var audioLatest = rpc.declare({ object: 'airsim-audio', method: 'audio_opt_latest', params: ['force'], expect: {}, reject: true, nobatch: true });
var flowCheck = rpc.declare({ object: 'airsim-net', method: 'flowfix_check', expect: {}, reject: true, nobatch: true });
var flowFix = rpc.declare({ object: 'airsim-net', method: 'flowfix_fix', expect: {}, reject: true, nobatch: true });

var releasesURL = 'https://github.com/shangjie1998/airsim-f50-openwrt/releases';

function base64ToBlob(b64, type) {
 var bin = atob(b64.replace(/\s+/g, '')), arr = new Uint8Array(bin.length);
 for (var i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i);
 return new Blob([arr], { type: type });
}

function readBase64(file) {
 return new Promise(function(resolve, reject) {
  var reader = new FileReader();
  reader.onload = function() { var s = String(reader.result || ''); var i = s.indexOf(','); resolve(i >= 0 ? s.slice(i + 1) : s); };
  reader.onerror = function() { reject(Error('读取文件失败')); };
  reader.readAsDataURL(file);
 });
}

function noticeBox() {
 return E('div', { 'class': 'as-feedback', hidden: true, 'role': 'status', 'aria-live': 'polite' }, [E('strong', {}, ''), E('p', {}, '')]);
}

function setNotice(box, title, text, kind) {
 if (!text) { box.hidden = true; return; }
 box.hidden = false;
 box.className = 'as-feedback as-feedback-' + (kind || 'info');
 box.firstChild.textContent = title || '';
 box.lastChild.textContent = text;
}

return view.extend({
 load: function() {
  return Promise.all([getStatus().catch(function() { return {}; }), updateStatus().catch(function() { return {}; })]);
 },
 render: function(data) {
  ui.install();
  var state = data[0] || {}, updateInfo = data[1] || {};
  var pending = false, busyFlag = false, restoreFile = null;

  /* ---------------- 备份与恢复 ---------------- */
  var backupNote = noticeBox();
  var fileInput = E('input', { 'type': 'file', 'accept': '.tar.gz,.tgz,application/gzip,application/x-gzip', hidden: true });
  var pickedName = E('p', { 'class': 'as-caption' }, '尚未选择文件。');
  var pickButton = E('button', { 'type': 'button', 'class': 'as-button as-secondary' }, '选择备份文件');
  var restoreConfirm = E('button', { 'type': 'button', 'class': 'as-button as-danger' }, '确认覆盖');
  var restoreCancel = E('button', { 'type': 'button', 'class': 'as-button as-secondary' }, '取消');
  var restorePanel = E('div', { 'class': 'as-feedback as-feedback-error', hidden: true }, [
   E('strong', {}, '确认用这份备份覆盖本机数据？'),
   E('p', {}, '会覆盖当前插件配置、配对凭据与运行状态，并重启电话服务。覆盖前会把现在的状态另存到 /tmp/airsim-before-restore-*。'),
   E('div', { 'class': 'as-buttons' }, [restoreConfirm, restoreCancel])
  ]);
  var restoreBtn = E('button', { 'type': 'button', 'class': 'as-button as-primary' }, '恢复这份备份');
  var backupBtn = E('button', { 'type': 'button', 'class': 'as-button as-primary' }, '备份到电脑');
  var dropZone = E('div', { 'class': 'as-backup-drop', 'tabindex': '0' }, [
   E('strong', {}, '把备份文件拖到这里'),
   E('span', {}, '或点「选择备份文件」。只接受本插件导出、且属于这台机器的 .tar.gz')
  ]);

  function backupMessage(text, kind) { setNotice(backupNote, kind === 'error' ? '这一步没成' : '备份与恢复', text, kind); }
  function syncBackupButtons() {
   backupBtn.disabled = busyFlag;
   restoreBtn.disabled = busyFlag || !restoreFile;
   restoreConfirm.disabled = busyFlag || !restoreFile;
   pickButton.disabled = busyFlag;
  }

  function takeFile(file) {
   if (busyFlag) return;
   if (!/\.(tar\.gz|tgz|gz)$/i.test(file.name || '')) { backupMessage('这不是 .tar.gz 备份包，请重新选择。', 'error'); return; }
   if (file.size > 8388608) { backupMessage('文件超过 8 MB，不像是本插件的备份。', 'error'); return; }
   restoreFile = file;
   pickedName.textContent = '已选择：' + file.name + '（' + Math.max(1, Math.round(file.size / 1024)) + ' KB）';
   restorePanel.hidden = true;
   backupMessage('文件已读入，点「恢复这份备份」后还会再确认一次。', 'info');
   syncBackupButtons();
  }

  dropZone.addEventListener('dragover', function(ev) { ev.preventDefault(); dropZone.className = 'as-backup-drop is-active'; });
  dropZone.addEventListener('dragleave', function() { dropZone.className = 'as-backup-drop'; });
  dropZone.addEventListener('drop', function(ev) {
   ev.preventDefault(); dropZone.className = 'as-backup-drop';
   var files = (ev.dataTransfer && ev.dataTransfer.files) || [];
   if (files.length) takeFile(files[0]);
  });
  dropZone.addEventListener('click', function() { if (!busyFlag) fileInput.click(); });
  dropZone.addEventListener('keydown', function(ev) { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); if (!busyFlag) fileInput.click(); } });
  pickButton.addEventListener('click', function() { if (!busyFlag) fileInput.click(); });
  fileInput.addEventListener('change', function() { if (fileInput.files && fileInput.files.length) takeFile(fileInput.files[0]); });

  backupBtn.addEventListener('click', async function() {
   if (busyFlag) return;
   busyFlag = true; syncBackupButtons(); backupMessage('正在打包配置与配对资料…', 'pending');
   try {
    var result = await backupMake();
    if (!result || result.ok === false || !result.data) throw Error((result && result.error) || '打包失败');
    var blob = base64ToBlob(result.data, 'application/gzip');
    var url = URL.createObjectURL(blob);
    var link = E('a', { href: url, download: result.filename || 'airsim-f50-backup.tar.gz' });
    document.body.appendChild(link); link.click(); link.remove();
    setTimeout(function() { URL.revokeObjectURL(url); }, 1500);
    backupMessage('已交给浏览器保存（' + (result.size ? Math.round(result.size / 1024) + ' KB' : '大小未知') + '，本机标识 ' + (result.short_id || '--') + '）。恢复时校验的就是这个标识。', 'success');
   } catch (e) { backupMessage(e.message || '备份失败，请重试。', 'error'); }
   finally { busyFlag = false; syncBackupButtons(); }
  });

  restoreBtn.addEventListener('click', function() {
   if (!restoreFile || busyFlag) return;
   restorePanel.hidden = false;
   backupMessage('已读取文件，请确认覆盖。', 'info');
  });
  restoreCancel.addEventListener('click', function() { restorePanel.hidden = true; });
  restoreConfirm.addEventListener('click', async function() {
   if (busyFlag || !restoreFile) return;
   restorePanel.hidden = true;
   busyFlag = true; syncBackupButtons(); backupMessage('正在导入备份并重启服务…', 'pending');
   try {
    var payload = await readBase64(restoreFile);
    var result = await backupRestore(payload);
    if (!result || result.ok === false) throw Error((result && result.error) || '恢复没确认，请刷新后核对状态');
    backupMessage(result.message || '备份已恢复，服务正在重启。', 'success');
    restoreFile = null; pickedName.textContent = '尚未选择文件。';
   } catch (e) { backupMessage(e.message || '恢复失败，请重试。', 'error'); }
   finally { busyFlag = false; syncBackupButtons(); }
  });
  syncBackupButtons();

  /* ---------------- 诊断 ---------------- */
  var diagNote = noticeBox();
  var diagBtn = E('button', { 'type': 'button', 'class': 'as-button as-secondary', click: async function() {
   if (pending) return;
   pending = true; ui.busy(diagBtn, true); setNotice(diagNote, '诊断', '正在生成脱敏状态报告…', 'pending');
   try {
    var result = await getDiagnostics();
    if (!result.ok || !result.report || result.error) throw Error(result.error || '诊断生成失败');
    var blob = new Blob([JSON.stringify(result.report, null, 2) + '\n'], { type: 'application/json;charset=utf-8' });
    var url = URL.createObjectURL(blob);
    var link = E('a', { href: url, download: 'airsim-f50-diagnostics.json' });
    document.body.appendChild(link); link.click(); link.remove();
    setTimeout(function() { URL.revokeObjectURL(url); }, 1000);
    setNotice(diagNote, '诊断', '脱敏报告已交给浏览器下载，仅含状态白名单，不含原始日志。', 'success');
   } catch (e) { setNotice(diagNote, '诊断', e.message || '导出失败，请重试。', 'error'); }
   finally { pending = false; ui.busy(diagBtn, false); }
  } }, '导出脱敏诊断');

  /* ---------------- 插件更新 ---------------- */
  var updateNote = noticeBox();
  var jobNote = E('p', { 'class': 'as-caption' }, '');
  var updateResult = E('p', { 'class': 'as-copy' }, '正在读取自动检查结果…');
  var confirmedTag = '', latest = '', updateKnown = !!updateInfo.ok;
  var automatic = E('input', { 'type': 'checkbox', 'id': 'as-auto-update' });
  automatic.checked = updateInfo.policy ? !!updateInfo.policy.automatic : true;
  var previews = E('input', { 'type': 'checkbox', 'checked': true, 'id': 'as-previews' });
  if (updateInfo.policy) previews.checked = !!updateInfo.policy.previews;
  var updateLink = E('a', { 'href': releasesURL, 'target': '_blank', 'rel': 'noopener noreferrer', 'class': 'as-text-button' }, '打开项目 Release');
  var installBtn = E('button', { 'type': 'button', 'class': 'as-button as-primary' }, '下载并更新');
  var confirmBtn = E('button', { 'type': 'button', 'class': 'as-button as-primary' }, '确认更新');
  var cancelUpdate = E('button', { 'type': 'button', 'class': 'as-button as-secondary' }, '取消');
  var installPanel = E('div', { 'class': 'as-feedback as-feedback-info', hidden: true }, [
   E('p', {}, ''),
   E('div', { 'class': 'as-buttons' }, [confirmBtn, cancelUpdate])
  ]);

  function editable() { return state.service_alive === false && state.running === false; }
  function syncUpdateButtons() {
   installBtn.disabled = pending || !updateKnown || !!updateInfo.busy || !editable() || !latest;
   confirmBtn.disabled = installBtn.disabled || confirmedTag !== latest;
   automatic.disabled = previews.disabled = pending || !!updateInfo.busy;
  }

  function showRelease(result) {
   var valid = /^v\d+\.\d+\.\d+\.\d+-r\d+-6\.18$/.test(result.latest || '');
   // F50 的主包是 tar.gz 覆盖包：后端已改成「下载 → 核 sha256 → 解包 → 跑 install.sh」，
   // 所以这里可以放心把在线安装亮出来。
   latest = result.status === 'available' && valid ? result.latest : '';
   var messages = {
    available: '发现新版本', current: '当前已是此频道最新版本', ahead: '当前构建领先于公开 Release',
    unknown_version: '当前开发版本无法自动比较',
    no_release: '这个频道暂时没有可用的整包；可以先去「打开项目 Release」看一眼'
   };
   updateResult.textContent = (messages[result.status] || '版本状态未知') + (valid ? ' · ' + result.latest : '') + (result.prerelease ? '（开发预览）' : '') + (result.checked_at ? ' · 检查于 ' + new Date(result.checked_at).toLocaleString() : '');
   updateLink.href = valid ? releasesURL + '/tag/' + result.latest : releasesURL;
   syncUpdateButtons();
  }

  function showUpdateStatus(info) {
   updateKnown = !!info.ok; updateInfo = info;
   if (!updateKnown) { latest = ''; jobNote.textContent = '更新状态暂不可用，等待重新连接；不会自动重试安装。'; syncUpdateButtons(); return; }
   var cache = info.cache || {};
   if (cache.previews === !!previews.checked && cache.result) showRelease(cache.result);
   else { latest = ''; updateResult.textContent = cache.error || '尚无当前频道的检查结果，可点击检查更新。'; }
   var job = info.job || {};
   jobNote.textContent = job.message || '';
   if (job.phase === 'succeeded' && job.target === latest) latest = '';
   syncUpdateButtons();
  }

  async function savePolicy() {
   if (pending) return;
   pending = true; latest = ''; syncUpdateButtons();
   try {
    var info = await updatePolicy(JSON.stringify({ automatic: !!automatic.checked, previews: !!previews.checked }));
    if (!info.ok || info.error) throw Error(info.error || '保存更新偏好失败');
    showUpdateStatus(info);
   } catch (e) { updateKnown = false; jobNote.textContent = e.message || '保存失败'; }
   finally { pending = false; syncUpdateButtons(); }
  }
  automatic.addEventListener('change', savePolicy);
  previews.addEventListener('change', savePolicy);

  var checkBtn = E('button', { 'type': 'button', 'class': 'as-button as-primary', click: async function() {
   if (pending) return;
   pending = true; latest = ''; syncUpdateButtons(); updateResult.textContent = '正在检查项目 GitHub Release…';
   updateLink.href = releasesURL;
   try {
    var result = await getUpdates(!!previews.checked);
    if (!result.ok || result.error) throw Error(result.error || '检查失败');
    showRelease(result);
   } catch (e) { updateResult.textContent = e.message || '检查失败，请稍后重试。'; }
   finally { pending = false; syncUpdateButtons(); }
  } }, '检查更新');

  installBtn.addEventListener('click', function() {
   if (installBtn.disabled) return;
   confirmedTag = latest;
   installPanel.hidden = false;
   installPanel.firstChild.textContent = '将更新到 ' + confirmedTag + '。保留设置与配对资料；更新期间请勿断电，完成后需手动启动电话服务。';
   syncUpdateButtons();
  });
  cancelUpdate.addEventListener('click', function() { installPanel.hidden = true; confirmedTag = ''; syncUpdateButtons(); });
  confirmBtn.addEventListener('click', async function() {
   if (confirmBtn.disabled || pending) return;
   pending = true; syncUpdateButtons(); installPanel.hidden = true;
   try {
    var result = await installUpdate(confirmedTag);
    if (!result.ok || result.error) throw Error(result.error || '升级没确认，请等状态刷新');
    updateInfo.busy = true; jobNote.textContent = result.job.message;
   } catch (e) { updateKnown = false; jobNote.textContent = e.message || '连接中断，请等状态刷新，别重复安装。'; }
   finally { pending = false; confirmedTag = ''; syncUpdateButtons(); }
  });

  showUpdateStatus(updateInfo);

  /* 备份、恢复分两张卡：一件事一张卡，宽屏并排，窄屏上下 */
  var backupCard = E('section', { 'class': 'as-card' }, [
   E('h3', {}, '备份'),
   E('p', { 'class': 'as-copy as-lead' }, '把插件配置、配对凭据和运行状态打成一个 .tar.gz 存到电脑。'),
   E('p', { 'class': 'as-caption' }, '备份包按本机硬件指纹标记，日志、音频、短信正文都不进包。'),
   E('div', { 'class': 'as-buttons' }, [backupBtn]),
   backupNote
  ]);

  var restoreCard = E('section', { 'class': 'as-card' }, [
   E('h3', {}, '恢复'),
   E('p', { 'class': 'as-copy as-lead' }, '换固件、刷机、折腾完再导回来；先校验指纹，别人的备份会被直接拒绝。'),
   dropZone, fileInput, pickButton, pickedName,
   E('div', { 'class': 'as-buttons' }, [restoreBtn]),
   restorePanel,
   E('p', { 'class': 'as-caption as-lead' }, '恢复会覆盖当前配置与配对资料并重启服务，覆盖前自动把现在的状态存到 /tmp/airsim-before-restore-*。')
  ]);

  var updateCard = E('section', { 'class': 'as-card' }, [
   E('h3', {}, '插件更新'),
   E('label', { 'class': 'as-check', 'for': 'as-auto-update' }, [automatic, E('span', {}, '自动检查更新（每 6 小时）')]),
   E('label', { 'class': 'as-check', 'for': 'as-previews' }, [previews, E('span', {}, '包含开发预览版')]),
   E('p', { 'class': 'as-caption' }, '自动检查只取公开版本信息，不自动安装。要先在概览页关掉电话服务再更新。'),
   E('div', { 'class': 'as-buttons' }, [checkBtn]),
   updateResult, jobNote, updateLink,
   E('p', { 'class': 'as-caption' }, 'F50 是自编整包，更新源指本项目：发现新版本时可以在这里一键装（先关电话服务，装完重启服务）。装的是固定 tag 的整包，装前核对 sha256。')
  ]);

  var diagCard = E('section', { 'class': 'as-card' }, [
   E('h3', {}, '诊断导出'),
   E('p', { 'class': 'as-copy as-lead' }, '导出一份可分享的状态报告，出问题时直接发给维护的人。'),
   E('p', { 'class': 'as-caption' }, '不读取短信正文，不导出号码、IP、Relay 地址、标识、配对码、密钥、原始日志或音频；不会自动上传。'),
   E('div', { 'class': 'as-buttons' }, [diagBtn]),
   diagNote
  ]);


  /* 检查更新小部件在 ui.js（ui.latestChecker / ui.stampEpoch），两个页面共用一套。 */

  /* ---------------- 可选配件：蓝牙 ---------------- */
  var optNote = noticeBox();
  var optChip = E('span', { 'class': 'as-pill as-pill-neutral' }, '检查中');
  var optMsg = E('p', { 'class': 'as-caption' }, '正在问本机装没装。');
  var optBtn = E('button', { 'type': 'button', 'class': 'as-button', 'disabled': true }, '安装蓝牙');
  var optBusy = false;
  var btLc = ui.latestChecker({ opt: 'bt-opt', current: '', installLabel: '重新安装蓝牙', check: btLatest });

  function syncOpt(info) {
   info = info || {};
   var installed = info.installed === true, busy = info.busy === true;
   optBusy = busy;
   btLc.setCurrent(info.ver || (info.version || '').replace(/^bt-opt-v/, ''));
   optChip.textContent = busy ? '正在装' : (installed ? '已安装' : '未安装');
   optChip.className = 'as-pill ' + (installed || busy ? 'as-pill-green' : 'as-pill-neutral');
   optBtn.textContent = installed ? '重新安装蓝牙' : '安装蓝牙';

   if (busy) {
    optBtn.disabled = true;
    optMsg.textContent = info.message || '正在装，别断电。';
    return;
   }
   optBtn.disabled = info.kernel_ok !== true;
   if (info.kernel_ok !== true) {
    optMsg.textContent = '本机内核 ' + (info.kernel || '未知') + '，可选件是给 ' + (info.want_kernel || '—') + ' 打的，对不上装不上（驱动模块必须跟内核一致）。';
   }
   else if (info.state === 'done') optMsg.textContent = info.message || '装好了。';
   else if (info.state === 'failed') {
    optMsg.textContent = info.message || '这次没成。';
    setNotice(optNote, '安装没成', info.message || '看下面的输出再试一次。', 'error');
   }
   else if (installed) optMsg.textContent = '这台机器已经有蓝牙那一套了；页面出过问题可以重装一遍，不动已配对记录。';
   else optMsg.textContent = '主包不带蓝牙驱动与 bluez。需要时从这里装回来：约 1.7MB，从项目 Release（' + (info.version || 'bt-opt') + '）拉，装前核对 sha256。';
   if (optBtn.disabled) optBtn.title = '内核或控制器条件不满足';
   else optBtn.title = '从 GitHub 拉可选件并安装';
  }

  optBtn.addEventListener('click', function() {
   if (optBusy) return;
   optBtn.disabled = true;
   optMsg.textContent = '正在启动安装……';
   btInstall().then(function(res) {
    res = res || {};
    if (res.ok !== true) {
     optBtn.disabled = false;
     setNotice(optNote, '没启动', res.error || '启动失败', 'error');
     return;
    }
    setNotice(optNote, '开始装了', '下载 → 校验 → 解压 → 装驱动，几十秒。页面会自动刷新进度。', 'info');
    optMsg.textContent = '正在下载……';
   }).catch(function(e) {
    optBtn.disabled = false;
    setNotice(optNote, '没启动', String(e && e.message || e), 'error');
   });
  });


  /* ---------------- 可选配件：6.18 音频模块 ---------------- */
  var audNote = noticeBox();
  var audChip = E('span', { 'class': 'as-pill as-pill-neutral' }, '检查中');
  var audMsg = E('p', { 'class': 'as-caption' }, '正在问本机音频模块齐不齐。');
  var audBtn = E('button', { 'type': 'button', 'class': 'as-button', 'disabled': true }, '安装音频模块');
  var audBusy = false;
  var audLc = ui.latestChecker({ opt: 'audio-opt', current: '', installLabel: '安装音频模块', check: audioLatest });

  function syncAud(info) {
   info = info || {};
   var installed = info.installed === true, busy = info.busy === true;
   audLc.setCurrent(info.ver || (info.version || '').replace(/^audio-opt-v/, ''));
   var matched = info.matched === true, kOk = info.kernel_ok === true, extOk = info.ext_present === true;
   var kern = info.kernel || '未知', avail = String(info.available || '').replace(/\s+/g, ' ').trim();
   audBusy = busy;
   audChip.textContent = busy ? '正在装' : (installed ? '已安装' : (matched ? ((info.ko_count || 0) + '/' + (info.want_ko || 25)) : '内核不匹配'));
   audChip.className = 'as-pill ' + (busy ? 'as-pill-neutral' : (installed ? 'as-pill-green' : (matched ? 'as-pill-neutral' : 'as-pill-red')));
   audBtn.textContent = installed ? '重新安装音频模块' : '安装音频模块';

   if (busy) {
    audBtn.disabled = true;
    audMsg.textContent = info.message || '正在装，别断电。';
    return;
   }
   audBtn.disabled = !(extOk && (matched || kOk));
   if (!extOk) {
    audMsg.textContent = '本机没有厂商音频扩展目录（/mnt/mu300-disk/extra/airsim-audio），先有那一套才能补模块。';
   }
   else if (!matched) {
    audMsg.textContent = '本机内核 ' + kern + '，扩展目录里现有的驱动是 ' + (avail ? avail.replace(/ /g, '、') : '（空）') + '，没有与当前内核 vermagic 对齐的一套：语音不会启动（服务、短信、来电通知照常）。等 ' + kern + ' 的驱动补进来会自动恢复，或者把系统切回上面任意一套内核。';
   }
   else if (info.state === 'failed') {
    audMsg.textContent = info.message || '这次没成。';
    setNotice(audNote, '安装没成', info.message || '看下面的输出再试一次。', 'error');
   }
   else if (installed) {
    audMsg.textContent = kern + ' 的模块 ' + (info.ko_count || 0) + '/' + (info.want_ko || 25) + ' 齐了' + (info.opt_version ? '（可选件 ' + info.opt_version + '）' : '') + '；页面出问题可以重装一遍，旧目录会先备份。';
   }
   else {
    var lack = info.ko_missing || 0;
    audMsg.textContent = kern + ' 的音频驱动缺 ' + lack + ' 个（现 ' + (info.ko_count || 0) + '/' + (info.want_ko || 25) + '）。从这里补：约 420KB，走固定 tag ' + (info.version || 'audio-opt') + '，装前核对 sha256。';
   }
   if (matched && extOk && info.dsp_present === false) audMsg.textContent += ' 注意：本机没有 mu300-audio-dsp，光补模块也放不出声。';
   audBtn.title = audBtn.disabled ? ('没有可用于 ' + kern + ' 的驱动包（可选件里是 ' + (info.opt_kernels || '—') + '）') : '从 GitHub 拉可选件并安装';
  }

  audBtn.addEventListener('click', function() {
   if (audBusy) return;
   audBtn.disabled = true;
   audMsg.textContent = '正在启动安装……';
   audioInstall().then(function(res) {
    res = res || {};
    if (res.ok !== true) {
     audBtn.disabled = false;
     setNotice(audNote, '没启动', res.error || '启动失败', 'error');
     return;
    }
    setNotice(audNote, '开始装了', '下载 → 核 sha256 → 解包 → 放模块，几秒到几十秒。页面会自动刷新进度。', 'info');
    audMsg.textContent = '正在下载……';
   }).catch(function(e) {
    audBtn.disabled = false;
    setNotice(audNote, '没启动', String(e && e.message || e), 'error');
   });
  });

  /* 刷机后自检挪到「音频」独立页（admin/services/airsim/audio）。 */

  var optCard = E('section', { 'class': 'as-card' }, [
   E('h3', {}, '可选配件'),
   E('p', { 'class': 'as-copy' }, '这两样都不在主包里：刷机、升级之后如果对应页面没了或者放不出声，就是这一套缺了，从这里补回来。'),
   E('div', { 'class': 'as-opt' }, [
    E('div', { 'class': 'as-card-head' }, [
     E('h4', {}, '蓝牙配件包'),
     E('div', { 'class': 'as-head-meta' }, [btLc.ver, btLc.btn])
    ]),
    btLc.mini,
    E('div', { 'class': 'as-row' }, [E('dt', {}, '状态'), E('dd', {}, [optChip])]),
    optMsg,
    E('div', { 'class': 'as-buttons' }, [optBtn]),
    optNote
   ]),
   E('div', { 'class': 'as-opt' }, [
    E('div', { 'class': 'as-card-head' }, [
     E('h4', {}, '音频模块（可选配件）'),
     E('div', { 'class': 'as-head-meta' }, [audLc.ver, audLc.btn])
    ]),
    audLc.mini,
    E('div', { 'class': 'as-row' }, [E('dt', {}, '状态'), E('dd', {}, [audChip])]),
    audMsg,
    E('div', { 'class': 'as-buttons' }, [audBtn]),
    audNote
   ]),
   E('p', { 'class': 'as-caption' }, '只装可选件本身：蓝牙不动开关状态（/etc/config/f50-bt）和已配对记录；音频只补 6.18.60 的 .ko，不动 DSP 以外的程序。两者都走固定 tag，内核对不上会直接拒绝，不会装半截。')
  ]);

  /* 5.4 专用：连热点的客户端"无互联网"体检与修复（脚本 /usr/libexec/airsim-flowfix） */
  var flowOut = E('pre', { 'class': 'as-out' }, '还没体检。');
  var flowMsg = E('p', { 'class': 'as-caption' }, '只在 5.4 厂商内核上出现这个问题；6.18 / 7.3 上点了会提示不需要。');
  var flowBusy = false;

  function renderFlow(r) {
   r = r || {};
   flowOut.textContent = r.output || '（没有输出）';
   var res = r.result || '';
   if (res === 'ok') { flowMsg.className = 'as-caption as-mini-green'; flowMsg.textContent = '当前没这个问题，不用动。'; }
   else if (res === 'need_fix') { flowMsg.className = 'as-caption as-mini-red'; flowMsg.textContent = '已经坏了：点「修复」会把软件流卸载关掉并重启防火墙。'; }
   else if (res === 'suggest') { flowMsg.className = 'as-caption'; flowMsg.textContent = '现在还没坏，但开关是开的——建议先点「修复」预防，切到流量模式就不会踩雷。'; }
   else if (res === 'fixed') { flowMsg.className = 'as-caption as-mini-green'; flowMsg.textContent = '已经修好。客户端断开重连一次热点即可。'; }
   else if (res === 'skip') { flowMsg.className = 'as-caption'; flowMsg.textContent = '这个内核没有这个问题，不用修。'; }
   else { flowMsg.className = 'as-caption as-mini-red'; flowMsg.textContent = '执行失败，看下面的输出。'; }
  }

  function runFlow(fn) {
   if (flowBusy) return;
   flowBusy = true;
   flowCheckBtn.disabled = flowFixBtn.disabled = true;
   flowOut.textContent = '执行中…';
   fn().then(function(r) { renderFlow(r[0]); }).catch(function(e) {
    flowMsg.className = 'as-caption as-mini-red';
    flowMsg.textContent = '接口没反应：' + String((e && e.message) || e);
   }).then(function() { flowBusy = false; flowCheckBtn.disabled = flowFixBtn.disabled = false; });
  }

  var flowCheckBtn = E('button', { 'type': 'button', 'class': 'as-button', 'click': function() { runFlow(flowCheck); } }, '体检');
  var flowFixBtn = E('button', { 'type': 'button', 'class': 'as-button', 'click': function() { runFlow(flowFix); } }, '修复');

  var flowCard = E('section', { 'class': 'as-card' }, [
   E('h3', {}, '客户端上网体检（5.4）'),
   flowMsg,
   flowOut,
   E('div', { 'class': 'as-buttons' }, [flowCheckBtn, flowFixBtn])
  ]);

  var versionCard = E('section', { 'class': 'as-card' }, [
   E('h3', {}, '版本说明'),
   E('p', { 'class': 'as-copy', style: 'margin-top:8px' }, '当前安装：' + (state.version || '—')),
   E('ul', { 'class': 'as-instructions' }, [
    E('li', {}, '主包不绑内核：5.4.254 / 6.18.60 / 7.3.1 一条包，只剩 aarch64 架构闸。'),
    E('li', {}, '备份包按设备指纹（eMMC cid + 序列号）标记，换 TF 卡不变，别人的包装不进来。'),
    E('li', {}, '5.4 的热点客户端"无互联网"老问题：脚本 /usr/libexec/airsim-flowfix 已随主包提供，维护页有体检/修复按钮。'),
    E('li', {}, '重启后丢音频属上游内存预留限制，靠开机轮询脚本抢占，不改内核。'),
    E('li', {}, '双卡逻辑保留在页面代码里，本机只有一张卡，卡选择自动隐藏。')
   ])
  ]);

  btProbe().then(syncOpt).catch(function() { optMsg.textContent = '这台机器没有安装器接口（主包版本较旧）。'; });
  audioProbe().then(syncAud).catch(function() { audMsg.textContent = '这台机器没有音频可选件接口（主包版本较旧）。'; });

  poll.add(function() {
   var jobs = [btProbe().then(syncOpt).catch(function() { return null; }),
               audioProbe().then(syncAud).catch(function() { return null; })];
   if (pending || busyFlag) return Promise.all(jobs);
   jobs.push(getStatus().catch(function() { return {}; }).then(function(r) { state = r[0] || state; }));
   jobs.push(updateStatus().catch(function() { return {}; }).then(function(r) { showUpdateStatus(r[1] || {}); }));
   return Promise.all(jobs);
  }, 5);

  return E('div', { 'class': 'as-root' }, [
   (ui.menu ? ui.menu('maintenance') : ''),
   ui.stylesheet(),
   E('style', {}, glass.css()),
   E('header', { 'class': 'as-header' }, [
    E('div', {}, [E('div', { 'class': 'as-eyebrow' }, 'AirSIM · 维护'), E('h2', {}, '备份、更新、诊断。'), E('p', {}, '折腾完想回退的时候，这里能救回来。')]),
    E('span', { 'class': 'as-version' }, 'v' + (state.version || '—'))
   ]),
   E('div', { 'class': 'as-layout' }, [backupCard, restoreCard, updateCard, optCard, flowCard, diagCard, versionCard]),
   E('footer', { 'class': 'as-footer' }, 'AirSIM for F50 · 维护')
  ]);
 },
 handleSaveApply: null, handleSave: null, handleReset: null
});
