#!/bin/sh
# 给 F50 的 6.18 线把打包序号 +1，并把要同步的地方一次写齐。
#
# 版本契约（2026-11-09 起）：
#   airsim-f50-0.1.0.<N>-r<NN>-<内核线>
#          └┬─┘ └─┬─┘ └┬┘ └─┬┘ └──┬──┘
#           │    │    │    │      └ 内核线：6.18（在研）／5.4（冻结）
#           │    │    │    └──────── 对应上游 release（上游 tag 是 v0.1.0-rNN），
#           │    │    │               源码钉哪个就写哪个，现在是 r22
#           │    │    └───────────── 主版本，与上游版本号一致，上游升版才动
#           │    └────────────────── 本仓库打包序号，每出一包 +1
#           └─────────────────────── 机型线：F50（M8 线是 m8，两者不可互换）
#
# 例：airsim-f50-0.1.0.4-r22-6.18
#
# 注意：
#   - 序号只给 6.18 线加。5.4 线已冻结在 0.1.0.3-r22-f50，脚本遇到 5.4 直接拒绝。
#   - pkg/ 里那套覆盖包属于 5.4 线，不参与这次计数，脚本不碰它
#     （包内 /usr/lib/airsim/VERSION 仍是上游 0.1.0-r22，更别动）。
#
# 用法：sh scripts/bump-version.sh        # +1
#       sh scripts/bump-version.sh 7      # 显式指定序号
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VFILE=$HERE/VERSION
PVER=$HERE/PACKAGING-VERSION
CUR=$(cat "$VFILE")

KREL=$(printf '%s' "$CUR" | sed -n 's/^0\.1\.0\.[0-9][0-9]*-r[0-9][0-9]*-\(.*\)$/\1/p')
if [ -z "$KREL" ]; then
	echo "VERSION '$CUR' 不是 0.1.0.<n>-r<nn>-<内核线> 形式" >&2
	exit 1
fi
if [ "$KREL" != "6.18" ]; then
	echo "VERSION '$CUR' 属于已冻结的 5.4 线，不再加号" >&2
	exit 1
fi
N=$(printf '%s' "$CUR" | sed -n 's/^0\.1\.0\.\([0-9][0-9]*\)-r[0-9][0-9]*-.*$/\1/p')
if [ "${1:-}" != "" ]; then
	case "$1" in
	''|*[!0-9]*) echo "序号必须是非负整数" >&2; exit 1 ;;
	esac
	NEXT=$1
else
	NEXT=$((N + 1))
fi
NEW="0.1.0.$NEXT-r22-$KREL"

printf '%s\n' "$NEW" > "$VFILE"
[ -f "$PVER" ] && printf '%s\n' "$NEW" > "$PVER"

printf '%s\n' "$NEW"
printf '建议产物名：dist/airsim-f50-%s-*.tar.gz；Release tag: v%s\n' "$NEW" "$NEW" >&2
