#!/bin/sh
# 按指定内核 release 重编 F50 的语音模块（audio-opt 的 payload 来源）。
#
# 用法（在仓库根目录）：
#   sh parts/audio/build-krel.sh --krel 6.18.54 --tree /path/linux-6.18.54 --tag nightly-20261109
#   sh parts/audio/build-krel.sh --krel 6.18.54 --tree /path/kernel --modsrc /path/vendor-audio-modules
#   sh parts/audio/build-krel.sh --check 6.18.54     # 只看现有产物对不对，不编译
#
# 前置条件（缺一个就别往下跑，先补）：
#   1. --tree 指向与设备当前内核同一份的构建树：有 Makefile、.config、
#      Module.symvers、include/generated/autoconf.h，且 `make -C <tree> kernelrelease`
#      必须等于 --krel。对不上就别编，编了装上去也会被内核拒。
#   2. 模块源码：要么在 tree 里（vendor 内核整树），要么用 --modsrc 指向外置模块目录。
#   3. 交叉工具链在 PATH 里，或显式 --cross：默认按 tree 的 .config 读出 gcc 版本要求。
#   4. 主机是 Linux（deb 容器即可）。macOS 上不保证能编。
#
# 产物：
#   parts/audio/payload/<KREL>+<tag>/  25 个 .ko + KERNEL
# 之后 `sh parts/audio/make-opt.sh` 就能打到音频可选件包里。
#
# 校验：本脚本会逐个 .ko 读 vermagic，与 --krel 不一致的直接失败，
# 不会产出“看着编完了、装上去内核拒绝”的包。
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO=$(CDPATH= cd -- "$HERE/../.." && pwd)
LIST=$HERE/module-list.txt
PAY=$HERE/payload

KREL=""
TREE=""
MODSRC=""
TAG="local"
CROSS=""
JOBS=$(nproc 2>/dev/null || echo 4)
CHECK=""

die() { echo "build-krel: $*" >&2; exit 1; }
note() { echo "  $*"; }

usage() {
	cat <<'EOF'
用法:
  sh parts/audio/build-krel.sh --krel <KREL> --tree <内核构建树> [--tag <tag>] [--modsrc <模块源码>] [--cross <前缀>] [--jobs N]
  sh parts/audio/build-krel.sh --check <KREL>+<tag>
EOF
	exit 2
}

vermagic_of() { grep -ao 'vermagic=[^[:space:]]*' "$1" 2>/dev/null | head -n 1 | cut -d= -f2; }

while [ $# -gt 0 ]; do
	case $1 in
		--krel) KREL=$2; shift 2 ;;
		--tree) TREE=$2; shift 2 ;;
		--modsrc) MODSRC=$2; shift 2 ;;
		--tag) TAG=$2; shift 2 ;;
		--cross) CROSS=$2; shift 2 ;;
		--jobs) JOBS=$2; shift 2 ;;
		--check) CHECK=$2; shift 2 ;;
		-h|--help) usage ;;
		*) die "不认识参数 $1" ;;
	esac
done

# ---- --check 模式：只校验已有 payload 目录 ----
if [ -n "$CHECK" ]; then
	dir=$PAY/$CHECK
	[ -d "$dir" ] || die "payload/$CHECK 不存在"
	reported=0; bad=0; missing_n=0
	while IFS= read -r m; do
		case $m in ''|'#'*) continue ;; esac
		reported=$((reported + 1))
		if [ ! -f "$dir/$m" ]; then
			echo "  缺: $m"; missing_n=$((missing_n + 1)); continue
		fi
		v=$(vermagic_of "$dir/$m")
		case $v in
			"${CHECK%%+*}"|"${CHECK%%+*}"' '*) ;;
			*) echo "  vermagic 不对: $m -> '$v'"; bad=$((bad + 1)) ;;
		esac
	done < "$LIST"
	echo "== 校验 $CHECK == 清单 ${reported} 个，缺 ${missing_n}，vermagic 错 ${bad}"
	[ "$missing_n" = 0 ] && [ "$bad" = 0 ] || exit 1
	exit 0
