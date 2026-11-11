#!/bin/sh
# 给 F50 主线升号，并把要同步的地方一次写齐。
#
# 版本契约：airsim-f50-0.1.0.<N>-r<NN>-<内核范围>
#   0.1.0  与上游一致，上游升版才动
#   <N>    本仓库打包序号，每出一包 +1
#   r<NN>  对齐上游 release（上游 tag 是 v0.1.0-rNN）
#   6.18 / any   内核范围标识。
#                主包自 2026-11 起不绑内核（Go 静态件 + 脚本页面），
#                语音由配件包按内核分别装，所以新包一律用 any；
#                老包仍带 6.18，改动它只为了继续给老线升号。
#
# 用法：
#   sh scripts/bump-version.sh              # 序号 +1，内核范围保持不变
#   sh scripts/bump-version.sh 6            # 指定序号
#   sh scripts/bump-version.sh 6 r24        # 序号与上游 release 一起改
#   sh scripts/bump-version.sh 6 r24 any    # 顺便把内核范围改成 any（推荐新包用）
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
CUR=$(cat "$HERE/VERSION")

KREL=$(printf '%s' "$CUR" | sed -n 's/^0\.1\.0\.[0-9][0-9]*-r[0-9][0-9]*-\(.*\)$/\1/p')
[ -n "$KREL" ] || { echo "VERSION '$CUR' 不是 0.1.0.<n>-r<nn>-<内核范围> 形式" >&2; exit 1; }
case "$KREL" in
6.18|any) ;;
5.4) echo "VERSION '$CUR': 5.4 线的旧包名（-r22-f50），不再升号；新包统一走 any" >&2; exit 1 ;;
*) echo "VERSION '$CUR' 的内核范围 '$KREL' 不认识（只认 6.18 / any）" >&2; exit 1 ;;
esac

REL=$(printf '%s' "$CUR" | sed -n 's/^0\.1\.0\.[0-9][0-9]*-\(r[0-9][0-9]\)-.*$/\1/p')
[ -n "$REL" ] || { echo "VERSION '$CUR' 里取不到上游段 rNN" >&2; exit 1; }

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
if [ "${3:-}" != "" ]; then
	case "$3" in 6.18|any) KREL=$3 ;; *) echo "内核范围只认 6.18 / any" >&2; exit 1 ;; esac
fi

NEW="0.1.0.$NEXT-$REL-$KREL"
AGENT="0.4.7-mu300.$NEW"

printf '%s\n' "$NEW"   > "$HERE/VERSION"
printf '%s\n' "$NEW"   > "$HERE/PACKAGING-VERSION"
printf '%s\n' "$AGENT" > "$HERE/AGENT-VERSION"

echo "$NEW"
echo "agent 版本: $AGENT" >&2
echo "产物名建议: dist/airsim-f50-$NEW.tar.gz；Release tag: v$NEW" >&2
