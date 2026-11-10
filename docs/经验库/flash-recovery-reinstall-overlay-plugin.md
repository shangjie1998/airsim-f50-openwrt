---
name: flash-recovery-reinstall-overlay-plugin
description: 刷机后自建 OpenWrt 插件装不回、服务起不来或语音失效时，按五处坑逐层排查（基础层缺失、vendor CLI 契约变化、厂商工具链丢失、内核模块强绑、音频需干净启动）
metadata:
    type: work_experience
    source: unknown
    created_at: "2026-11-10"
    tags:
        - openwrt
        - 刷机恢复
        - 打包
        - rpcd
        - 内核模块
        - vendor-cli
        - 静默失败
---
## 适用场景
嵌入式 OpenWrt 设备重新刷机后，自建插件整棵树丢失、`ubus` 里没有对象、`/etc/init.d/<svc>` 不存在，或装回后服务起不来、语音/音频链失效。

## 关键做法
1. 打包时先铺上游基础层，再叠 overlay（同名文件以 overlay 为准）：
```sh
rsync -a "$REPO/src/plugin/packaging/openwrt/root/" "$STAGE/files/"
rsync -a "$REPO/overlay/<board>/files/"          "$STAGE/files/"
```
2. 安装脚本用「核心二进制是否存在」判定首次安装：首次不备份、直接铺，并 `uci set <svc>.main.enabled=1` + `commit` + `/etc/init.d/<svc> enable`（上游默认 `enabled='0'`，否则装完是停的）。
3. 服务起不来先按顺序查：`ubus list` 有没有对象 → 启动前置脚本的 `command -v` 缺哪个工具 → `/etc/config/<svc>` 的 `enabled` 值 → 模块 `vermagic` 是否匹配。
4. 厂商工具链（alsa 小工具、软链、固件 bin）通常不在插件包里，但上一份根文件系统（`/mnt/.../openwrt.old`）往往还在，按需拷回。
5. 音频链要求模块与运行内核完全匹配，且要一次干净启动（占页模块只抢一次）。
6. 判断内核变没变的两个判据：旧根文件系统 `/lib/modules/` 里出现过的目录名（OpenWrt 只为自己那条内核生成），以及 `/etc/mu300/image-version` 之类的厂商镜像版本串。若 OpenWrt 发行串（`DISTRIB_REVISION`）新旧一致而内核不同，说明变化来自厂商镜像的新构建，不是系统升级。

## 踩过的坑
- **只打 overlay 包会得到"文件都在、服务起不来、`ubus list` 无对象"**：`ubus` 对象、`/etc/init.d` 主脚本、启动前置脚本都在上游打包层，正常设备靠上游 apk 包装好，刷机后那层没了。必须先补基础层。
- **vendor CLI 的调用契约会在固件更新里变**：业务程序用 `mu300-at -t N -`（`-` 从 stdin 喂命令，为了一次送 AT+CMGS+PDU+Ctrl-Z），新版脚本只接受 argv，`-` 被当成一条 AT 命令发出去，回 `+CME ERROR: 0`。表现是手工跑正常、程序里读不到号码。定位方法：把程序里的调用原样复现，再和参数模式对比。恢复：换回旧镜像里兼容版脚本（新版留 `.fw-new` 备查），长期正解是探测 stdin 支持再回退 argv。
- **模块版本不能看目录名**：同名 release 串下也可能是不同 ABI，跨版本 `insmod` 报 `struct module size must match` 绕不过去，必须用 `strings x.ko | grep vermagic` 比对。
- **音频装好后必须重启一次**：占页模块只抢一次固定物理窗口，重试脚本抢不回来只能重启；不要在旧会话里反复 `start`。
- **过渡手段**：给启动前置脚本加显式开关文件（如 `/etc/airsim/no-audio`），存在就跳过 DSP，让插件本体先起来（页面/短信/推送可用、通话无声），且升级不自动清除，等模块对齐后手工删。

## 可复用的产物或模板
（无）

## 补充坑（0.1.0.16 装机实测 2026-11-10）

**装包会把 uci 配置打回默认**：包里 `files/etc/config/airsim` 是 `enabled=0`，
`( cd files && tar cf - . ) | ( cd / && tar xf - )` 一路铺下去就盖掉了设备上的
`enabled=1` 和 relay 设置，症状是 `status` 报 `active with no instances`、
进程数为 0，而不是报错。修法：备份段之后、铺完 files 立刻用 `$BK/config/*`
回写；包里配置只当首次安装的默认值。

**包里二进制可能是旧构建**：`make-package.sh` 不带 `--build` 时直接拿 `out/f50/`
里上一次的二进制，VERSION 文件却是新版本号 → 页面显示 0.1.0.16、`strings` 里还是
0.1.0.15，而旧二进制的 `--interface` 语义不同，两个 procd 实例会同时抢
`127.0.0.1:8575`，日志刷 `bind: address already in use` + `crash loop`。
判据：`strings /usr/lib/airsim/airsim-mu300d | grep 0.4.7-mu300` 必须与 VERSION 一致。

**装完出现 `running (1/2)`**：旧 agent 还没退、新 agent 抢不到端口。
`/etc/init.d/airsim restart` 一次即恢复两实例，不必重装。
