# 脚本索引

按内核线分组。**新增脚本请放进对应内核的目录，别再往根目录扔。**

## 6.18/

- `装6.18音频模块.sh` —— 把自编的 25 个 `.ko` 装进 `/mnt/mu300-disk/extra/airsim-audio/modules/6.18.60/`，
  比 vermagic、改名备份旧目录、从 `.orig` 一次生成两处 `mu300-audio-dsp` 补丁。
  `--install` 装，`--verify` 只查。
- `airsim-audio-retry.init` + `装音频抢占.sh` —— 开机多轮抢占 DSP 保留页（A 方案）：
  独立 rc 脚本（`START=11`，装到 `/etc/init.d/airsim-audio-retry`），不覆盖上游的 `airsim-audio-reserve`，
  上游更新顶不掉它。清页缓存 + `retry` 轮询，日志 `/tmp/airsim-audio-claim.log`。

## 5.4/（冻结）

- `切内核-5.4.sh` —— 切回 5.4 内核
- `音频暂存-空列表保护.sh` —— 空列表保护
- `19-mu300-fw4-钩子版.sh` —— 钩子版修复

## 根目录（与内核无关）

- 修断网脚本已随 5.4 线归档：见 `archive/5.4-pkg/修断网.sh`（6.18 线不需要，`flow_offloading=0` 已在设备落盘）
- `通话实时监控.sh` —— 盯 `/status` 的通话帧数
- `mu300-at-补stdin.sh` —— AT 通道补 stdin
- `bump-version.sh` —— 版本号三处同步（`VERSION`、`PACKAGING-VERSION`、`AGENT-VERSION`），遇非 6.18 线直接拒绝
