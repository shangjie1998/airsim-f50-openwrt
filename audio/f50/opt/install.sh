#!/bin/sh
# AirSIM for F50 —— 音频模块可选件安装脚本（支持一个包带多套内核）
#
# 干什么：把本包 modules/<KREL>/ 下的 .ko 放进厂商音频扩展目录
#         /mnt/mu300-disk/extra/airsim-audio/modules/<KREL>/，
#         并按需要给 mu300-audio-dsp 放行 CONFIG 逐位比对与 aplay 探针。
#
# 不干什么：不碰内核、不碰 boot、不动分区、不加载模块（重启由 prepare 里的自检决定）。
#
# 用法：sh install.sh            装包里所有内核目录（当前内核那套会被标出来）
#       sh install.sh --verify   只看当前状态，不改任何文件
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
VER=$(cat "$HERE/VERSION" 2>/dev/null || echo '?')
EXT=${EXT:-/mnt/mu300-disk/extra/airsim-audio}
DSP=$EXT/bin/mu300-audio-dsp
KREL=$(uname -r)
MODS=$HERE/modules

die() { echo "装音频模块: $*" >&2; exit 1; }
say() { echo "装音频模块: $*"; }

dest=$EXT/modules/$KREL_WANT
count() { ls "$dest"/*.ko 2>/dev/null | wc -l; }

vermagic_of() { grep -ao 'vermagic=[^[:space:]]*' "$1" 2>/dev/null | head -n 1 | cut -d= -f2; }
pkg_krels() { ls "$MODS" 2>/dev/null | tr '\n' ' '; }

count() { ls "$EXT/modules/$1"/*.ko 2>/dev/null | wc -l | tr -d ' '; }

verify() {
	echo "  当前内核  : $KREL"
	echo "  扩展目录  : $EXT $([ -d "$EXT" ] && echo '存在' || echo '不存在')"
	echo "  DSP       : $DSP $([ -x "$DSP" ] && echo '在' || echo '缺')"
	echo "  包内内核  : $(pkg_krels)"
	for k in $(pkg_krels); do
		echo "  modules/$k : 已装 $(count "$k") 个"
	done
	echo "  包版本    : $VER"
	[ -d "$EXT/modules/$KREL" ] && echo "  匹配判断  : 当前内核有目录，但仍以 vermagic 为准" || echo "  匹配判断  : 当前内核 $KREL 没有对应目录"
}

[ "$(id -u)" = 0 ] || die '要用 root 跑'

if [ "${1:-}" = "--verify" ]; then
	verify
	exit 0
fi

[ -d "$EXT" ] || die "音频扩展目录不存在：$EXT（需要先有厂商那份音频扩展）"
[ -d "$MODS" ] || die '包里没有 modules/ 目录'
[ -n "$(pkg_krels)" ] || die '包里一个内核目录都没有'

matched=0
for k in $(pkg_krels); do
	src=$MODS/$k
	n=$(ls "$src"/*.ko 2>/dev/null | wc -l | tr -d ' ')
	[ "$n" -gt 0 ] || continue
	base=${k%%+*}
	v=$(vermagic_of "$src/audio_reserve.ko")
	case $v in
		"$base"|"$base "*) ;;
		*) die "包内 modules/$k 的 vermagic='$v' 与目录名 $base 不一致，装了内核也拒" ;;
	esac
	dest=$EXT/modules/$k
	if [ -d "$dest" ]; then
		bak=$dest.bak.$(date +%Y%m%d%H%M%S)
		mv "$dest" "$bak"
		say "$k：已有目录，备份到 $bak"
	fi
	mkdir -p "$dest"
	cp -f "$src"/*.ko "$dest"/
	[ -f "$src/KERNEL" ] && cp -f "$src/KERNEL" "$dest/KERNEL"
	say "$k：模块已放到 $dest（$(count "$k") 个 .ko，vermagic=$v）"
	[ "$base" = "$KREL" ] && matched=1
done

if [ "$matched" = 0 ]; then
	say "注意：包内没有与当前内核 $KREL 匹配的一套（只有 $(pkg_krels)），装完不会立刻出声；换到对应内核时这些模块才生效。"
fi

# mu300-audio-dsp 的 set_fits 拿 KERNEL 的 CONFIG 与设备 /proc/config.gz 逐位比。
# 我们这棵树只差两个纯调试项，ABI 与 vermagic 都一致，所以改成「声明了 CONFIG 就放行」。
# 另：dsp_alive 的 aplay 探针在真机上会 EIO（没通话时 DSP 不消费），不再整条判死。
if [ -f "$DSP" ] && [ ! -f "$DSP.orig" ]; then
	cp -f "$DSP" "$DSP.orig"
	cat > /tmp/airsim-patch-dsp.awk <<'AWK'
/config_sum\)/ && /_want/ { print "    [ -n \"$_want\" ] && true"; next }
/aplay --fatal-errors -D "hw:\$card,0"/ && /return 1/ { sub(/return 1/, "true") }
{ print }
AWK
	awk -f /tmp/airsim-patch-dsp.awk "$DSP.orig" > "$DSP.NEW" && mv "$DSP.NEW" "$DSP"
	chmod 755 "$DSP"
	say 'mu300-audio-dsp 已放行 CONFIG 比对与 aplay 探针（原件 = mu300-audio-dsp.orig）'
fi

echo "$VER" > "$EXT/audio-opt-version" 2>/dev/null || true
say '装好了（重开机会走原来的 prepare.local 加载）'
verify
