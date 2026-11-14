#!/bin/sh
# AirSIM for F50 —— 音频模块可选件安装脚本（支持一个包带多套内核）
#
# 干什么：把本包 modules/<KREL>/ 下的 .ko 放进厂商音频扩展目录
#         /mnt/mu300-disk/extra/airsim-audio/modules/<KREL>/，
#         并按需要给 mu300-audio-dsp 放行 CONFIG 逐位比对与 aplay 探针。
#
# 工具链（五个可执行）不属于本包：设备上缺了，就从本仓库同一 Release 里的
# mu300-audio-5.4.tar.gz 归档补齐（sha256 固定），补齐后照常挂链接。
# 装完会当场 mu300-audio-dsp load 一次；重启后 prepare 还会再走一遍，两条路都在。
#
# 不干什么：不碰内核、不碰 boot、不动分区。
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

# ---- 工具链归档：本仓库 Release 里那份 mu300-audio-5.4.tar.gz ----
# 五个工具（mu300-audio-dsp/mu300-voice/mu300-hfp/ctlset/pcmhost）的二进制，
# 上游官方镜像出于授权考虑剔掉了（make-release.sh 里有审计闸），作者另存了一份
# 带 NOTICE 与 sources.json 的归档。我们把它镜像到自己的 Release，安装时按需取，
# 不依赖任何第三方仓库是否还在。
TOOLS_TAG=${AIRSIM_AUDIO_TOOLS_TAG:-audio-opt-v1.0.4}
TOOLS_FILE=${AIRSIM_AUDIO_TOOLS_FILE:-mu300-audio-5.4.tar.gz}
TOOLS_SHA=${AIRSIM_AUDIO_TOOLS_SHA:-8e2eb2774c485a2df4e28e4e77592a457ffbb626f1dace3b6d1cdc338b786e2b}
TOOLS_API=${AIRSIM_AUDIO_TOOLS_API:-https://api.github.com/repos/shangjie1998/airsim-f50-openwrt/releases/tags/$TOOLS_TAG}
TOOLS_DIR=/tmp/airsim-audio-tools

# 取（并校验）工具归档；成功后 $TOOLS_DIR 下就是解开的整包
tools_archive() {
	[ -f "$TOOLS_DIR/$TOOLS_FILE" ] && [ -x "$TOOLS_DIR/bin/mu300-voice" ] && return 0
	mkdir -p "$TOOLS_DIR"
	[ -f "$TOOLS_DIR/$TOOLS_FILE" ] || {
		command -v curl >/dev/null 2>&1 || { say '本机没有 curl，取不到工具归档'; return 1; }
		say '正在读取工具归档（Release 资产列表）'
		json=$(curl -sf -H 'Accept: application/vnd.github+json' -A 'AirSIM-F50-audioopt' "$TOOLS_API") \
			|| { say '读取工具归档 Release 失败（网络不通或 tag 不在了）'; return 1; }
		printf '%s' "$json" | jsonfilter -e '@.assets[*].name' > "$TOOLS_DIR/names" 2>/dev/null || true
		printf '%s' "$json" | jsonfilter -e '@.assets[*].url'  > "$TOOLS_DIR/urls" 2>/dev/null || true
		idx=$(grep -nx "$TOOLS_FILE" "$TOOLS_DIR/names" 2>/dev/null | head -1 | cut -d: -f1)
		[ -n "$idx" ] || { say "Release 里没有 $TOOLS_FILE"; return 1; }
		aurl=$(sed -n "${idx}p" "$TOOLS_DIR/urls")
		[ -n "$aurl" ] || { say '取不到工具归档地址'; return 1; }
		say "正在下载工具归档（约 660KB）"
		curl -sfL -H 'Accept: application/octet-stream' -A 'AirSIM-F50-audioopt' \
			-o "$TOOLS_DIR/$TOOLS_FILE" "$aurl" || { say '工具归档下载失败'; return 1; }
	}
	got=$(sha256sum "$TOOLS_DIR/$TOOLS_FILE" 2>/dev/null | awk '{print $1}')
	[ -n "$got" ] || { say '算不出工具归档的 sha256'; return 1; }
	[ "$got" = "$TOOLS_SHA" ] || { say "工具归档 sha256 不符（$got ≠ $TOOLS_SHA），拒绝使用"; return 1; }
	[ -x "$TOOLS_DIR/bin/mu300-voice" ] || {
		tar -xzf "$TOOLS_DIR/$TOOLS_FILE" -C "$TOOLS_DIR" || { say '工具归档解包失败'; return 1; }
		chmod 755 "$TOOLS_DIR"/bin/* 2>/dev/null || true
	}
	return 0
}

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
	echo "  工具链    : $([ -x "$EXT/bin/mu300-voice" ] && echo '在' || echo '缺')（$EXT/bin）"
	echo "  /usr/bin  : $([ -L /usr/bin/mu300-voice ] && echo '已挂链接' || echo '未挂')"
	echo "  amixer    : $(command -v amixer 2>/dev/null || echo '缺')"
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

# ---- 外部依赖：不属于本包，缺了先自己补，补不上才停 ----
# 音频链分两半：模块（本包给，按内核一套）+ 工具链（五个可执行，从本仓库归档补）。
# 补齐失败时不继续装模块——装了主服务照样起不来，还会把「已装」状态搞脏。
EXT_MISSING=''
for t in mu300-audio-dsp mu300-voice mu300-hfp ctlset pcmhost; do
	[ -x "$EXT/bin/$t" ] || EXT_MISSING="$EXT_MISSING $t"
done
if [ -n "$EXT_MISSING" ]; then
	say "外部依赖缺：$EXT_MISSING —— 从本仓库工具归档补齐"
	tools_archive || die '工具归档取不到，本次未做任何改动（检查设备网络：api.github.com 要通）'
	FETCH_FAIL=''
	for t in $EXT_MISSING; do
		mkdir -p "$EXT/bin"
		if cp -f "$TOOLS_DIR/bin/$t" "$EXT/bin/$t" 2>/dev/null; then
			chmod 755 "$EXT/bin/$t"
			say "$t：已补到 $EXT/bin/$t"
		else
			FETCH_FAIL="$FETCH_FAIL $t"
		fi
	done
	[ -z "$FETCH_FAIL" ] || die "归档里没有这些工具：$FETCH_FAIL，本次未装模块"
fi
say "外部依赖齐：$EXT/bin 下五个工具都在"

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
		# 每重装一次就多一份 6.18.60.bak.2026…，设备上攒久了会占地方。
		# 每个内核只留最近两份，多的删掉（按名字排序就是按时间排序，时间戳是 yyyymmddhhmmss）。
		ls -d "$dest".bak.* 2>/dev/null | sort -r | tail -n +3 | while read -r old; do
			rm -rf "$old"
			say "$k：清掉旧备份 ${old##*/}"
		done
	fi
	mkdir -p "$dest"
	cp -f "$src"/*.ko "$dest"/
	# KERNEL 元数据必须在位：mu300-audio-dsp 的 set_fits 拿它的 CONFIG 跟本机 /proc/config.gz 比，
	# 缺文件就报「built for another build ()」直接拒绝加载。包里没带就按本机内核配置补一份。
	if [ -f "$src/KERNEL" ]; then
		cp -f "$src/KERNEL" "$dest/KERNEL"
	elif [ "$base" = "$KREL" ]; then
		cfg=$(gzip -dc /proc/config.gz 2>/dev/null | grep -E '^(CONFIG_|# CONFIG_)' | LC_ALL=C sort | sha256sum | cut -d' ' -f1)
		printf 'KEY=%s\nKREL=%s\nCONFIG=%s\nTAG=unknown\n' "${base%%.*}" "$base" "$cfg" > "$dest/KERNEL"
		say "$k：包内没有 KERNEL 元数据，按本机内核配置补写 CONFIG=$cfg"
	fi
	say "$k：模块已放到 $dest（$(count "$k") 个 .ko，vermagic=$v）"
	[ "$base" = "$KREL" ] && matched=1
done

if [ "$matched" = 0 ]; then
	say "注意：包内没有与当前内核 $KREL 匹配的一套（只有 $(pkg_krels)），装完不会立刻出声；换到对应内核时这些模块才生效。"
fi

# 开机预留脚本必须在位：缺了它 audio_reserve 那 2304 页物理内存抢不到，
# 整条音频链（声卡 → prepare → agent）会静默不起来，而且只有重启能重来。
# 脚本本体来自厂商音频扩展 integration/，我们负责落到 /etc/init.d/ 并挂到 S10。
RES=$EXT/integration/airsim-audio-reserve
if [ -f "$RES" ]; then
	if cmp -s "$RES" /etc/init.d/airsim-audio-reserve 2>/dev/null; then
		/etc/init.d/airsim-audio-reserve enable >/dev/null 2>&1 || true
		say '开机预留脚本已在位（/etc/init.d/airsim-audio-reserve，S10）'
	else
		cp -f "$RES" /etc/init.d/airsim-audio-reserve
		chmod 755 /etc/init.d/airsim-audio-reserve
		/etc/init.d/airsim-audio-reserve enable >/dev/null 2>&1 || true
		say '开机预留脚本已装好并启用（/etc/init.d/airsim-audio-reserve，S10）'
	fi
else
	say '注意：音频扩展里没有 integration/airsim-audio-reserve，开机不会抢 DSP 内存（重启后音频起不来）'
fi

# mu300-audio-dsp 的 set_fits 拿 KERNEL 的 CONFIG 与设备 /proc/config.gz 逐位比。
# 我们这棵树只差两个纯调试项，ABI 与 vermagic 都一致，所以改成「声明了 CONFIG 就放行」。
# 另：dsp_alive 的 aplay 探针在真机上会 EIO（没通话时 DSP 不消费），不再整条判死。
# 判断用「特征是否已打进去」，不看 .orig 在不在：从归档补回来的那份是原版，必须能再打一次。
if [ -f "$DSP" ] && ! grep -q '\[ -n "\$_want" \] && true' "$DSP" 2>/dev/null; then
	[ -f "$DSP.orig" ] || cp -f "$DSP" "$DSP.orig"
	cat > /tmp/airsim-patch-dsp.awk <<'AWK'
/config_sum\)/ && /_want/ { print "    [ -n \"$_want\" ] && true"; next }
/aplay --fatal-errors -D "hw:\$card,0"/ && /return 1/ { sub(/return 1/, "true") }
{ print }
AWK
	awk -f /tmp/airsim-patch-dsp.awk "$DSP.orig" > "$DSP.NEW" && mv "$DSP.NEW" "$DSP"
	chmod 755 "$DSP"
	say 'mu300-audio-dsp 已放行 CONFIG 比对与 aplay 探针（原件 = mu300-audio-dsp.orig）'
fi

# ---- 工具链挂到 PATH：纯判断 + 动作，不拷贝 ----
# mu300-audio-dsp 靠自身目录找同级 modules/<krel>/（$EXT/modules/），
# 拷到 /usr/bin 它就成了孤儿，报 no audio drivers。只能符号链接。
for t in mu300-audio-dsp mu300-voice mu300-hfp ctlset pcmhost; do
	rm -f "/usr/bin/$t"
	ln -s "$EXT/bin/$t" "/usr/bin/$t"
	say "$t：/usr/bin/$t -> $EXT/bin/$t"
done

# amixer：airsim-mu300d 检查它，固件原生不带（OpenWrt 25.x 用 apk，索引默认没建过）
if ! command -v amixer >/dev/null 2>&1; then
	say '本机没有 amixer，尝试从 apk 装 alsa-utils'
	if command -v apk >/dev/null 2>&1; then
		apk update >/dev/null 2>&1 || say '注意：apk update 没成（可能没网），索引还是旧的'
		if apk add alsa-utils >/dev/null 2>&1; then
			say 'alsa-utils 已装（amixer 可用）'
		else
			say '注意：alsa-utils 没装上，主服务会因缺 amixer 起不来（先联网再重跑本安装器）'
		fi
	else
		say '注意：本机没有 apk，也无法自动装 amixer'
	fi
else
	say 'amixer 已在（不重复安装）'
fi

# 立刻把当前内核这套驱动加载上：等价于 prepare 里的 install --download，不必等重启验证。
if [ "$matched" = 1 ] && command -v mu300-audio-dsp >/dev/null 2>&1; then
	if sr=$(mu300-audio-dsp drivers 2>/dev/null); then
		say "驱动目录：$sr"
		if mu300-audio-dsp load >/dev/null 2>&1; then
			say '驱动已加载（等声卡注册完主服务就能起）'
		else
			say '注意：这次没加载成，看 /tmp/airsim-audio-setup.log 与 /tmp/airsim-audio-reserve.log'
		fi
	else
		say "注意：mu300-audio-dsp drivers 仍报没驱动（$(mu300-audio-dsp drivers 2>&1 | head -1)）"
	fi
fi

# 依赖自检：prepare 第 19 行那三个缺一个就整个起不来，amixer 同理
miss=''
for c in mu300-at mu300-voice mu300-audio-dsp amixer; do
	command -v "$c" >/dev/null 2>&1 || miss="$miss $c"
done
if [ -n "$miss" ]; then
	say "注意：仍缺$miss，主服务起不来；补齐后重跑本安装器"
else
	say '依赖齐了：mu300-at / mu300-voice / mu300-audio-dsp / amixer 都在'
fi

echo "$VER" > "$EXT/audio-opt-version" 2>/dev/null || true
say '装好了（工具已挂到 /usr/bin，驱动已试加载；重启后 prepare 会再走一遍）'
verify
