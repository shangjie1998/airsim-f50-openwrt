#!/usr/bin/env python3
"""F50 <-> M8 自定义层比对（只读，不改任何文件）。

用法：
    python3 scripts/sync-m8.py                # 默认找 ../airsim-mu300
    python3 scripts/sync-m8.py --m8 /path/to/airsim-mu300
    python3 scripts/sync-m8.py --only shared  # 只看可直搬的

它做三件事：
1. 把两边的 overlay 摊平（去掉 overlay/<机型>/ 前缀），按相对路径求并集
2. 按规则给每个文件打标签：shared（可直搬）/ rewrite（得改写）/ device（设备专属）
3. 对两边同名文件做 diff，给出差异行数

只出报告，不动手。改不改、搬不搬，人拍板。
"""
import argparse
import os
import re
import subprocess
import sys

# --- 分类规则（先看路径，再看内容） -----------------------------------------
# device: 设备专属，任何情况下都不跨项目搬
DEVICE_PATH = [
    r"pkg/", r"m8-pkg/", r"install\.sh", r"uninstall\.sh",
    r"init$", r"\.init$", r"modules/", r"kbuild/",
]
# rewrite: 能参考，但必须按另一台的硬件/参数改写
REWRITE_PATH = [
    r"internal/audio/", r"internal/gateway/", r"cmd/", r"device", r"audio",
    r"VERSION", r"MANIFEST",
]
# shared: 视觉与文案层，原则上可直搬（仍要人确认）
SHARED_PATH = [
    r"ui\.css", r"ui\.js", r"\.html", r"docs/", r"README", r"\.md$",
]
# 内容嗅探：命中即降级为 rewrite，并给出原因
CONTENT_SNIFF = [
    (re.compile(r"hw:\d+,\d+"), "写死了 ALSA 节点"),
    (re.compile(r"[0-9]+k(?:hz)?\b", re.I), "写死了采样率/码率"),
    (re.compile(r"sim[_ ]?slot|slot[_ ]?id|dual[_ ]?sim|card_num|双卡|卡槽", re.I), "涉及卡槽/双卡"),
    (re.compile(r"m8|f50|mu300", re.I), "含机型标识"),
    (re.compile(r"16 ?000|8 ?000|8k|16k", re.I), "涉及音频采样参数"),
]


def walk(root, prefix):
    out = {}
    base = os.path.join(root, prefix)
    for dirpath, _dirs, files in os.walk(base):
        for f in files:
            if f == ".DS_Store":
                continue
            full = os.path.join(dirpath, f)
            rel = os.path.relpath(full, base).replace(os.sep, "/")
            out[rel] = full
    return out


def classify(rel, full):
    tags, why = [], []
    base = rel.rsplit("/", 1)[-1]
    style = bool(re.match(r"(ui\.css|ui\.js|.*\.css|.*\.html)$", base))
    if style:
        tags.append("shared")
        why.append("样式/页面资产")
    if any(re.search(p, rel) for p in DEVICE_PATH):
        tags.append("device")
        why.append("设备专属路径")
    if any(re.search(p, rel) for p in REWRITE_PATH):
        tags.append("rewrite")
        why.append("涉及运行逻辑/设备参数")
    if any(re.search(p, rel) for p in SHARED_PATH):
        tags.append("shared")
        why.append("视觉/文案层")
    if full and os.path.isfile(full) and os.path.getsize(full) < 2_000_000:
        try:
            text = open(full, encoding="utf-8", errors="ignore").read()
        except OSError:
            text = ""
        for rx, reason in CONTENT_SNIFF:
            if rx.search(text):
                tags.append("rewrite")
                why.append(reason)
    if not tags:
        tags.append("rewrite")
        why.append("未命中规则，默认按需改写")
    # 优先级：样式资产只被"内容写死设备参数"降级；其余取最严格
    if style and not any(w for w in why if w in ("写死了 ALSA 节点", "涉及卡槽/双卡")):
        return "shared", sorted(set(why))
    if "device" in tags:
        final = "device"
    elif "rewrite" in tags:
        final = "rewrite"
    else:
        final = "shared"
    return final, sorted(set(why))


def diff_lines(a, b):
    try:
        r = subprocess.run(["diff", "-u", a, b], capture_output=True, text=True)
        return len([l for l in r.stdout.splitlines() if l[:1] in "+-" and not l.startswith(("+++", "---"))])
    except OSError:
        return -1


def norm(rel):
    """把两边的打包路径归一到可对齐的键。
    F50: files/www/luci-static/...        （插件包内根）
    M8 : m8-pkg/luci/www/luci-static/...  （自带包）
    归一后都变成 www/luci-static/...
    """
    for pre in ("m8-pkg/luci/", "files/"):
        if rel.startswith(pre):
            return rel[len(pre):]
    if rel.startswith("m8-pkg/"):
        return "pkg/" + rel[len("m8-pkg/"):]
    return rel


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--m8", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), "../../airsim-mu300"))
    ap.add_argument("--f50", default=os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
    ap.add_argument("--only", choices=["shared", "rewrite", "device", "all"], default="all")
    a = ap.parse_args()

    f50 = walk(os.path.join(a.f50, "overlay/f50"), "")
    m8 = walk(os.path.join(a.m8, "overlay/m8"), "")
    if not f50 and not m8:
        print("两边 overlay 都没找到文件，检查路径", file=sys.stderr)
        return 1

    # 按归一化键配对，才能把 F50 的 files/www/... 和 M8 的 m8-pkg/luci/www/... 认成同一份
    f50n = {norm(k): (k, v) for k, v in f50.items()}
    m8n = {norm(k): (k, v) for k, v in m8.items()}

    rows = []
    for key in sorted(set(f50n) | set(m8n)):
        frec = f50n.get(key)
        mrec = m8n.get(key)
        rel = frec[0] if frec else mrec[0]
        f = frec[1] if frec else None
        m = mrec[1] if mrec else None
        kind, why = classify(rel, f or m)
        if a.only != "all" and kind != a.only:
            continue
        n = diff_lines(f, m) if (f and m) else None
        rows.append((kind, rel, bool(f), bool(m), n, ",".join(why)))

    print(f"F50 overlay {len(f50)} 个文件 / M8 overlay {len(m8)} 个文件 / 归一后 {len(set(f50n)|set(m8n))} 组")
    print(f"{'分类':<8}{'F50':<5}{'M8':<5}{'差异行':<7}路径 → 依据")
    for kind, rel, inf50, inm8, n, why in rows:
        print(f"{kind:<8}{'有' if inf50 else '-':<5}{'有' if inm8 else '-':<5}"
              f"{(str(n) if n is not None else '单边'):<7}{rel} → {why}")

    counts = {k: sum(1 for r in rows if r[0] == k) for k in ("shared", "rewrite", "device")}
    print(f"\n汇总: shared {counts['shared']} / rewrite {counts['rewrite']} / device {counts['device']}")
    print("提醒：shared 也只是『候选』，跨项目落地前仍要人确认（F50 单卡、8k；M8 双卡、16k）。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
