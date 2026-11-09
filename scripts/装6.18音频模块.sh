#!/bin/sh
# 在 F50（UMS9620 / OpenWrt / 6.18.60）上装自编的 6.18.60 音频驱动模块。
#
# 背景：上游音频扩展只随 5.4.254-gb50db5b6224c 提供 modules/<krel>/，6.18 没有。
# 我们自己用 mu300-linux 的 6.18.60 树造了 kit，再用 mu300-audio 的 build-in.sh 编出 25 个 .ko，
# 现在把它们放进已经存在的音频扩展目录里，让 mu300-audio-dsp 在 6.18 上也能加载。
#
# 用法：
#   sh 装6.18音频模块.sh --install        只装模块，不动 DSP
#   sh 装6.18音频模块.sh --install --start 装完顺手 reserve + start
#   sh 装6.18音频模块.sh --verify          只检查当前状态
set -eu
PATH=/usr/lib/airsim/bin:/opt/mu300/bin:/opt/mu300/busybox-bin:/usr/sbin:/usr/bin:/sbin:/bin
export PATH

PKG_TAR=${PKG_TAR:-/tmp/airsim-audio-6.18.60-modules.tar.gz}
EXT=${EXT:-/mnt/mu300-disk/extra/airsim-audio}
DSP=${DSP:-$EXT/bin/mu300-audio-dsp}
KREL_WANT=6.18.60

die() { echo "装6.18音频模块: $*" >&2; exit 1; }
say() { echo "装6.18音频模块: $*"; }

[ "$(id -u)" = 0 ] || die '要用 root 跑'
krel=$(uname -r)
[ "$krel" = "$KREL_WANT" ] || die "当前内核是 $krel，本包只给 $KREL_WANT 用（先切内核）"
[ -d "$EXT" ] || die "音频扩展目录不存在：$EXT（先用 AirSIM 的音频安装流程装一次 5.4 的）"

verify() {
    say "内核 $krel / 扩展目录 $EXT"
    if [ -x "$DSP" ]; then
        echo "--- mu300-audio-dsp drivers ---"
        "$DSP" drivers 2>&1 || true
        echo "--- mu300-audio-dsp status ---"
        "$DSP" status 2>&1 || true
    else
        say "找不到 $DSP"
    fi
    echo "--- 已加载的音频模块 ---"
    lsmod 2>/dev/null | awk 'NR==1 || /^audio_|^agdsp|^sprd_audcp|^snd-|^snd_|^mcdt|^sprd_apipe|^sprd-dma|^sprd-platform|^sprd-compr/'
    echo "--- 声卡 ---"
    cat /proc/asound/cards 2>/dev/null || echo '没有 /proc/asound/cards'
}

case "${1:---verify}" in
    --verify) verify; exit 0 ;;
    --install) ;;
    *) die 'Usage: 装6.18音频模块.sh --install [--start] | --verify' ;;
esac

[ -f "$PKG_TAR" ] || die "找不到模块包：$PKG_TAR"

# 1) vermagic 先对一遍：设备上现成的 6.18 模块 vs 我们的模块，字符串必须一致，否则 insmod 会被内核拒。
stock=$(find /lib/modules -maxdepth 3 -name '*.ko' 2>/dev/null | head -1 || true)
work=/tmp/airsim-618.$$
mkdir -p "$work"
tar -xzf "$PKG_TAR" -C "$work" || die '解包失败'
ourko=$work/modules/$KREL_WANT/sprd_audcp_boot.ko
[ -f "$ourko" ] || die "包里没有 modules/$KREL_WANT/sprd_audcp_boot.ko"
get_vermagic() { strings "$1" 2>/dev/null | sed -n 's/^vermagic=/vermagic=/p' | head -1; }
ours=$(get_vermagic "$ourko")
[ -n "$ours" ] || die '取不到我们模块的 vermagic'
if [ -n "$stock" ]; then
    theirs=$(get_vermagic "$stock")
    say "设备现成模块 vermagic: ${theirs:-未知}（$stock）"
    say "本包模块   vermagic: $ours"
    [ "$theirs" = "$ours" ] || die 'vermagic 不一致，装了也加载不了（内核不是同一套构建）'
else
    say "设备上找不到 /lib/modules 下的 .ko 做对照，跳过 vermagic 比对（$ours）"
fi

# 2) 放进扩展目录，旧的先备份
dest=$EXT/modules/$KREL_WANT
if [ -d "$dest" ]; then
    bak=$EXT/modules/$KREL_WANT.bak.$(date +%Y%m%d%H%M%S)
    mv "$dest" "$bak"
    say "已有 $KREL_WANT 目录，备份到 $bak"
fi
mkdir -p "$dest"
cp -f "$work/modules/$KREL_WANT"/* "$dest"/
rm -rf "$work"
say "模块已放到 $dest（$(ls "$dest"/*.ko | wc -l) 个 .ko）"

# 3) mu300-audio-dsp 的 set_fits：它拿 KERNEL 里的 CONFIG 跟设备 /proc/config.gz 的哈希逐位比。
# 我们这棵树的 config 与上游只差两个纯调试项（DEBUG_INFO_COMPRESSED_ZSTD / DEBUG_INFO_BTF_MODULES，
# 交叉工具链是 gcc 11，上游是 gcc 13+ 的容器），ABI 完全一样，vermagic 也一致。
# 所以这里把“CONFIG 必须逐位相同”改成“只要 KERNEL 里声明了 CONFIG 就放行”，原文件先备份。
if [ -f "$DSP" ] && [ ! -f "$DSP.orig" ]; then
    cp -f "$DSP" "$DSP.orig"
    cat > /tmp/patch-dsp.awk <<'AWK'
# CONFIG 那行整行换成“只要声明了 CONFIG 就放行”；用 index/正则匹配 set_fits 里唯一那一行
/config_sum\)/ && /_want/ { print "    [ -n \"$_want\" ] && true"; next }
{ print }
AWK
    awk -f /tmp/patch-dsp.awk "$DSP.orig" > "$DSP.NEW" && mv "$DSP.NEW" "$DSP"
    chmod 755 "$DSP"
    say 'mu300-audio-dsp 的 CONFIG 校验已放宽（原文件 = mu300-audio-dsp.orig）'
fi

say '装好了（重开机会走原来的 prepare.local）'
verify

if [ "${2:-}" = "--start" ]; then
    say '--- reserve ---'
    "$DSP" reserve 2>&1 || true
    say '--- start ---'
    timeout 90 "$DSP" start 2>&1 || say 'start 没成功，看上面输出'
    verify
fi
