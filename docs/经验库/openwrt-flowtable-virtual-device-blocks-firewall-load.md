---
name: openwrt-flowtable-virtual-device-blocks-firewall-load
description: OpenWrt 上设备自身能上网但下挂终端无网时，如何判断是 flowtable 含自定义虚拟网卡导致整张防火墙规则拒绝加载
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-07
 tags: [openwrt, fw4, nftables, flowtable, nat, 转发, 静默失败]
---
## 适用场景
OpenWrt 上出现"路由器/设备自身 ping、HTTP、默认路由、DNS 全正常，但接在同一台设备上的终端（Wi-Fi 或网口）显示无互联网"的分离症状。尤其当 wan 侧是自定义虚拟网卡（SIPA、RMNET 之类非标准 ARPHRD 的口）时。

## 关键做法
1. 先分侧：设备自身 `ping` 公网通、`ubus call network.interface.wan status` 报 `up=true`，而客户端已拿到 DHCP 租约 → 问题在 forward/NAT 链，不在拨号、不在 DNS、不在桥接。
2. 看实际生效链：`nft list ruleset | awk '/chain accept_to_wan/,/^\t}$/'`。如果这条链是空的、而 `chain forward` 的 policy 是 `drop`，客户端必然被丢包。
3. 关键判据：链空但 `fw4 print` 能打印出规则，说明配置没问题，是**运行时表没被应用**。此时直接跑 `/etc/init.d/firewall restart` 看输出，出现类似
   `flowtable ft { devices = { ... } }`、`Error: Could not process rule: No such file or directory`、
   `The rendered ruleset contains errors, not doing firewall restart`
   即确认整表渲染失败、跑的是旧表。
4. 修法：`uci set firewall.@defaults[0].flow_offloading='0'; uci commit firewall; /etc/init.d/firewall restart`。另外给 wan zone 显式补 `uci add_list firewall.<zone>.device='<wan 网卡名>'`，别依赖 interface 展开。
5. 验证：`accept_to_wan` 里出现带 counter 的 accept 规则且在增长；`/proc/net/nf_conntrack` 里能看到客户端源地址被 NAT 成 wan 地址、并有对向回包。

## 踩过的坑
- 不要因为链里已经有 `oifname "<wan>" masquerade` 就认定 NAT 正常：NAT 规则在旧表里也会存在，而 forward 的 accept 链是空的，症状仍是客户端无网。
- 重启 `firewall` 后链依然为空，很容易误判成"fw4 不认这个自定义接口/zone 没展开"，从而去改 zone、加 device，白折腾。真正的信号是 restart 的 stderr——一定要把输出打出来看，而不是只看结果。
- 这个失败是静默的：每次开机的日志里可能只有一句渲染错误，后续 walk 全部基于旧表。设备自身流量不经过 forward 链，所以"自己测一切正常"会强烈误导。
- 关掉 flow offloading 会失去软件流卸载，高吞吐时 CPU 略高，但换来规则表能加载。fw4 不支持把单个 zone 排除出 flowtable，要保留卸载得自己写 nft 片段。

## 可复用的产物或模板
```sh
# 判断是否整表未加载
/etc/init.d/firewall restart 2>&1 | tail -6
nft list ruleset | awk '/chain accept_to_wan/,/^\t}$/' | head -8

# 修复
uci set firewall.@defaults[0].flow_offloading='0'
uci add_list firewall.@zone[1].device='sipa_eth0'
uci commit firewall
/etc/init.d/firewall restart

# 验证客户端已 NAT 出网
grep '<客户端 IP>' /proc/net/nf_conntrack | head -4
```