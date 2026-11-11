#!/usr/bin/env python3
"""补丁体检：本地能不能干净打上 + 上游是不是已经自己做了。

用法： python3 build/tools/check-patches.py [-v]
读 overlay/f50/patches/manifest.tsv。退出码 1 表示有补丁打不上或上游已合并（构建前就该看）。
"""
import subprocess
import sys
import tempfile
import shutil
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent.parent
PATCH_DIR = ROOT / "overlay/f50/patches"
MANIFEST = PATCH_DIR / "manifest.tsv"
RAW = "https://raw.githubusercontent.com/{repo}/main/{path}"


def read_manifest():
    rows = []
    for line in MANIFEST.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        cols = line.split("\t")
        cols += [""] * (4 - len(cols))
        rows.append({
            "patch": cols[0],
            "repo": cols[1].strip(),
            "merged": cols[2].strip(),
            "anchors": [a for a in cols[3].split(";") if a.strip()],
        })
    return rows


def targets_of(patch: Path):
    out = []
    for line in patch.read_text(encoding="utf-8").splitlines():
        if line.startswith("+++ b/"):
            out.append(line[6:].strip())
    return out


def dry_run(patch: Path):
    """在干净 src 副本上试打，返回 (ok, 详情)。"""
    tmp = Path(tempfile.mkdtemp(prefix="patchcheck-"))
    try:
        subprocess.run(["rsync", "-a", str(ROOT / "src") + "/", str(tmp)], check=True)
        r = subprocess.run(["patch", "-p1", "-N", "--dry-run", "--batch", "-i", str(patch)],
                           cwd=str(tmp), capture_output=True, text=True)
        detail = (r.stdout + r.stderr).strip().splitlines()
        return r.returncode == 0, detail
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def upstream_paths(local_paths):
    """本地路径 -> 上游 raw 路径。src/plugin/ 那层是打包前缀，上游没有。"""
    out = []
    for p in local_paths:
        p = p.replace("plugin/", "", 1) if p.startswith("plugin/") else p
        if p.startswith("AirSIM/"):
            continue  # AirSIM 模块不在这个上游仓库里，没有对照
        out.append(p)
    return out


def fetch(repo, path):
    url = RAW.format(repo=repo, path=path)
    try:
        with urllib.request.urlopen(url, timeout=12) as r:
            return r.read().decode("utf-8", "replace")
    except Exception as e:
        return None


def main():
    verbose = "-v" in sys.argv
    rows = read_manifest()
    bad = []

    print(f"{'补丁':34} {'本地 dry-run':12} {'上游合并':10} 锚点")
    for row in rows:
        patch = PATCH_DIR / row["patch"]
        if not patch.is_file():
            print(f"{row['patch']:34} {'文件不存在':12} {'-':10} -")
            bad.append(row["patch"])
            continue

        ok, detail = dry_run(patch)
        local = "干净" if ok else "打不上"
        if not ok:
            bad.append(row["patch"])

        merged = "-"
        anchor_note = "-"
        if row["repo"]:
            paths = upstream_paths(targets_of(patch))
            if paths:
                fetched = [fetch(row["repo"], p) for p in paths]
                text = "\n".join(t for t in fetched if t) if any(fetched) else None
                if text is None:
                    merged = "取不到"
                elif row["merged"] and row["merged"] in text:
                    merged = "已合并→删补丁"
                    bad.append(row["patch"] + "（上游已合并）")
                else:
                    merged = "未合并"
                    missing = [a for a in row["anchors"] if a and a not in text]
                    anchor_note = "缺:" + ",".join(missing) if missing else "都在"
                    if missing:
                        bad.append(row["patch"] + "（锚点缺失）")

        print(f"{row['patch']:34} {local:12} {merged:10} {anchor_note}")
        if verbose and not ok:
            for line in detail[:8]:
                print("    " + line)

    if bad:
        print("\n需要处理：")
        for item in bad:
            print("  -", item)
        print("\n规则：打不上就去改补丁的上下文（不许加 --fuzz 绕过）；上游已合并就删补丁、跟随上游。")
        print("详见 overlay/f50/patches/README.md")
        return 1
    print("\n全部正常。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
