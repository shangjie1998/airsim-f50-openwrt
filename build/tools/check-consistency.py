#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""打包前的一致性闸：把「同一个事实写在两处」的地方机械对齐，错了就不许打包。

只查「必须有唯一真值」的几项，不查业务逻辑：
  1. VERSION 格式合法、适用范围必须是 any
  2. AGENT-VERSION 必须由 VERSION 派生（0.4.7-mu300.<VERSION>）
  3. 音频配件包版本：parts/audio/AUDIO-OPT-VERSION  ==  rpcd 里 TAG/FILE 的版本
  4. 内核口径文档存在（docs/内核矩阵.md），避免有人把它删了还照旧引用

用法：python3 build/tools/check-consistency.py
退出码：0 一致；1 有冲突（make-package.sh 会据此拒绝打包）
"""
import os
import re
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

# 4) 内核口径单一来源必须在
if not os.path.exists(os.path.join(ROOT, 'docs', '内核矩阵.md')):
    problems.append('docs/内核矩阵.md 不见了 —— 它是内核支持的唯一口径，删了别处引用就全散')

if problems:
    print('!! 一致性检查没过：', file=sys.stderr)
    for p in problems:
        print('   - ' + p, file=sys.stderr)
    sys.exit(1)

print(f'一致性 OK：VERSION={ver}  AGENT={agent}  audio-opt={aopt}')
