# 已保存的 Wi-Fi 还要再输密码？先分清是「密码」还是「接口没建」

## 适用场景

LuCI 联网页上明明「密码已保存」，点连接仍要求输密码；或者输对了也连不上，
页面卡在 `wifi-pending`，`wlan0` 关联成功却没有 IP。

## 结论：这是两个独立故障，现象很像，根因完全不同

### 故障一：前端不认「已保存」，后端也不给复用

- 后端 `wifi_connect` 原来只校验本次请求里有没有 `key`，完全不看 uci 里早存着的那份，
  所以空密码一律报「缺少密码」。
- 前端扫描列表里点同一个 SSID 仍要求填密码；「已保存」那张卡片只有删除，没有连接。

修法（2026-11-14 已落地）：

- 后端：没传 `key` 时，若 SSID 与 uci 里已存的是同一个且存过 key，直接复用；
  **只认同一个 SSID**，不拿旧密码去试新网络。
- 后端：已经连着同一个 SSID 且有地址时直接返回成功，**不再 reload**（否则点一下要断几秒网）。
- 前端：已保存卡片加「连接」；扫描列表标「已保存」并换成「直接连接」；
  手动输入框留空时不再猜成开放网络（原来会传 `none`，把已存的 psk2 配置改坏）。

### 故障二（更阴）：`network.wwan` 和 `wireless.wwan0` 分裂

**这才是「怎么都连不上」的真因。**

`netmode` 原来只写 `wireless.$STA.disabled`，但 **netifd 只认 `network.wwan.disabled`**。
当 `wireless.wwan0.disabled=0` 而 `network.wwan.disabled=1` 时：

- `ubus list network.interface.*` 里**根本没有 `wwan`**（这是最快的判据）；
- `ifup wwan` **静默成功返回、实际什么都没做**；
- 永远没有 DHCP，`check_wifi` 30 秒超时 → `rollback` → 页面停在 `wifi-pending`。

修法：`set_sta()` 把 `network.$STA_NET.disabled` 一起写（接口名从 `wireless.$STA.network` 读，
默认 `wwan`），`ifup` 也用这个变量，不再写死 `wwan`。

## 判据速查

```sh
# 两者必须相等；不等就一定拿不到 IP
echo "wireless=$(uci -q get wireless.wwan0.disabled) network=$(uci -q get network.wwan.disabled)"

# 看不到 network.interface.wwan = netifd 根本没建这个接口
ubus list network.interface.* | grep wwan

# 关联了但没 IP：先查上面两条，再看这个
iwinfo wlan0 info | grep ESSID ; ip -4 addr show wlan0
```

## 踩的时候我自己判断错的地方

- 看到「密码已保存」但连不上，第一反应是密码逻辑（故障一）。改完仍然连不上，
  说明还有第二层。**「要求输密码」和「连不上」必须分开验证**：
  先 `ubus call airsim-net wifi_connect '{"ssid":"X","encryption":"psk2"}'` 看返回，
  再看 `wlan0` 有没有 IP，最后才去怀疑密码。
- `ifup wwan` 返回 0 不代表起来了。netifd 对不存在的接口就是静默跳过。
  **要验的是 `ubus list` 里有没有这个接口**，不是命令的退出码。
- 现场把 `network.wwan.disabled` 设回 1 复现后，一次修好的验证路径是：
  分裂状态 → 跑新 `netmode wifi` → `wwan=0` 且 `mode=wifi` 且 `wlan0` 拿到 192.168.50.230
  且只留一条默认路由，四项全中才算过。
