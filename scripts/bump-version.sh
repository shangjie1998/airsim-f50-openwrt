#!/bin/sh
# 把 F50 的发布号 +1，并把版本号一处写死的地方全部同步。
#
# 版本契约：
#   0.1.0.<n>-r22-f50
#     ^^^^^           上游基础版本，跟上游走，不动
#            ^^^^     上游修订号，不动
#         ^^^         本地打包计数，每出一包 +1
#                  ^^ 机型线标识（M8 线是 m8）
#
# 为什么要有这个脚本：包目录里也有一份 VERSION，手工只改仓库根那份的话，
# 打包时按旧值算，MANIFEST 首行就报出过期版本。所以这里三处一起写：
#   1) 仓库根 VERSION
#   2) pkg/<版本>/VERSION
#   3) pkg/<版本>/MANIFEST 首行（目录名里的版本也一起改）
#
# 用法：sh scripts/bump-version.sh        # +1
#       sh scripts/bump-version.sh 7      # 显式指定计数
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
VFILE=$HERE/VERSION
CUR=$(cat "$VFILE")

if ! printf '%s' "$CUR" | grep -q '^0\.1\.0\.[0-9][0-9]*-r22-f50$'; then
	echo "VERSION '$CUR' 不是 0.1.0.<n>-r22-f50 形式" >&2
	exit 1
fi
N=$(printf '%s' "$CUR" | sed -n 's/^0\.1\.0\.\([0-9][0-9]*\)-r22-f50$/\1/p')
if [ "${1:-}" != "" ]; then
	case "$1" in
	''|*[!0-9]*) echo "计数必须是非负整数" >&2; exit 1 ;;
	esac
	NEXT=$1
else
	NEXT=$((N + 1))
fi
NEW="0.1.0.$NEXT-r22-f50"
printf '%s\n' "$NEW" > "$VFILE"

PKGDIR=$HERE/pkg/airsim-f50-$CUR
if [ -d "$PKGDIR" ]; then
	NEWDIR=$HERE/pkg/airsim-f50-$NEW
	[ "$NEWDIR" = "$PKGDIR" ] || mv "$PKGDIR" "$NEWDIR"
	printf '%s\n' "$NEW" > "$NEWDIR/VERSION"
	if [ -f "$NEWDIR/MANIFEST" ]; then
		sed -i '' "1s|.*|$NEW|" "$NEWDIR/MANIFEST"
		sed -i '' "s|$CUR|$NEW|" "$NEWDIR/MANIFEST"
		sed -i '' "s|$CUR|$NEW|" "$NEWDIR/install.sh"
	fi
fi
printf '%s\n' "$NEW"
printf '包名: dist/airsim-f50-%s.tar.gz\n' "$NEW" >&2
