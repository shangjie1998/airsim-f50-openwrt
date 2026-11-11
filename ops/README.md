# ops —— 设备侧一次性运维脚本（不随包发）

**跟 `build/` 分界很清楚**：`build/` 造包，`ops/` 上机干活。
这里的脚本都是**人工在设备上跑**的，不打进任何安装包。

| 目录 | 脚本 | 什么时候用 |
|---|---|---|
| `install-audio/` | `装音频模块.sh` | 把自编的 25 个 `.ko` 装进 `/mnt/mu300-disk/extra/airsim-audio/modules/<KREL>/`：比 vermagic、旧目录改名备份、从 `.orig` 生成两处 `mu300-audio-dsp` 补丁。`--install` 装，`--verify` 只查 |
| `install-audio/` | `装音频抢占.sh` | 配 `build/pkg/airsim-audio-retry.init`，开机多轮抢占 DSP 保留页；日志 `/tmp/airsim-audio-claim.log` |
| `kernel/` | `切内核.sh` | 切 5.4 / 6.18 / 7.3。内含两套命令：`rollback-boot`（要求 `prev.*` 还在）与离线喂包（**两个环境变量必给**） |
| `call-monitor/` | `通话实时监控.sh` | 盯 `/status` 的通话帧数，排查「有声但断续」 |
| `call-monitor/` | `mu300-at-补stdin.sh` | AT 通道补 stdin，某些固件上 `mu300-at` 不喂 stdin 会卡住 |
| `legacy/` | `音频暂存-空列表保护.sh`、`19-mu300-fw4-钩子版.sh` | 5.4 时代的临时补丁，**只留档，别再往新机器上装** |

## 规矩

1. **先读再跑**：这些脚本都会改设备状态，跑之前把命令看清楚，尤其 `切内核.sh`。
2. **不许自动化代跑重启**：重启是高危动作，由使用者本人按。
3. **内核相关先看口径**：`../docs/内核矩阵.md`；联网相关看 `../docs/联网.md`。
4. 新的一次性脚本放这里，**别再往仓库根或 `build/` 扔**。
