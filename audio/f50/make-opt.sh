#!/bin/sh
# 打音频驱动可选件（audio-opt），支持一个包带多套内核：
#   dist/airsim-f50-audio-opt-<ver>-k<krel>[+<krel>...].tar.gz
#   dist/...tar.gz.sha256
#
# 目录约定（payload/ 下）：
#   payload/<KREL>/            默认：目录名就是内核 release（uname -r）
#   payload/<KREL>+<tag>/      同一个 KREL 有多套构建时（tag 记系统镜像版本）
# 每个目录里放该内核的 *.ko，可带 KERNEL 元数据（KEY/KREL/CONFIG/TAG）。
# 判据永远是 .ko 里的 vermagic，与目录名一致性在这里先卡一道。
#
# 用法：sh make-opt.sh              打 payload/ 下全部内核
#       sh make-opt.sh 6.18.60      只打指定内核
#       sh make-opt.sh 6.18.54 6.18.60
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO=$(CDPATH= cd -- "$HERE/../.." && pwd)
PAY=$HERE/payload
VER=$(cat "$HERE/AUDIO-OPT-VERSION")
OUT=$REPO/dist

die() { echo "打音频可选件: $*" >&2; exit 1; }
vermagic_of() { grep -ao 'vermagic=[^[:space:]]*' "$1" 2>/dev/null | head -n 1 | cut -d= -f2; }

if [ $# -gt 0 ]; then
	KRELS="$*"
else
	KRELS=$(ls "$PAY" 2>/dev/null | tr '\n' ' ')
fi
[ -n "$KRELS" ] || die "payload/ 下没有内核目录"

for k in $KRELS; do
	[ -d "$PAY/$k" ] || die "payload/$k 不存在"
	n=$(ls "$PAY/$k"/*.ko 2>/dev/null | wc -l | tr -d ' ')
	[ "$n" -gt 0 ] || die "payload/$k 里没有 .ko"
	[ -f "$PAY/$k/audio_reserve.ko" ] || die "payload/$k 缺 audio_reserve.ko（判 vermagic 要用它）"
	base=${k%%+*}
	v=$(vermagic_of "$PAY/$k/audio_reserve.ko")
	case $v in
		"$base"|"$base "*) ;;
		*) die "payload/$k 的 vermagic='$v' 与目录名 $base 对不上，装上去内核也会拒" ;;
	esac
	[ -f "$PAY/$k/KERNEL" ] || die "payload/$k 缺 KERNEL 元数据（至少写 KEY=/KREL=/CONFIG=/TAG=）"
done

NAME=airsim-f50-audio-opt-$VER-k$(printf '%s' $KRELS | tr ' ' '+')
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
ROOT=$STAGE/$NAME
mkdir -p "$ROOT"

cp "$HERE/opt/install.sh" "$ROOT/install.sh"
chmod 755 "$ROOT/install.sh"
echo "$VER" > "$ROOT/VERSION"

{
	echo "name: airsim-f50-audio-opt"
	echo "version: $VER"
	echo "arch: aarch64"
	echo "kernels: $(printf '%s ' $KRELS)"
	for k in $KRELS; do
		n=$(ls "$PAY/$k"/*.ko 2>/dev/null | wc -l | tr -d ' ')
		echo "kernel[$(printf '%s' $k)]: $n modules, vermagic=$(vermagic_of "$PAY/$k/audio_reserve.ko")"
	done
} > "$ROOT/MANIFEST"

for k in $KRELS; do
	mkdir -p "$ROOT/modules/$k"
	cp "$PAY/$k"/*.ko "$ROOT/modules/$k"/
	cp "$PAY/$k/KERNEL" "$ROOT/modules/$k"/KERNEL
done

mkdir -p "$OUT"
tar -C "$STAGE" -czf "$OUT/$NAME.tar.gz" "$NAME"
( cd "$OUT" && sha256sum "$NAME.tar.gz" > "$NAME.tar.gz.sha256" )

echo "== 打好 =="
ls -l "$OUT/$NAME.tar.gz"
cat "$OUT/$NAME.tar.gz.sha256"
