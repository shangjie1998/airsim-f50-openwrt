# 补丁层规则（改上游 Go 只用这条路）

`src/` 永远保持上游原样。凡是必须动上游 Go 源码才能实现的功能，一律写成
`overlay/f50/patches/*.patch`，由 `overlay/f50/apply.py` 在合并树里按 `patch -p1 -N --batch` 打。

## 硬规则

1. **打不上就失败，不许静默跳过。**
   `apply.py` 里任何一个补丁 `returncode != 0` 直接 `sys.exit(1)`，构建当场停下，
   报 `补丁打不上: <文件名>` 并打印 patch 的 stdout/stderr。
   目的：上游改了上下文（函数签名、结构体字段、路由注册位置）时，我们不能"编过去了但功能没了"。
2. **冲突由人修，不由 AI 猜。**
   构建失败时人工改 `patches/*.patch` 里的 hunk 上下文（行的 `-`/`+` 内容按上游新代码对齐），
   修完重跑 `apply.py`，再编译。**不要**改成 `--fuzz`、`-l`、`--forward` 之类的容错参数绕过。
3. **上游自己合并了同功能，就删我们的补丁、跟随上游。**
   发现上游已经加上（例如 `/pair/clients` 路由出现在 `src/plugin/cmd/airsim-mu300d/main.go`），
   就把对应 `.patch` 文件删掉，让 `apply.py` 少打一个；不要和上游同功能两份并存。
4. 补丁只做"加法"，不改上游既有逻辑的语义；确实要改行为时，在文件头注释里写清改了什么、为什么。
5. 每次加补丁都在 `docs/SYNC-M8-F50.md` 第九节记一笔：补的是哪个功能、动哪两个文件、冲突了怎么修。

## 登记表与体检

`overlay/f50/patches/manifest.tsv` 是登记表，一行一个补丁，四列：
补丁文件 / 上游仓库（空=没有上游对照）/ 已合并标志串 / 补丁依赖的锚点串（分号分隔）。

`scripts/check-patches.py` 读它做两件事，`overlay/f50/build.sh` 会在编译前自动跑：

- **本地 dry-run**：在干净的 `src/` 副本上试打，结果「干净 / 打不上」。打不上说明上游改了上下文。
- **上游对照**：拉 `kai-wu-cortex/airsim-mu300` 最新文件（注意上游没有 `plugin/` 这层打包前缀，
  脚本会自动去掉），命中「已合并标志串」就标记 **已合并→删补丁**；锚点串缺失标记「锚点缺失」。

手动跑：`python3 scripts/check-patches.py`（加 `-v` 打印冲突明细）。退出码 1 = 有要处理的事。

当前输出：

```
补丁                                 本地 dry-run   上游合并       锚点
AirSIM-e164-cnum.patch             干净           -          -
AirSIM-pair-clients.patch          干净           未合并        都在
```

## 现有补丁

| 补丁 | 动哪里 | 干什么 |
|---|---|---|
| `AirSIM-e164-cnum.patch` | `AirSIM/module/module-agent/sms.go` | 本机号码统一成 E.164（`8613…`/`0086…` → `+8613…`） |
| `AirSIM-pair-clients.patch` | `plugin/internal/gateway/gateway.go`、`plugin/cmd/airsim-mu300d/main.go` | 加 `GET /pair/clients`：逐客户端"上次使用 / 在线"（鉴权成功时记时间戳，120 秒窗口判定在线） |
