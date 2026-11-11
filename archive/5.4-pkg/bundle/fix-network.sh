#!/bin/sh
# F50 客户端「无互联网」一键修复 / 体检（在设备上以 root 执行）
#
# 症状：设备自身能上网（ping、curl、LuCI 都正常），但连热点的手机/电脑显示"无互联网"。
# 根因：fw4 渲染的 flowtable 里含 sipa_eth0（SIPA 委托出来的自定义 ARPHRD 虚拟口），
#       5.4 内核在整表事务里建这个 flowtable 会失败 → 整张防火墙规则表拒绝加载 →
#       跑的还是开机 S19 那版（那时 br-lan / sipa_eth0 都还没建）→ accept_to_wan 为空，
#       而 forward 的 policy 是 drop → 客户端包全被丢。
#       设备自身流量走 output 链、不经过 forward，所以"设备能上网、客户端不能"。
#
# 修法：关掉软件流卸载，fw4 就不再生成 flowtable，事务冲突从源头消失。
#       副作用只是转发不走 flowtable 快路径，LTE 几十兆量级 CPU 看不出差别。
#
# 用法：
#   sh 修断网.sh --check    只体检，不改动
#   sh 修断网.sh --fix      体检 + 修复 + 复验
set -eu

fail() { echo "ERROR: $*" >&2; exit 1; }

case "${1:---check}" in
 --check|--fix) mode=$1 ;;
 --help) echo 'Usage: sh 修断网.sh --check | --fix'; exit 0 ;;
 *) fail '未知参数，用 --check 或 --fix' ;;
esac

[ "$(id -u)" = 0 ] || fail '请在设备上以 root 执行'

say() { printf '%s\n' "$*"; }

say '== 1. 拨号状态 =='
up=$(ubus call network.interface.wan status 2>/dev/null | jsonfilter -e '@.up' 2>/dev/null || echo unknown)
l3=$(ubus call network.interface.wan status 2>/dev/null | jsonfilter -e '@.l3_device' 2>/dev/null || echo unknown)
say "  wan.up=$up  l3_device=$l3"
[ "$up" = true ] || say '  ⚠ wan 没起来，先解拨号问题（见 docs/F50-数据联网修复.md 的 sipa 部分）'

say '== 2. 出网（设备自身）=='
if ping -c 2 -W 4 223.5.5.5 >/dev/null 2>&1; then say '  OK 设备自身出网正常'; else say '  ⚠ 设备自身也不通，先修拨号/DNS'; fi

say '== 3. 软件流卸载开关 =='
offload=$(uci get firewall.@defaults[0].flow_offloading 2>/dev/null || echo '?')
say "  flow_offloading=$offload （正确值必须是 0）"

say '== 4. flowtable 是否泄漏进来了 =='
if nft list flowtables inet fw4 2>/dev/null | grep -q 'flowtable ft '; then
 say "  ⚠ 存在 flowtable："
 nft list flowtable inet fw4 ft 2>/dev/null | sed 's/^/    /'
 bad=1
else
 say '  OK 没有 flowtable 对象'
 bad=0
fi

say '== 5. wan 方向规则是否真的加载 =='
acc=$(nft list chain inet fw4 accept_to_wan 2>/dev/null | grep -c 'sipa_eth0' || true)
if [ "$acc" -ge 1 ]; then say "  OK accept_to_wan 里有 sipa_eth0（$acc 条）"; else say '  ⚠ accept_to_wan 为空 → 整表没加载成功'; bad=1; fi
nat=$(nft list chain inet fw4 srcnat_wan 2>/dev/null | grep -c masquerade || true)
say "  srcnat_wan masquerade 规则：$nat 条（应为 2：v4+v6）"

say '== 6. 客户端转发计数 =='
nft list chain inet fw4 accept_to_wan 2>/dev/null | grep 'sipa_eth0' | tail -1 | sed 's/^/  /'

if [ "$mode" = '--check' ]; then
 say ''
 say '体检结束（未改动）。要修：sh 修断网.sh --fix'
 exit 0
fi

say ''
say '== 修复：关闭软件流卸载 =='
uci set firewall.@defaults[0].flow_offloading='0'
uci commit firewall
/etc/init.d/firewall restart >/dev/null 2>&1 || fail '防火墙重启失败'
sleep 2

say '== 复验 =='
if nft list flowtables inet fw4 2>/dev/null | grep -q 'flowtable ft '; then
 fail '仍是存在 flowtable，修复未生效'
fi
nft list chain inet fw4 accept_to_wan 2>/dev/null | grep 'sipa_eth0' | tail -1 | sed 's/^/  /'
say ''
say '完成。客户端重新连一下热点即可（DHCP 租约不用换）。'
say '注意：以后不要在 LuCI 防火墙页勾"软件流量分载"，一勾这个坑就回来。'
