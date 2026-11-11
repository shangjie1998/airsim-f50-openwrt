# build —— 只放「造包」的东西

**这里只造包，不跑设备运维。** 运维脚本在 `../ops/`，配件在 `../parts/`。
新增造包脚本放这里；新增一次性设备脚本放 `ops/`，别混。

## 文件

| 文件 | 干什么 | 用法 |
|---|---|---|
| `make-package.sh` | 合成 `overlay/f50/files` + `src/plugin/packaging/openwrt/root` + `out/f50` 二进制，打成主包 | `sh build/make-package.sh`（用现成二进制）<br>`sh build/make-package.sh --build`（先重编） |
| `bump-version.sh` | 版本号三处同步（`VERSION` / `PACKAGING-VERSION` / `AGENT-VERSION`），适用范围只认 `any` / `6.18` | `sh build/bump-version.sh <N> r<NN> [any\|6.18]` |
| `pkg/install.sh` | **包内**安装脚本（装到设备上跑，不是本机跑）：备份 → 铺 overlay → 起服务 → 提示内核与语音 | 由 `make-package.sh` 打进包里 |
| `pkg/uninstall.sh` | 包内卸载 | 同上 |
| `pkg/airsim-audio-retry.init` | 开机多轮抢占 DSP 保留页（`START=11`），**随主包发**，装到 `/etc/init.d/airsim-audio-retry` | 由 `make-package.sh` 打进 `audio/` |
| `tools/check-patches.py` | 逐个补丁 `patch --dry-run`，并比对上游是否已合并（命中就提示删补丁） | `python3 build/tools/check-patches.py` |
| `tools/sync-m8.py` | F50 与 M8 两仓库只读比对，按 `shared/rewrite/device` 三档分类 | `python3 build/tools/sync-m8.py --only shared` |

## 主包里都装了什么

```
airsim-f50-<VER>/
├── airsim-mu300d  airsim-agent       ← out/f50 编好的 Go 静态件
├── install.sh  uninstall.sh  VERSION  MANIFEST
├── audio/airsim-audio-retry.init     ← build/pkg/airsim-audio-retry.init
└── files/…                            ← overlay/f50/files 整棵树（rpcd、页面、菜单、ACL、uci、init.d）
```

**不带内核模块**（那是 `parts/audio` 的配件包干的活），**不改内核、不改 boot、不动分区**。

## 相关文档

- 流程与检查点：`../docs/打包发布.md`
- 版本号规则：`../VERSIONING.md`
- 内核支持口径：`../docs/内核矩阵.md`
