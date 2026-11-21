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
	# 下划线开头的目录是待重建占位（还只有 README），全量打包时跳过
	KRELS=$(ls "$PAY" 2>/dev/null | grep -v '^_' | tr '\n' ' ')
fi
[ -n "$KRELS" ] || die "payload/ 下没有内核目录"

for k in $KRELS; do
	case $k in _*) die "payload/$k 是待重建占位，还不能打包" ;; esac
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

# 多套内核时用 + 连接：-k6.18.60+7.3.1
# 坑：printf '%s' 后不加引号时，多个参数会被拼成一串（7.3.1 粘到 6.18.60 后面变 6.18.607.3.1）
NAME=airsim-f50-audio-opt-$VER-k$(printf '%s\n' $KRELS | grep -v '^$' | paste -sd+ -)
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
ROOT=$STAGE/$NAME
mkdir -p "$ROOT"

cp "$HERE/opt/install.sh" "$ROOT/install.sh"
chmod 755 "$ROOT/install.sh"
echo "$VER" > "$ROOT/VERSION"

# 开机预留脚本（S10）随包自带：解到 integration/，install.sh 会同步到扩展目录并落到 /etc/init.d/
if [ -d "$HERE/opt/integration" ]; then
	mkdir -p "$ROOT/integration"
	cp -f "$HERE/opt/integration/"* "$ROOT/integration/"
	chmod 755 "$ROOT/integration/"*
fi

# 随包自带的文本脚本（bin/）：目前只有 mu300-audio-dsp。它是自研 MIT 脚本，
# 必须跟模块一起进包，安装时逐位校验权威哈希（见 opt/install.sh 的 DSP_SHA），
# 不再从 5.4 工具归档补、也不在现场打补丁。四个 ELF 工具仍走外部归档，不在这里。
DSP_FILE=mu300-audio-dsp
DSP_SHA=db096a0b94b62ee0d781f6e3707d20613e6fe515b91bdef865d24d65aba66a09
if [ -d "$HERE/opt/bin" ] && [ -n "$(ls -A "$HERE/opt/bin" 2>/dev/null)" ]; then
	mkdir -p "$ROOT/bin"
	cp -f "$HERE/opt/bin/"* "$ROOT/bin/"
	chmod 755 "$ROOT/bin/"*
else
	die "opt/bin 里没有 mu300-audio-dsp：随包脚本缺失，不能打包"
fi
# 随包 dsp 打包前先核哈希，混进老版在这道就拦住。
_dsp_sha=$(shasum -a 256 "$ROOT/bin/$DSP_FILE" | awk '{print $1}')
[ "$_dsp_sha" = "$DSP_SHA" ] || die "包内 $DSP_FILE 哈希 $_dsp_sha ≠ 权威 $DSP_SHA，拒绝打包"

{
	echo "name: airsim-f50-audio-opt"
	echo "version: $VER"
	echo "arch: aarch64"
	echo "kernels: $(printf '%s ' $KRELS)"
	echo "depends: audio-extra(/mnt/mu300-disk/extra/airsim-audio)"
	echo "tools: bundled=mu300-audio-dsp (sha256 $DSP_SHA); external=mu300-voice mu300-hfp ctlset pcmhost (mu300-audio-5.4.tar.gz)"
	echo "integration: airsim-audio-reserve (S10 开机预留)"
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
# 跟主包一样关掉 macOS 的扩展属性与 AppleDouble：bsdtar 默认会把每个人文件都塞一个
# ._影子进来，包里多一层噪音，校验脚本也会被 ._VERSION 这种东西骗到。
COPYFILE_DISABLE=1 tar --no-xattrs --no-mac-metadata -C "$STAGE" -czf "$OUT/$NAME.tar.gz" "$NAME"
( cd "$OUT" && shasum -a 256 "$NAME.tar.gz" | awk '{print $1"  "$2}' > "$NAME.tar.gz.sha256" )

echo "== 打好 =="
ls -l "$OUT/$NAME.tar.gz"
cat "$OUT/$NAME.tar.gz.sha256"

# 闸二：确认包里的 VERSION / TOOLS_TAG / MANIFEST 跟源码一致
python3 "$REPO/build/tools/check-package.py" "$OUT/$NAME.tar.gz" || {
	echo "!! 闸二没过，包不能发" >&2
	exit 1
}
echo "提示：上传后请跑闸三核对线上内容 ——"
echo "  python3 build/tools/check-release.py --tag audio-opt-v$VER $OUT/$NAME.tar.gz parts/audio/thirdparty/mu300-audio-5.4.tar.gz"
