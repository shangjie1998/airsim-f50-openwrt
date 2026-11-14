#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""打包后闸二：解包抽查关键常量，确认「包里装的确实是刚改的那份源码」。

闸一只保证「仓库里的版本号彼此一致」，管不住「包是怎么生成的」。这轮就出过
「源码改了、包里还是旧常量」的事，所以打完之后再拆开看一遍。

查什么（只查容易忘、且错了会让人白折腾的几项）：
  主包
    - rpcd/airsim-audio 的 TAG / FILE 版本 == parts/audio/AUDIO-OPT-VERSION
    - audio.js / maintenance.js 里得有 ready 判断（防退回只认 installed 的旧版）
    - airsim-audioopt-install 的下载提示是 1MB 档（防退回 420KB 旧文案）
  配件包
    - VERSION == parts/audio/AUDIO-OPT-VERSION
    - opt/install.sh 的 TOOLS_TAG 版本 == AUDIO-OPT-VERSION
    - MANIFEST 里有 depends: 与 tools: 两行（外部依赖声明不能丢）

用法：
  python3 build/tools/check-package.py dist/airsim-f50-0.1.0.29-r23-any.tar.gz
  python3 build/tools/check-package.py dist/airsim-f50-audio-opt-1.0.4-k6.18.60+7.3.1.tar.gz

