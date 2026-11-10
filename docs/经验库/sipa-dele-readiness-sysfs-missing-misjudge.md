---
name: sipa-dele-readiness-sysfs-missing-misjudge
description: OpenWrt 上 modem 数据不拨号、日志报 "sipa delegate did not become ready"，如何判断是就绪接口缺失的误判还是真的委托未工作
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-07
 tags: [openwrt, sipa, ipa, unisoc, mu300, pdp, 数据拨号, sysfs]
---
## 适用场景
Unisoc MU300/UMS9620 一类带 SIPA/IPA 委托的 modem 在 OpenWrt 上不拨号，日志出现 `sipa delegate did not become ready; refusing unsafe PDP activation` 或 `missing or ambiguous kernel readiness interface`，但 SIM、注册、APN 都正常。

## 关键做法
先分清两层：一是"内核没暴露就绪接口"，二是"委托线程没工作"，前者不等于后者。
1. 用 `strings /lib/modules/$(uname -r)/sipa-dele.ko | grep -i sipa_dele_wait` 查该版驱动是否真的编了这个属性；没有就是驱动未实现该接口，不是模块选错。
2. 确认模块的 insmod 时机仍在网络注册之后、PDP 激活之前（脚本里 `mobile-data` 先等注册再调 `sipa-dele-start`），这个顺序本身是对的。
3. 把启动脚本的就绪判定改为"模块已加载即继续"，但保留 `grep -q '^sipa_dele ' /proc/modules` 守卫，模块未加载时仍然拒绝——早加载才会 panic。
4. 验证不靠脚本返回值和日志，靠实际流量：`cat /sys/class/net/sipa_eth0/statistics/rx_packets` 是否增长，再 ping 公网、看 `ubus call network.interface.wan status` 的 `up` 与 `l3_device`。

## 踩过的坑
看到"missing kernel readiness interface"容易直接判定硬件不支持或缺模块，转去找新内核的 .ko 或换内核，白绕一大圈。实际 6.18 版模块才有 `sipa_dele_wait_show`，5.4 vendor 版没有该属性但委托照样工作。反过来也不能无条件放行：模块没加载时就放行会重演"委托早于 modem 起来"的 panic。
本次只在热加载/`ifdown wan; ifup wan` 路径验证通过，整机冷启动的自动拨号未单独实测。

## 可复用的产物或模板
把就绪判定改成：
```sh
    [ "$count" = 1 ] || {
        if ! grep -q '^sipa_dele ' /proc/modules; then
            log 'missing or ambiguous kernel readiness interface; matching delegate module required'
            return 1
        fi
        log 'no readiness sysfs on this driver build; module loaded after registration, proceeding'
        return 0
    }
```