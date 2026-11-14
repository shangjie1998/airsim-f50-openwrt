---
name: mu300-at-stdin-mode-mismatch
description: 插件报 "AT request failed" 但手动同条 AT 正常时，如何判断是设备侧 AT 包装脚本不认 `-`（stdin 读命令）的版本不匹配，而不是基带坏了
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-07
 tags: [openwrt, at-command, stdin, wrapper-script, version-mismatch, mu300, silent-failure]
---
## 适用场景
上游组件（Go/二进制服务）通过 shell 包装脚本下发 AT 命令，接口报"AT 请求失败"，但用同样的 AT 命令手动执行却返回 OK，且基带侧 `AT+CGATT`/`AT+CREG` 都正常时。

## 关键做法
1. 先读上游源码里真正的调用形式，不要假设是 `mu300-at "AT+XXX"`。本案例是 `exec.CommandContext(ctx, "/usr/bin/mu300-at", "-t", "6", "-")` 配 `cmd.Stdin = command+"\n"`——用的是**从 stdin 读命令**的新约定。
2. 用同样的 argv 在设备上手工复现：`printf 'AT+CLCC\n' | /usr/bin/mu300-at -t 6 -`。若返回 `+CME ERROR: 0` 且 rc=0，说明脚本把 `-` 当成了 AT 命令本身发给基带，是包装脚本版本落后，不是基带故障。
3. 对照上游最新版同一脚本的对应片段，只补最小差异（本案例四行：`if [ "$cmd" = - ]; then cmd=; IFS= read -r cmd || [ -n "$cmd" ] || cmd=; fi`），先 `sh -n` 语法检查，原文件备份后再覆盖。
4. 改完两条路径都要验：stdin 路径返回 `OK`、argv 路径仍返回 `OK`，再重启服务看上游报错字段是否清空。

## 踩过的坑
- 报错文案"AT request failed (AT+CLCC)"会把人往基带/信号方向带。手动 `mu300-at 'AT+CLCC'` 成功并不能证明通道没问题——它走的是 argv 路径，和上游实际调用不是同一条。
- 包装脚本对未知参数不报错，而是原样发给 modem，于是错误以 `+CME ERROR` 形式出现且退出码为 0，属于静默失败。
- 直接用上游最新脚本整份覆盖风险更大（体积大、可能改掉本机适配），只补最小差异更安全。

## 可复用的产物或模板
先复现：`printf 'AT+CLCC\n' | /usr/bin/mu300-at -t 6 -`
补丁片段：
```sh
cmd=${1:-}
if [ "$cmd" = - ]; then
    cmd=
    IFS= read -r cmd || [ -n "$cmd" ] || cmd=
fi
```
原文件备份 `mu300-at.bak-20261107`。

## 已固化（2026-11-14）
上面那个「改设备上的脚本」是治标：2026-11-14 刷机之后又原样复发，因为固件把 `/opt/mu300/bin/mu300-at` 重置回了不认 `-` 的版本。

现在改成**兼容层进包**，位置 `overlay/f50/files/usr/bin/mu300-at`：

- `/usr/bin/mu300-at` 原本是 `-> /opt/mu300/bin/mu300-at` 的软链，包里的真实文件会盖掉这个软链，厂商脚本本身不动。
- 兼容层不判定版本，只 `grep` 真脚本里有没有 `if [ "$cmd" = - ]`：有就 `exec` 原样带 `-` 转交；没有就自己 `IFS= read -r` 读 stdin 那一行，按 argv 形式传过去。
- 这样厂商脚本将来自己支持 `-` 了也不会打架，刷机也不需要再手改设备，装我们自己的包就自愈。

验证要三条路径全绿（缺一条都说明只修了一半）：

```sh
printf 'AT+CNUM\n' | /usr/bin/mu300-at -t 6 -    # agent 走这条
/usr/bin/mu300-at 'AT+CPIN?'                    # 人工排查走这条
/usr/bin/mu300-at 'AT+COPS?'                    # 默认超时那条
```

再重启服务复核：`curl -s http://127.0.0.1:8575/api/sim/identity` 有号码、`/api/calls/status` 的 `last_poll_error` 为空。

**注意别被假象带偏**：`mu300-at 'AT+XXX'`（argv 路径）成功**不能**证明通道正常，它和 agent 走的压根不是同一条路。也不能拿 `ubus call mu300.modem at` 的结果下结论——那条是 ubus 直连，同样绕过了包装脚本。