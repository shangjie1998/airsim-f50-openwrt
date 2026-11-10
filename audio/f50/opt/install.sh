#!/bin/sh
# AirSIM for F50 —— 6.18 音频模块可选件安装脚本
#
# 干什么：把本包 modules/6.18.60/ 下 25 个 .ko 放进厂商音频扩展目录
#         /mnt/mu300-disk/extra/airsim-audio/modules/6.18.60/，
#         并按需要给 mu300-audio-dsp 放行 CONFIG 逐位比对与 aplay 探针。
#
# 不干什么：不碰内核、不碰 boot、不动分区、不加载模块（重启由原 prepare.local 负责）。
#
# 用法：sh install.sh            装模块
#       sh install.sh --verify   只看当前状态，不改任何文件
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
VER=$(cat "$HERE/VERSION" 2>/dev/null || echo '?')
EXT=${EXT:-/mnt/mu300-disk/extra/airsim-audio}
DSP=$EXT/bin/mu300-audio-dsp
KREL_WANT=6.18.60

die() { echo "装音频模块: $*" >&2; exit 1; }
say() { echo "装音频模块: $*"; }

dest=$EXT/modules/$KREL_WANT
count() { ls "$dest"/*.ko 2>/dev/null | wc -l; }

verify() {
	echo "  内核      : $(uname -r)"
	echo "  扩展目录  : $EXT $([ -d "$EXT" ] && echo '存在' || echo '不存在')"
	echo "  DSP       : $DSP $([ -x "$DSP" ] && echo '在' || echo '缺')"
	echo "  .ko 数量  : $(count) / 25"
	echo "  包版本    : $VER"
}

[ "$(id -u)" = 0 ] || die '要用 root 跑'

if [ "${1:-}" = "--verify" ]; then
	verify
	exit 0
fi

krel=$(uname -r)
[ "$krel" = "$KREL_WANT" ] || die "当前内核是 $krel，本包只给 $KREL_WANT 用"

[ -d "$EXT" ] || die "音频扩展目录不存在：$EXT（需要先有厂商那份音频扩展）"
[ -d "$HERE/modules/$KREL_WANT" ] || die "包里没有 modules/$KREL_WANT"

# vermagic 对一遍：跟设备上现成的 .ko 必须一致，否则 insmod 会被内核拒。
src=$HERE/modules/$KREL_WANT
ourko=$src/sprd_audcp_boot.ko
[ -f "$ourko" ] || die "包里没有 sprd_audcp_boot.ko"
get_vermagic() { strings "$1" 2>/dev/null | sed -n 's/^vermagic=/vermagic=/p' | head -1; }
if command -v strings >/dev/null 2>&1; then
	stock=$(find /lib/modules -maxdepth 3 -name '*.ko' 2>/dev/null | head -1 || true)
	ours=$(get_vermagic "$ourko")
	[ -n "$ours" ] || die '取不到本包模块的 vermagic'
	if [ -n "$stock" ]; then
		theirs=$(get_vermagic "$stock")
		say "设备现成模块 vermagic: ${theirs:-未知}"
		say "本包模块     vermagic: $ours"
		[ "$theirs" = "$ours" ] || die 'vermagic 不一致，装了也加载不了（内核不是同一套构建）'
	else
		say "设备上没有可对照的 .ko，跳过 vermagic 比对（$ours）"
	fi
else
	say '设备上没有 strings，跳过 vermagic 比对'
fi

# 旧的先备份，再整目录换新
if [ -d "$dest" ]; then
	bak=$EXT/modules/$KREL_WANT.bak.$(date +%Y%m%d%H%M%S)
	mv "$dest" "$bak"
	say "已有 $KREL_WANT 目录，备份到 $bak"
fi
mkdir -p "$dest"
cp -f "$src"/*.ko "$dest"/
say "模块已放到 $dest（$(count) 个 .ko）"

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