退出码：0 通过；1 有不符合（打包脚本据此中止）
"""
import re
import sys
import tarfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
AUDIO_VER = (ROOT / 'parts/audio/AUDIO-OPT-VERSION').read_text(encoding='utf-8').strip()

problems = []
checked = []

# 主包里允许出现在 usr/libexec/ 的脚本（相对路径）。加新功能脚本要顺手加到这里，
# 但测试、调试、一次性排查用的东西不进包，放 build/tools/ 或 build/tools/remote/。
LIBEXEC_ALLOWED = {
    'airsim-audioopt-install',
    'airsim-bt-agent',
    'airsim-btopt-install',
    'airsim-flowfix',
    'airsim-net-probe',
    'airsim-opt-latest',
    'airsim-postflash-check',
    'rpcd/airsim',
    'rpcd/airsim-audio',
    'rpcd/airsim-backup',
    'rpcd/airsim-bt',
    'rpcd/airsim-call',
    'rpcd/airsim-net',
    'rpcd/airsim-pair',
    'rpcd/airsim-sms',
}


def read_member(tf, member):
    """取包内某个文件的内容。跳过 macOS 打的 ._ 影子文件——它们名字以 ._ 开头，
    但 endswith 判断会把它们当成目标文件（._VERSION 就是 endswith('VERSION')）。"""
    for m in tf.getmembers():
        base = m.name.rsplit('/', 1)[-1]
        if base.startswith('._'):
            continue
        if m.name.endswith(member) and m.isfile():
            f = tf.extractfile(m)
            return f.read().decode('utf-8', 'replace') if f else ''
    return None


def check_main(tf, names):
    rpcd = read_member(tf, 'usr/libexec/rpcd/airsim-audio')
    if rpcd is None:
        problems.append('主包里没有 rpcd/airsim-audio')
    else:
        tag = re.search(r'^TAG=audio-opt-v(\d+\.\d+\.\d+)', rpcd, re.M)
        fle = re.search(r'^FILE=airsim-f50-audio-opt-(\d+\.\d+\.\d+)-', rpcd, re.M)
        if not tag or tag.group(1) != AUDIO_VER:
            problems.append(f'rpcd TAG 版本不是 {AUDIO_VER}（实得 {tag.group(1) if tag else "无"}）')
        else:
            checked.append(f'rpcd TAG = {AUDIO_VER}')
        if not fle or fle.group(1) != AUDIO_VER:
            problems.append(f'rpcd FILE 版本不是 {AUDIO_VER}（实得 {fle.group(1) if fle else "无"}）')
        else:
            checked.append(f'rpcd FILE = {AUDIO_VER}')

    for js in ('view/airsim/audio.js', 'view/airsim/maintenance.js'):
        body = read_member(tf, js)
        if body is None:
            problems.append(f'主包里没有 {js}')
        elif 'info.ready' not in body:
            problems.append(f'{js} 没有 ready 判断，可能打进了旧版')
        else:
            checked.append(f'{js} 用 ready')

    wrapper = read_member(tf, 'usr/libexec/airsim-audioopt-install')
    if wrapper is None:
        problems.append('主包里没有 airsim-audioopt-install')
    elif '约 1MB' not in wrapper:
        problems.append('airsim-audioopt-install 还是旧文案（没有「约 1MB」），可能没重打')
    else:
        checked.append('安装器提示为 1MB 档')

    # 白名单：usr/libexec 下只许这些脚本进包。
    # 起因是 airsim-audioopt-selftest 那种自测脚本一度放在 overlay 里，
    # 打包就跟着进去了 —— 它第一件事是 rm -rf 模块目录，误跑等于自毁。
    # 测试/调试工具一律放 build/tools/，不参与打包；这里多出一个就拦住。
    present = set()
    for m in tf.getmembers():
        if '/usr/libexec/' not in m.name or not m.isfile():
            continue
        rel = m.name.split('/usr/libexec/', 1)[1]
        if rel and not rel.rsplit('/', 1)[-1].startswith('._'):
            present.add(rel)
    for extra in sorted(present - LIBEXEC_ALLOWED):
        problems.append(f'usr/libexec/{extra} 不在白名单里（测试/调试脚本不许进包，放 build/tools/）')
    if not (present - LIBEXEC_ALLOWED):
        checked.append(f'usr/libexec 只有白名单内 {len(present)} 个脚本')


def check_audio(tf, names):
    ver = read_member(tf, 'VERSION')
    if ver is None:
        problems.append('配件包里没有 VERSION')
    elif ver.strip() != AUDIO_VER:
        problems.append(f'配件包 VERSION={ver.strip()}，源码是 {AUDIO_VER}')
    else:
        checked.append(f'配件包 VERSION = {AUDIO_VER}')

    inst = read_member(tf, 'install.sh')
    if inst is None:
        problems.append('配件包里没有 opt/install.sh')
    else:
        tt = re.search(r'TOOLS_TAG=\$\{AIRSIM_AUDIO_TOOLS_TAG:-audio-opt-v(\d+\.\d+\.\d+)\}', inst)
        if not tt or tt.group(1) != AUDIO_VER:
            problems.append(f'install.sh 的 TOOLS_TAG 版本不是 {AUDIO_VER}')
        else:
            checked.append(f'install.sh TOOLS_TAG = {AUDIO_VER}')
        if 'tools_archive' not in inst:
            problems.append('install.sh 没有 tools_archive 函数（自动补齐丢了）')
        else:
            checked.append('install.sh 有 tools_archive')

    manifest = read_member(tf, 'MANIFEST')
    if manifest is None:
        problems.append('配件包里没有 MANIFEST')
    else:
        if 'depends:' not in manifest:
            problems.append('MANIFEST 缺少 depends: 行（外部依赖声明丢了）')
        else:
            checked.append('MANIFEST 有 depends:')
        if 'tools:' not in manifest:
            problems.append('MANIFEST 缺少 tools: 行')
        else:
            checked.append('MANIFEST 有 tools:')


def main():
    if len(sys.argv) < 2:
        print('用法: check-package.py <包路径.tar.gz>')
        return 2
    pkg = Path(sys.argv[1])
    if not pkg.is_file():
        print(f'找不到包: {pkg}')
        return 2

    with tarfile.open(pkg, 'r:gz') as tf:
        names = [m.name for m in tf.getmembers()]
        # 配件包顶层就是 install.sh / MANIFEST / VERSION / modules/，没有 opt/ 前缀；
        # 主包才是 files/... 那套结构。用「有 install.sh 且有 modules/」认配件包。
        is_audio = any(n.endswith('/install.sh') for n in names) and any('/modules/' in n for n in names)
        if is_audio:
            check_audio(tf, names)
            kind = '音频配件包'
        else:
            check_main(tf, names)
            kind = '主包'

    if problems:
        print(f'!! {kind} 闸二没过：{pkg.name}')
        for p in problems:
            print('   - ' + p)
        return 1

    print(f'闸二 OK（{kind} {pkg.name}）：')
    for c in checked:
        print('   ✓ ' + c)
    return 0


if __name__ == '__main__':
    sys.exit(main())