fi

[ -n "$KREL" ] || die "缺 --krel"
[ -n "$TREE" ] || die "缺 --tree（内核构建树路径）"
[ -d "$TREE" ] || die "--tree $TREE 不存在"
for need in Makefile .config Module.symvers include/generated/autoconf.h; do
	[ -e "$TREE/$need" ] || die "$TREE 不像构建树：缺 $need（先在内核树里跑一遍 make modules_prepare）"
done
[ "$(uname -s)" = "Linux" ] || note "警告：当前不是 Linux，交叉编译可能失败"

# ---- 内核 release 必须自证 ----
tree_rel=$(make -s -C "$TREE" kernelrelease 2>/dev/null | tail -n 1)
[ "$tree_rel" = "$KREL" ] || die "构建树 kernelrelease='$tree_rel'，与 --krel '$KREL' 不符（编出来必被内核拒）"
note "构建树 kernelrelease=$tree_rel 对上"

# ---- 交叉编译前缀 ----
[ -n "$CROSS" ] || CROSS=""
export ARCH=arm64
[ -n "$CROSS" ] && export CROSS_COMPILE="$CROSS"
# vermagic 必须带 SMP mod_unload aarch64，构建树 .config 里 CONFIG_SMP 得是 y
grep -q '^CONFIG_SMP=y' "$TREE/.config" || note "警告：.config 里没有 CONFIG_SMP=y，vermagic 会和设备不一致"

# ---- 编译 ----
NAME=$KREL
[ "$TAG" != "local" ] && NAME=$KREL+$TAG
OUT=$PAY/$NAME
LOG=${TMPDIR:-/tmp}/airsim-build-$NAME.log
: > "$LOG"

note "开始编（日志 $LOG）"
if [ -n "$MODSRC" ]; then
	[ -d "$MODSRC" ] || die "--modsrc $MODSRC 不存在"
	make -j"$JOBS" -C "$MODSRC" KDIR="$TREE" M="$MODSRC" ARCH=arm64 ${CROSS:+CROSS_COMPILE=$CROSS} modules >>"$LOG" 2>&1 \
		|| { tail -n 25 "$LOG"; die "外置模块编译失败，完整日志 $LOG"; }
	SRCROOT=$MODSRC
else
	make -j"$JOBS" -C "$TREE" modules >>"$LOG" 2>&1 \
		|| { tail -n 25 "$LOG"; die "内核树 modules 编译失败，完整日志 $LOG"; }
	SRCROOT=$TREE
fi

# ---- 收模块 ----
rm -rf "$OUT"
mkdir -p "$OUT"
miss=""
while IFS= read -r m; do
	[ -n "$m" ] || continue
	found=$(find "$SRCROOT" -name "$m" -type f 2>/dev/null | head -n 1)
	if [ -z "$found" ]; then
		miss="$miss $m"
		continue
	fi
	v=$(vermagic_of "$found")
	case $v in
		"$KREL"|"$KREL"' '*) ;;
		*) die "$m 的 vermagic='$v' 不是 $KREL，这份构建树不对" ;;
	esac
	cp -p "$found" "$OUT/$m"
done < "$LIST"

[ -z "$miss" ] || die "这些模块没编出来：$miss（检查 .config 里对应 CONFIG_ 是否 =m，或源码里是否还有这个模块）"

# ---- KERNEL 元数据 ----
conf_hash=$(sha256sum "$TREE/.config" 2>/dev/null | cut -d' ' -f1)
{
	echo "KEY=${KREL%%.*}"
	echo "KREL=$KREL"
	echo "CONFIG=$conf_hash"
	echo "TAG=$TAG"
} > "$OUT/KERNEL"

echo "== 编好 =="
ls -l "$OUT" | tail -n +2 | wc -l | tr -d ' ' | sed 's/^/  模块数（含 KERNEL）: /'
cat "$OUT/KERNEL"
echo "下一步：sh parts/audio/make-opt.sh $NAME"
