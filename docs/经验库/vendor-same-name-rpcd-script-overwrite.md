# 别用同名文件覆盖固件自带的 rpcd 脚本 / ubus 对象

## 场景
往厂商定制 OpenWrt 设备（中兴/紫光展锐这类，单 ext4 根分区、无 overlay）安装自研插件，
插件需要一个 ubus 对象来喂 LuCI 页面。上游包在别的机型（F50）用的是 `mu300dash`，
到新机型上直接 `cp` 同名脚本到 `/usr/libexec/rpcd/mu300dash`。

## 症状
- `cp` 当下一切正常，rpcd 未重启时 `ubus -v list mu300dash` 仍显示**厂商的方法**
  （`sysinfo` / `sim_get` / `sim_set` / `sim_radio_get` / `sim_radio_set`），
  因为 rpcd 启动时已把旧脚本加载进内存 —— 这个"看起来没坏"的窗口极具欺骗性。
- 重启 rpcd 后对象换成自己的方法，厂商前端（`/www/luci-static/resources/view/mu300/*.js`）
  开始报错，功能静默失效。
- 这类设备**没有 `/rom`、没有 overlay、apk 缓存也常是空的**，
  覆盖即不可恢复；`find / -name '<name>*'` 只剩自己那份。

## 判据（装之前做，30 秒）
1. `ubus -v list <对象名>` —— 对象已存在且方法不是你写的，就停手。
2. `grep -rl '<对象名>' /www/luci-static/resources` —— 有厂商页面引用，就别动它。
3. `ls -l /usr/libexec/rpcd/<name>` + `apk list --installed | grep <name>` ——
   存在即固件文件。
4. `find /rom -name <name>` / `mount | grep overlay` —— 有备份才谈得上"可以覆盖再还原"。

## 正确做法
- **用自己的命名空间**：`airsim-sms` 之类，前端 `rpc.declare({object:'airsim-sms'})` 跟着改，
  ACL 里同样写自己名字（结构仍是"对象→方法数组映射"）。
- 安装器加**固件文件守卫**：装完打印厂商文件的 md5，卸载器再打一次，两次一致才算没碰过。
- 安装器/卸载器只 `rm` 自己明确列出的路径，绝不用通配符扫 `/usr/libexec/rpcd`。

## 卸载可逆性要连状态一起想
`/opt/<插件>/data` 里的已读状态、id 映射随卸载一起没了 —— 卸载后重装短信全部变未读，
这是预期，不是 bug，交付文档里要说清楚。
