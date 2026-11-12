---
name: single-radio-ap-mode-wifi-scan-empty
description: 单射频设备在热点(AP)模式下扫描 Wi-Fi 永远返回空时，如何判断是接口类型不对而不是驱动坏，并改用临时 managed 接口扫描
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-12
 tags: [openwrt, wifi, 扫描, iwinfo, iw, AP模式, 静默失败, 嵌入式]
---
## 适用场景
嵌入式路由器/随身 WiFi 只有一个射频，热点开着时（wlan0 type AP）用户点“扫描附近的 Wi-Fi”拿不到任何结果，或返回空却报“驱动还在忙”这类含糊错误。

## 关键做法
先用 `iw dev <iface> info` 看接口 `type`：只有 managed/station 能直接 `iwinfo <iface> scan`；AP 接口本身不响应扫描请求，拿它扫必然空，且不会报错。所以“当前可用无线接口”不能只按“哪个 netdev 挂着 phy80211”来取（热点模式下取到的就是 AP 口）。判定到非 STA 类型时，临时建一个 managed 接口再扫：`iw phy phy0 interface add scan0 type managed` → `ip link set scan0 up` → **等 2 秒**（新建口立刻扫常常拿空）→ `iwinfo scan0 scan`，扫完 `ip link set scan0 down; iw dev scan0 del` 收尾。扫描仍为空时再重试 2 次、间隔 2 秒，别让用户点一次就吃闭门羹。

## 踩过的坑
一开始把“扫描失败”当成驱动没就绪，只加 sleep 和重试，结果耗时七八秒照样空——真正的错因是把 AP 模式的 wlan0 当成了扫描接口。真机上 `iwinfo wlan0 scan` 返回空、`iwinfo scan0 scan` 立刻能出结果（扫到周边 5G AP），对比就能定位。另外新建 managed 口后必须等驱动就绪，立即扫也是空。

## 可复用的产物或模板
ifc=$(取挂 phy80211 的 netdev)
_t=$(iw dev "$ifc" info 2>/dev/null | awk '/type/{print $2; exit}')
case "$_t" in managed|station) ;; *) ifc='' ;; esac
if [ -z "$ifc" ]; then
  iw phy phy0 interface add scan0 type managed && ip link set scan0 up && sleep 2
  iwinfo scan0 scan; ip link set scan0 down; iw dev scan0 del
fi