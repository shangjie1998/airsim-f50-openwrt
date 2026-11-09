# 上游原件 r23

- 来源: https://github.com/kai-wu-cortex/airsim-mu300/releases/download/v0.1.0-r23/airsim-mu300-0.1.0-r23-sources.tar.gz
- 本地解包树: `/tmp/r23`（临时，丢了按上面链接重下即可）
- 标记: git tag `upstream-r23`（本目录所在提交即上游原样快照的锚点）
- 说明: 30MB 源码包不入库（GitHub 自身已有 Release 附件）；本目录只记来源与校验值，`src/` 里保存的就是这份原样的解包结果，未经任何个性化改动。

## 与 r22 的差异文件清单（供对照）

- `AirSIM/module/module-agent/{cloud_audio_policy.go, cloud_call.go, cloud_pcm_buffer.go}`
- `plugin/cmd/airsim-mu300d/{direct.go, main.go, management.go}`
- `plugin/internal/audio/devices.go`
- `plugin/docs/{CLOUD_AUDIO_BUFFER.md, RELEASE_v0.1.0-r23.md, TAILSCALE_PAIRING.md}`
- `plugin/packaging/openwrt/root/www/luci-static/resources/view/airsim/{manage.js, overview.js}`
- `plugin/patches/AirSIM-mu300.patch`, `plugin/scripts/{install.sh, package-apk.sh, manage.test.mjs}`
- `plugin/VERSION` = `0.1.0-r23`
