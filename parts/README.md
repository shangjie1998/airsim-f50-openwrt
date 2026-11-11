# parts —— 可选配件的源码

主包**不带**配件；缺什么装什么，各回各的配件包。

| 目录 | 是什么 | 产物 |
|---|---|---|
| `audio/` | 语音驱动模块：要编哪 25 个、怎么编、按内核分目录落 payload | `airsim-f50-audio-opt-<ver>-k<KREL>[+…].tar.gz` |
| `bluetooth/` | 展锐 WCN 蓝牙的启用件（`payload/root/` 被 `.gitignore` 挡着，需现生成） | `airsim-f50-bt-opt-<ver>-k<KREL>.tar.gz` |

## audio/ 里有什么

```
AUDIO-OPT-VERSION     当前配件包版本（1.0.3）
module-list.txt       25 个模块清单（编什么、装什么，都按它）
build-krel.sh         造 kit + 编模块 + 收集 + vermagic 校验
make-opt.sh           打多内核配件包
opt/                  配件包自己的 install.sh 等
payload/<KREL>/       已编好的成品（`KERNEL` 文件写明 KEY/KREL/CONFIG/TAG）
payload/_*/           `_` 开头的占位目录，打包时跳过
```

## 三条硬规矩

1. **vermagic 必须字面相等**，内核变一个字符就要重编，别指望复用。
2. **别把模块塞进主包**（会让主包重新绑内核，等于把刚解开的东西又捆回去）。
3. **5.4 不要装配件包**：厂商镜像自带整套驱动，装了反而可能冲突，判据见 [`../docs/内核矩阵.md`](../docs/内核矩阵.md)。

用法细节：`../docs/音频/README.md`、`../docs/音频/配件包.md`。
