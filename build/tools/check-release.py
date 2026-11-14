#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""上传后闸三：回读 GitHub Release，确认线上资产就是本地这份。

这轮出的最大一次事故是「页面点了安装，下回来的是旧包」——本地改了、打了、
但上传的是上一次那份，谁也看不出来。这里用机器比对：

  1. 按 tag 拉 release，列出线上资产
  2. 每个本地文件：线上得有同名资产，size 必须一致
  3. 线上同名 .sha256 资产里的哈希，必须等于本地文件实算的哈希
     （size 一致而哈希不一致，说明有人重传过内容不同的包）

只读操作，不改线上任何东西。

用法：
  python3 build/tools/check-release.py --tag audio-opt-v1.0.4 \\
      dist/airsim-f50-audio-opt-1.0.4-k6.18.60+7.3.1.tar.gz \\
      parts/audio/thirdparty/mu300-audio-5.4.tar.gz

  # 指定仓库 / token 文件
  python3 build/tools/check-release.py --repo shangjie1998/airsim-f50-openwrt \\
      --token-file ~/Desktop/m8pat.txt --tag v0.1.0.29-r23-any dist/airsim-f50-0.1.0.29-r23-any.tar.gz

退出码：0 一致；1 有缺失或内容不一致；2 参数/网络问题
"""
import argparse
import hashlib
import json
import os
import sys
import urllib.request
from pathlib import Path

API = 'https://api.github.com'


def token_from(path):
    p = Path(os.path.expanduser(path))
    if not p.is_file():
        return None
    return p.read_text(encoding='utf-8').strip().replace('\n', '').replace('\r', '')


def api_get(url, token):
    req = urllib.request.Request(url, headers={
        'Authorization': f'token {token}',
        'Accept': 'application/vnd.github+json',
    })
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.loads(r.read().decode('utf-8'))


def download_bytes(url, token):
    req = urllib.request.Request(url, headers={
        'Authorization': f'token {token}',
        'Accept': 'application/octet-stream',
    })
    with urllib.request.urlopen(req, timeout=60) as r:
        return r.read()


def sha256_of(path):
    h = hashlib.sha256()
    with open(path, 'rb') as f:
        for chunk in iter(lambda: f.read(1 << 20), b''):
            h.update(chunk)
    return h.hexdigest()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--tag', required=True, help='Release tag，例如 audio-opt-v1.0.4')
    ap.add_argument('--repo', default='shangjie1998/airsim-f50-openwrt')
    ap.add_argument('--token-file', default='~/Desktop/m8pat.txt')
    ap.add_argument('files', nargs='+', help='本地文件路径（可多个）')
    args = ap.parse_args()

    token = token_from(args.token_file)
    if not token:
        print(f'取不到 token（{args.token_file}）', file=sys.stderr)
        return 2

    try:
        rel = api_get(f'{API}/repos/{args.repo}/releases/tags/{args.tag}', token)
    except Exception as e:
        print(f'拉 release 失败：{e}', file=sys.stderr)
        return 2

    assets = {a['name']: a for a in rel.get('assets', [])}
    if not assets:
        print(f'release {args.tag} 里没有任何资产')
        return 1

    problems = []
    checked = []

    for f in args.files:
        p = Path(f)
        if not p.is_file():
            problems.append(f'本地文件不存在：{f}')
            continue
        name = p.name
        a = assets.get(name)
        if not a:
            problems.append(f'线上没有同名资产：{name}')
            continue

        local_size = p.stat().st_size
        if a['size'] != local_size:
            problems.append(f'{name} 大小不一致：线上 {a["size"]}，本地 {local_size}')
            continue
        checked.append(f'{name} 大小一致（{local_size}）')

        # 比对哈希：以线上 .sha256 资产为真值
        sha_asset = assets.get(name + '.sha256')
        if not sha_asset:
            problems.append(f'线上没有 {name}.sha256，无法核对内容')
            continue
        try:
            remote_txt = download_bytes(sha_asset['url'], token).decode('utf-8', 'replace')
        except Exception as e:
            problems.append(f'下载 {name}.sha256 失败：{e}')
            continue
        want = remote_txt.split()[0].strip().lower() if remote_txt.split() else ''
        got = sha256_of(p)
        if not want:
            problems.append(f'{name}.sha256 是空的')
        elif want != got:
            problems.append(f'{name} 哈希不一致：线上 {want[:12]}…，本地 {got[:12]}…（线上是另一份内容）')
        else:
            checked.append(f'{name} 哈希一致（{got[:12]}…）')

    if problems:
        print(f'!! 闸三没过（{args.tag}）：')
        for p in problems:
            print('   - ' + p)
        return 1

    print(f'闸三 OK（{args.tag}）：')
    for c in checked:
        print('   ✓ ' + c)
    return 0


if __name__ == '__main__':
    sys.exit(main())
