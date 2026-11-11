# 音频（语音）总览

> 内核支持的唯一口径在 [../内核矩阵.md](../内核矩阵.md)。这里只讲"语音到底怎么来的、怎么判、坏了查哪"。

## 一、三档内核，三种来源

| 内核 | 驱动从哪来 | 要不要配件包 | 判据 |
|---|---|---|---|
| `5.4.254`（厂商） | **厂商镜像自带**：`saudio.ko`、`snd-soc-sprd-dai.ko`、`snd-soc-sprd-pa-ucp1301.ko`、`snd-soc-sprd-vaudio.ko` 等 24 个 `.ko` | ❌ 不要装（6.18/7.3 那套跟它不通用） | `audio_reserve missing=0` |
| `6.18.60` | 自编 25 个 `.ko`，落地 `/mnt/mu300-disk/extra/airsim-audio/modules/6.18.60/` | ✅ `audio-opt 1.0.3` | 25 件齐 + `missing=0` |
| `7.3.1` | 自编 25 个 `.ko`，落地 `modules/7.3.1/` | ✅ `audio-opt 1.0.3` | 同上（真机未验） |

**一句话记法**：mainline 内核（6.18 / 7.3）要自己编；厂商内核（5.4）人家本来就全。

## 二、判「真能用」四连（缺一条就不算）

```sh
cat /sys/module/audio_reserve/parameters/missing   # 必须是 0
lsmod | grep -c '^snd'                             # 5.4 实测 11 个
ls /dev/snd                                        # controlC* / pcmC* 要有
cat /proc/asound/cards                             # 期望：Loopback + sprdphone-sc2730
```

再往上才是业务层：`ubus call airsim-call call_status` 看 `service_running`，
真通话时看 `/status` 里 `downlink_frames` / `uplink_frames` 有没有涨。

## 三、常见坏法与去哪查

| 现象 | 大概率原因 | 看哪篇 |
|---|---|---|
| 重启后 `missing≠0`，这次启动补不回来 | 音频内存被 slab/匿名页占住，需要再重启一次 | [重启丢音频.md](重启丢音频.md) |
| `.ko` 装不上，报 `struct module size must match` | vermagic 必须**字面相等**，内核变了就得重编 | [自编过程.md](自编过程.md) |
| 上游 kit 下不到 | `kits.lock` 只钉了 `6.18.58/59`，链接全 404 | [自编过程.md](自编过程.md) |
| 想给新内核出一套驱动 | 造 kit → 编 25 件 → 校验 vermagic → 打配件包 | [配件包.md](配件包.md) |
| 页面「音频模块」显示缺件 | 见 `ubus call airsim-audio audio_opt_probe` 的 `modules_ok / ko_missing` | 本文第一节判据 |
| 厂商内核被误报「缺 1 个」 | 拿 25 件清单去判厂商内核了（旧版 bug，已在 `0.1.0.21` 修） | [../内核矩阵.md](../内核矩阵.md) |

## 四、配件包的边界

- 配件包**只带模块**，不带主程序；主包装不装它都能跑（只是没声）。
- 配件包按 `<KREL>` 分目录，装之前先看 `payload/` 里有没有你这一档。
- 主包 `install.sh` 只负责"提示有没有对应目录"，**不替配件包装任何东西**。

## 五、文件在哪

| 东西 | 位置 |
|---|---|
| 模块源码清单（要编哪 25 个） | `parts/audio/module-list.txt` |
| 编模块脚本 | `parts/audio/build-krel.sh` |
| 打配件包脚本 | `parts/audio/make-opt.sh` |
| 成品 payload | `parts/audio/payload/<KREL>/`（`_` 开头的是待重建占位，打包跳过） |
| 设备落地目录 | `/mnt/mu300-disk/extra/airsim-audio/{bin,ctlset,etc,integration,modules/<KREL>/}` |

## 六、独立页「音频」（2026-11-11）

菜单 `admin/services/airsim/audio`，页面文件 `overlay/f50/files/www/luci-static/resources/view/airsim/audio.js`，跟「蓝牙」页同构，三件事放一起：

| 卡片 | 内容 | 靠什么接口 |
|---|---|---|
| 音频配件包 | 状态 / 本机内核 / 覆盖情况 / 件数 / 已装配件包版本，右上角版本号 + 检查更新 | `audio_opt_probe`、`audio_opt_latest`、`audio_opt_install` |
| 刷机后自检 | 八项一次查完，能补的就地补（原来在「维护」页，已搬到这里） | `selfcheck_run`、`selfcheck_fix` |
| 切换内核 | 三档（5.4 / 6.18 / 7.3）选择 + 本机现成内核包 + 上游 release 号，**只拼命令不代跑** | `kernel_bundles`（只读） |

- 页面右上角版本号是**音频配件包版本**（`audio-opt vX`，设备上实际装着的那个），右边跟「检查更新」图标一组；主包版本不在这页重复显示。，跟「检查更新」查到的最新版比对，才会判"有新版本"（显示成 `v1.0.2 → v1.0.3` + 绿 ↑）。
  之前拿包内钉死的 `TAG` 当已装版本，结果是永远显示"已是最新"——已改成读 `audio-opt-version`。
- 切内核不给执行按钮：要写 boot 分区并重启，重启一律由用户自己来。命令固定为
  `MU300_RELEASE=<release> MU300_KERNEL_BUNDLE=<包> mu300-update kernel <5.4|6.18|7.3>`。

## 七、装完之后页面还是旧样子？

LuCI 给每个视图脚本拼的是**固定的** `?v=<固件版本>`，文件换了 URL 不变，浏览器可能一直用缓存里的旧版。
装完包/换完页面后在页面上按一次 `Cmd+Shift+R`（Windows 是 `Ctrl+F5`）就好；这不是脚本没装上。
