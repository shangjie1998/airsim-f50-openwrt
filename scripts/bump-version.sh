#!/bin/sh
# 给 F50 的 6.18 线升号，并把要同步的地方一次写齐。
#
# 版本契约：airsim-f50-0.1.0.<N>-r<NN>-<内核线>
#   0.1.0  与上游一致，上游升版才动
#   <N>    本仓库打包序号，每出一包 +1，只给在研线加
#   r<NN>  对齐上游 release（上游 tag 是 v0.1.0-rNN）
#   6.18   内核线（5.4 已冻结，脚本遇到非 6.18 直接拒绝）
#
# 用法：
#   sh scripts/bump-version.sh              # 序号 +1，rNN 保持不变
#   sh scripts/bump-version.sh 6            # 指定序号
#   sh scripts/bump-version.sh 6 r24        # 序号与上游 release 一起改
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
CUR=$(cat "$HERE/VERSION")

KREL=$(printf '%s' "$CUR" | sed -n 's/^0\.1\.0\.[0-9][0-9]*-r[0-9][0-9]*-\(.*\)$/\1/p')
REL=$(printf '%s' "$CUR" | sed -n 's/^0\.1\.0\.[0-9][0-9]*-\(r[0-9][0-9]\)-[^0-9].*$/\1/p')
[ -n "$REL" ] || REL=$(printf '%s' "$CUR" | sed -n 's/^0\.1\.0\.[0-9][0-9]*-\(r[0-9][0-9]\)-.*$/\1/p')
[ -n "$KREL" ] && [ -n "$REL" ] || { echo "VERSION '$CUR' 不是 0.1.0.<n>-r<nn>-<内核线> 形式" >&2; exit 1; }
[ "$KREL" = "6.18" ] || { echo "VERSION '$CUR' 属于已冻结的 5.4 线，不再加号" >&2; exit 1; }

N=$(printf '%s' "$CUR" | sed -n 's/^0\.1\.0\.\([0-9][0-9]*\)-.*$/\1/p')
if [ "${1:-}" != "" ]; then
	case "$1" in ''|*[!0-9]*) echo "序号必须是非负整数" >&2; exit 1 ;; esac
	NEXT=$1
else
	NEXT=$((N + 1))
fi
if [ "${2:-}" != "" ]; then
	case "$2" in r[0-9][0-9]) REL=$2 ;; *) echo "上游段必须是 rNN，如 r23" >&2; exit 1 ;; esac
fi

NEW="0.1.0.$NEXT-$REL-$KREL"
AGENT="0.4.7-mu300.$NEW"

printf '%s\n' "$NEW"   > "$HERE/VERSION"
printf '%s\n' "$NEW"   > "$HERE/PACKAGING-VERSION"
printf '%s\n' "$AGENT" > "$HERE/AGENT-VERSION"

echo "$NEW"
echo "agent 版本: $AGENT" >&2
echo "产物名建议: dist/airsim-f50-$NEW.tar.gz；Release tag: v$NEW" >&2
