#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""打包前的一致性闸：把「同一个事实写在两处」的地方机械对齐，错了就不许打包。

只查「必须有唯一真值」的几项，不查业务逻辑：
  1. VERSION 格式合法、适用范围必须是 any
  2. AGENT-VERSION 必须由 VERSION 派生（0.4.7-mu300.<VERSION>）
  3. 音频配件包版本：parts/audio/AUDIO-OPT-VERSION  ==  rpcd 里 TAG/FILE 的版本
  4. 内核口径文档存在（docs/内核矩阵.md），避免有人把它删了还照旧引用
  5. 音频配件包的内核档位三处一致：下载器 OPT_KERNELS == rpcd OPT_KERNELS == parts/audio/payload 目录
  6. out/f50 里两个二进制内嵌的版本串 == VERSION / AGENT-VERSION
     （这一项防的是「VERSION 升了、二进制没重编就打包」，包发出去标签和内容对不上）

用法：python3 build/tools/check-consistency.py
退出码：0 一致；1 有冲突（make-package.sh 会据此拒绝打包）
"""
import os
import re
import subprocess
import sys

ROOT = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..'))
problems = []


def read(p):
    path = os.path.join(ROOT, p)
    if not os.path.exists(path):
        return None
    with open(path, encoding='utf-8') as f:
        return f.read().strip()


# 1) VERSION
ver = read('VERSION') or ''
m = re.match(r'^0\.1\.0\.(\d+)-r(\d\d)-(any|6\.18|5\.4)$', ver)
if not m:
    problems.append(f'VERSION 格式不对：「{ver}」，应形如 0.1.0.22-r23-any')
elif m.group(3) != 'any':
    problems.append(f'VERSION 适用范围是「{m.group(3)}」：主包已不绑内核，只能是 any')

# 2) AGENT-VERSION 派生
agent = read('AGENT-VERSION') or ''
expect_agent = f'0.4.7-mu300.{ver}'
if agent != expect_agent:
    problems.append(f'AGENT-VERSION 与 VERSION 不同步：期望「{expect_agent}」，实际「{agent}」（跑 sh build/bump-version.sh）')

# 3) 音频配件包版本：仓库真值 vs 设备侧 rpcd
aopt = read('parts/audio/AUDIO-OPT-VERSION') or ''
rpcd = read('overlay/f50/files/usr/libexec/rpcd/airsim-audio') or ''
tag = re.search(r'^TAG=audio-opt-v(\d+\.\d+\.\d+)', rpcd, re.M)
fle = re.search(r'^FILE=airsim-f50-audio-opt-(\d+\.\d+\.\d+)-', rpcd, re.M)
if not aopt:
    problems.append('parts/audio/AUDIO-OPT-VERSION 不存在或为空')
if not tag or not fle:
    problems.append('rpcd/airsim-audio 里没找到 TAG=audio-opt-vX.Y.Z 或 FILE=...-X.Y.Z-...')
else:
    if tag.group(1) != aopt:
        problems.append(f'配件包版本打架：AUDIO-OPT-VERSION={aopt}，但 rpcd TAG 指向 {tag.group(1)}')
    if fle.group(1) != aopt:
        problems.append(f'配件包版本打架：AUDIO-OPT-VERSION={aopt}，但 rpcd FILE 指向 {fle.group(1)}')

# 3b) 蓝牙配件包版本：同一个套路，仓库真值 vs 设备侧 rpcd
btopt = read('parts/bluetooth/BT-OPT-VERSION') or ''
btrpcd = read('overlay/f50/files/usr/libexec/rpcd/airsim-bt') or ''
btag = re.search(r'^BT_OPT_TAG=bt-opt-v(\d+\.\d+\.\d+)', btrpcd, re.M)
bfle = re.search(r'^BT_OPT_FILE=airsim-f50-bt-opt-(\d+\.\d+\.\d+)-', btrpcd, re.M)
# 文件名两种合法写法：静态常量 BT_OPT_FILE=...-X.Y.Z-k<krel>.tar.gz，
# 或按内核动态拼（BW 侧写的是 airsim-f50-bt-opt-$BT_OPT_VER-k$kver.tar.gz），
# 动态那种没有静态 FILE，就用 BT_OPT_VER 当版本真值来比。
bver = re.search(r'^BT_OPT_VER=(\d+\.\d+\.\d+)', btrpcd, re.M)
bfile_ver = bfle.group(1) if bfle else (bver.group(1) if bver else None)
if not btopt:
    problems.append('parts/bluetooth/BT-OPT-VERSION 不存在或为空')
if not btag or not bfile_ver:
    problems.append('rpcd/airsim-bt 里没找到 BT_OPT_TAG=bt-opt-vX.Y.Z，也没找到 BT_OPT_FILE 或 BT_OPT_VER')
else:
    if btag.group(1) != btopt:
        problems.append(f'蓝牙配件包版本打架：BT-OPT-VERSION={btopt}，但 rpcd TAG 指向 {btag.group(1)}')
    if bfile_ver != btopt:
        problems.append(f'蓝牙配件包版本打架：BT-OPT-VERSION={btopt}，但 rpcd 文件名版本是 {bfile_ver}')

# 3c) 音频件数：设备侧 WANT_KO 必须等于 module-list.txt 里的 .ko 行数
modlist = read('parts/audio/module-list.txt') or ''
want = re.search(r'^WANT_KO=(\d+)', rpcd, re.M)
if not modlist:
    problems.append('parts/audio/module-list.txt 不存在')
elif not want:
    problems.append('rpcd/airsim-audio 里没找到 WANT_KO=NN')
else:
    real = sum(1 for ln in modlist.splitlines() if ln.strip().endswith('.ko'))
    if int(want.group(1)) != real:
        problems.append(f'音频件数打架：rpcd WANT_KO={want.group(1)}，但 module-list.txt 有 {real} 个 .ko')

# 4) 内核口径单一来源必须在
if not os.path.exists(os.path.join(ROOT, 'docs', '内核矩阵.md')):
    problems.append('docs/内核矩阵.md 不见了 —— 它是内核支持的唯一口径，删了别处引用就全散')


# 5) 二进制内嵌版本：防「VERSION 升了但 out/f50 还是旧产物就打包」
#    版本串是 build.sh 用 -ldflags -X main.version=<VERSION> 注进去的，
#    所以包里的二进制必然要含当前 VERSION，否则就是拿旧产物糊了张新标签。
BINS = [('out/f50/airsim-mu300d', ver), ('out/f50/airsim-agent', agent)]


def strings_of(path):
    """取二进制的可打印串：优先 strings（mac/linux 都有），没有就裸解 latin-1 兜底。"""
    try:
        r = subprocess.run(['strings', path], capture_output=True, check=True)
        return r.stdout
    except Exception:
        with open(path, 'rb') as f:
            return f.read()


for rel, want in BINS:
    path = os.path.join(ROOT, rel)
    name = os.path.basename(rel)
    if not os.path.exists(path):
        problems.append(f'{rel} 不存在 —— 先跑 sh overlay/f50/build.sh out/f50 再打包')
        continue
    if not want:
        continue
    if want.encode() not in strings_of(path):
        got = sorted({m.decode() for m in re.findall(rb'0\.(?:1\.0\.|4\.7-mu300\.)[0-9]+-r\d\d-[a-z0-9.]+', strings_of(path))})
        problems.append(f'{name} 内嵌版本不含「{want}」{("，里面是：" + "、".join(got)) if got else ""} —— VERSION 升了但二进制没重编，先跑 overlay/f50/build.sh')


# 5b) 音频配件包内核档位：下载器白名单 / rpcd 探测口径 / 实际打包好的 payload 目录，三处必须同一份
downloader = read('overlay/f50/files/usr/libexec/airsim-audioopt-install') or ''
dl_ok = re.search(r'AIRSIM_OPT_KERNELS:-"([^"]+)"', downloader)
rpcd_ok = re.search(r'^OPT_KERNELS="([^"]*)"', rpcd, re.M)
payload_dir = os.path.join(ROOT, 'parts', 'audio', 'payload')
payloads = sorted(d for d in os.listdir(payload_dir) if os.path.isdir(os.path.join(payload_dir, d))) if os.path.isdir(payload_dir) else []
if not dl_ok:
    problems.append('airsim-audioopt-install 里没找到 OPT_KERNELS 白名单（AIRSIM_OPT_KERNELS:-"..."）')
elif not rpcd_ok:
    problems.append('rpcd/airsim-audio 里没找到 OPT_KERNELS="..."')
else:
    dl = sorted(dl_ok.group(1).split())
    rc = sorted(rpcd_ok.group(1).split())
    if dl != rc:
        problems.append(f'音频档位打架：下载器给 {" ".join(dl)}，rpcd 探测口径给 {" ".join(rc)}')
    if payloads and sorted(payloads) != dl:
        problems.append(f'音频档位打架：白名单 {" ".join(dl)}，但 parts/audio/payload 里只有 {" ".join(payloads)}')

# 6) 厂商 CLI 契约：源码里 exec 的厂商脚本（/opt/mu300/bin/*、/usr/bin/mu300-at）
#    必须登记在 docs/厂商CLI契约.md。这类脚本刷机就被厂商换回新版，调用契约会无声改变
#    （典型：`-` / `--stdin` 从 stdin 读，新版只认 argv，命令被原样发给基带、退出码还是 0）。
#    没登记就等于没人检查过它，一旦刷机就可能整块功能静默消失。
contract = read('docs/厂商CLI契约.md')
if contract is None:
    problems.append('docs/厂商CLI契约.md 不存在 —— 厂商 CLI 契约清单丢了，刷机后静默失效无从预防')
else:
    registered = set(re.findall(r'\| `(/[^`]+)` \|', contract))
    called = set()
    for sub in ('src/AirSIM/module/module-agent', 'src/plugin/cmd/airsim-mu300d'):
        d = os.path.join(ROOT, sub)
        if not os.path.isdir(d):
            continue
        for fn in sorted(os.listdir(d)):
            if not fn.endswith('.go') or fn.endswith('_test.go'):
                continue
            text = read(os.path.join(sub, fn)) or ''
            called |= set(re.findall(r'"(/(?:opt/mu300/bin|usr/bin)/[A-Za-z0-9._/-]+)"', text))
    vendor = {p for p in called if not p.endswith('/')}
    unregistered = sorted(vendor - registered)
    if unregistered:
        problems.append('这些厂商 CLI 调用没登记契约：' + '、'.join(unregistered)
                        + ' —— 在 docs/厂商CLI契约.md 表格里补一行，说明契约和保护方式')

if problems:
    print('!! 一致性检查没过：', file=sys.stderr)
    for p in problems:
        print('   - ' + p, file=sys.stderr)
    sys.exit(1)

print(f'一致性 OK：VERSION={ver}  AGENT={agent}  audio-opt={aopt}')
