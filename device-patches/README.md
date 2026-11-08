# 设备侧补丁（从 F50 实机取回）

这些文件不在 `luci-app-airsim` 安装包里，是 `/opt/mu300` 运行库（来自 `mu300-linux`）上的改动。
它们让 F50 在**5.4 厂商内核**上真正跑通通话与拨号。2026-11-08 直接从设备取回，哈希与设备逐字节一致。

| 文件 | 角色 | sha256 | 字节 |
|---|---|---|---|
| `mu300-at` | 打过补丁（设备当前用这份） | `6d9bfab8…0d26` | 10340 |
| `mu300-at.orig` | 设备上的原始副本（`.bak-20261107`） | `b639d36e…7381` | 10258 |
| `sipa-dele-start` | 打过补丁（设备当前用这份） | `6503ec6a…9667` | 5226 |
| `sipa-dele-start.orig` | 设备上的原始副本（`.bak-20261107`） | `b701fc23…ff60` | 5024 |
| `run-dir.sh` | 新 rootfs 被裁掉的运行库文件（从上游补回，原样） | `ff9a30a8…6613` | 4069 |

差异见 `mu300-at.diff` / `sipa-dele-start.diff`，就两处，都是十几行：

## 1. `mu300-at`：认 `-`（stdin）

```
+ if [ "$cmd" = - ]; then
+     cmd=
+     IFS= read -r cmd || [ -n "$cmd" ] || cmd=
+ fi
```

设备自带这份不认 `-`，把连字符当 AT 命令发出去 → 插件来电轮询 `AT+CLCC` 永远失败，手动打同一条却正常。
上游新版有这段，所以是从上游补回、不是自创。

## 2. `sipa-dele-start`：5.4 上没有就绪节点时放行

```
-         log 'missing or ambiguous kernel readiness interface; matching delegate module required'
-         return 1
+         if ! grep -q '^sipa_dele ' /proc/modules; then
+             log 'missing or ambiguous kernel readiness interface; matching delegate module required'
+             return 1
+         fi
+         log 'no readiness sysfs on this 5.4 vendor driver build; module loaded after registration, proceeding'
+         return 0
```

5.4.254 的 `sipa-dele.ko` 只有 `sipa_dele_reset`，**没有** `sipa_dele_wait`（6.18 才有）。
原逻辑把"读不到就绪节点"当硬失败，于是日志里那句
`sipa delegate did not become ready; refusing unsafe PDP activation` 一直拦着拨号。
改成：`/proc/modules` 里能看到 `sipa_dele` 已加载，就记一行日志放行。

> **6.18 内核上不需要这个补丁**：那边 `sipa_dele_wait`/`sipa_dele_ready` 都在，走原生路径，补丁留着也不冲突。

## 怎么重放

```sh
# 设备上执行，先保原件
cp /opt/mu300/bin/mu300-at /opt/mu300/bin/mu300-at.bak-$(date +%Y%m%d)
cp /opt/mu300/bin/sipa-dele-start /opt/mu300/bin/sipa-dele-start.bak-$(date +%Y%m%d)

# 本仓库里这两个文件直接覆盖（已保留可执行位）
cp mu300-at sipa-dele-start /opt/mu300/bin/
chmod 755 /opt/mu300/bin/mu300-at /opt/mu300/bin/sipa-dele-start

# 缺运行库就补（644，不要可执行）
cp run-dir.sh /opt/mu300/lib/run-dir.sh && chmod 644 /opt/mu300/lib/run-dir.sh
```

也可以按 `.diff` 自己打：`cd /opt/mu300/bin && patch -p0 -i <仓库>/device-patches/mu300-at.diff`。

## 会丢的时机

`mu300-update apply` 重铺 `/opt` 时，这三处会被覆盖回原样。装完 `mu300-update` 之后要重新放一遍，
并复验：

```sh
printf 'AT+CLCC\n' | mu300-at -t 6 -            # 有输出 = stdin 补丁在
grep -c 'no readiness sysfs' /tmp/mu300-sipa-dele.log   # ≥1 = 拨号放行补丁生效过
```

## 还有一处不在这里

音频暂存的"空列表保护"是**一次性**的：装包时对 `/tmp/aud/integration/stage-device.sh` 打完补丁再跑，
不落盘（重启就没了）。所以它没有可复制的成品文件，重放靠 `scripts/音频暂存-空列表保护.sh`。
