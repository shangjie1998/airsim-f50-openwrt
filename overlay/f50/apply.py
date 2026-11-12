#!/usr/bin/env python3
"""把 overlay/f50 的个性化改动叠到一棵上游源码树上。
用法: apply.py <树根>   (树根下需有 AirSIM/ 与 plugin/)
任何一条替换规则一次都没命中 => 报错退出(上游改了文案时能被立刻发现)。
"""
import pathlib, subprocess, sys

ROOT = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else '.')
HERE = pathlib.Path(__file__).resolve().parent

# 1) 整文件替换：files/<设备绝对路径> -> plugin/packaging/openwrt/root/<设备绝对路径>
files_root = HERE / 'files'
dst_root = ROOT / 'plugin' / 'packaging' / 'openwrt' / 'root'
n = 0
for src in files_root.rglob('*'):
    if not src.is_file():
        continue
    rel = src.relative_to(files_root)
    dst = dst_root / rel
    dst.parent.mkdir(parents=True, exist_ok=True)
    data = src.read_bytes()
    dst.write_bytes(data)
    # 新增文件（不是覆盖 src 里已有的）默认 644，会让 rpcd 脚本跑不起来；
    # 带 shebang 的一律补上可执行位。
    if not data.startswith(b'#!'):
        mode = 0o644
    else:
        mode = 0o755
    dst.chmod(mode)
    n += 1
print(f'文件替换 {n} 个')

# 2) 补丁：overlay/f50/patches/*.patch，从树根按 -p1 打（补丁里的路径形如
#    a/AirSIM/module/module-agent/sms.go）。补丁必须干净打上，打不上就报错，
#    免得上游改了上下文后静默漏掉。
patch_dir = HERE / 'patches'
MANIFEST = patch_dir / 'manifest.tsv'



def read_manifest():
    rows = []
    if not MANIFEST.is_file():
        return rows
    for line in MANIFEST.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        cols = line.split("	")
        cols += [""] * (5 - len(cols))
        rows.append({
            "patch": cols[0].strip(),
            "state": (cols[1].strip() or "active"),
            "repo": cols[2].strip(),
            "merged": cols[3].strip(),
            "anchors": [a for a in cols[4].split(";") if a.strip()],
        })
    return rows

def contains(root, needle):
    """在树里找一段源码文本，跳 vendor 与二进制。"""
    exts = {'.go', '.sh', '.py', '.uc', '.json', '.lua'}
    for p in root.rglob('*'):
        if not p.is_file() or 'vendor' in p.parts or p.suffix not in exts:
            continue
        try:
            if needle in p.read_text(encoding='utf-8', errors='replace'):
                return True
        except OSError:
            pass
    return False


rows = {r['patch']: r for r in read_manifest()}
if not rows:
    sys.stderr.write('缺少 patches/manifest.tsv，不知道怎么处理补丁\n')
    sys.exit(1)

applied = 0
if patch_dir.is_dir():
    for pf in sorted(patch_dir.glob('*.patch')):
        row = rows.get(pf.name)
        if row is None:
            sys.stderr.write(f'补丁未登记到 manifest.tsv: {pf.name}\n')
            sys.exit(1)
        if row['state'] != 'active':
            sys.stderr.write(f'{pf.name} 状态是 {row["state"]}，不该留在 patches/ 顶层\n')
            sys.exit(1)
        r = subprocess.run(['patch', '-p1', '-N', '--batch', '-i', str(pf)],
                           cwd=str(ROOT), capture_output=True, text=True)
        if r.returncode != 0:
            sys.stderr.write(f'补丁打不上: {pf.name}\n{r.stdout}{r.stderr}')
            sys.exit(1)
        applied += 1

# 已合入源码的补丁：不打，但必须证明源码里真有这功能。
# 否则一旦有人同步上游把改动冲掉，这里能立刻拦住，而不是编出个没功能的二进制。
merged_n = 0
for key, row in rows.items():
    if row['state'] != 'merged':
        continue
    flags = [f for f in row['merged'].split(';') if f.strip()]
    if not flags:
        sys.stderr.write(f'{key} 标了 merged 却没写"已合并标志串"，无法校验\n')
        sys.exit(1)
    missing = [f for f in flags if not contains(ROOT, f)]
    if missing:
        sys.stderr.write(f'{key} 标了 merged，但源码里找不到: {missing}\n'
                         f'补救：把补丁移回 patches/ 顶层并把状态改回 active，'
                         f'或把这段功能重新写进 src/\n')
        sys.exit(1)
    merged_n += 1
if applied:
    print(f'补丁 {applied} 个')
if merged_n:
    print(f'已合入源码校验 {merged_n} 个')

# 2) 字符串替换：只改 .go，跳过 vendor 和 _test.go
pairs = []
for line in (HERE / 'strings.tsv').read_text(encoding='utf-8').splitlines():
    line = line.strip()
    if not line or line.startswith('#'):
        continue
    old, new = line.split('\t')
    pairs.append((old, new))

targets = [p for p in (ROOT / 'plugin').rglob('*.go')
           if 'vendor' not in p.parts and not p.name.endswith('_test.go')]

hits = {old: 0 for old, _ in pairs}
for f in targets:
    t = f.read_text(encoding='utf-8')
    o = t
    for old, new in pairs:
        c = t.count(old)
        if c:
            hits[old] += c
            t = t.replace(old, new)
    if t != o:
        f.write_text(t, encoding='utf-8')

missing = [old for old, c in hits.items() if c == 0]
if missing:
    print('未命中的规则（上游可能改了文案，需人工核对）:', missing, file=sys.stderr)
    sys.exit(1)
print('字符串替换 ' + ', '.join(f'{k}={v}' for k, v in hits.items()))
