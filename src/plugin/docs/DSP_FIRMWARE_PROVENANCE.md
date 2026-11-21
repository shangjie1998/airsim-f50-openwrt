# DSP 固件来源与许可记录

AGDSP 主固件 `agdsp.bin` 是 Unisoc/展锐专有镜像，**不随主包、配件包或公开 Release 分发**；
上游只做搬运，未提供再分发许可。缺失时由用户自行从上游固定 Release 地址获取，落到设备
`/lib/firmware/agdsp.bin`；已有固件保留复用，不重复下载。

1. 从上游固定 HTTPS Release 地址下载（地址见下表）。
2. 文件必须通过同一个固定 SHA-256 校验；校验不符不得安装。
3. 校验通过后由 `mu300-audio-dsp install <文件>` 原子落位，并把哈希记入 `/etc/mu300/agdsp.sha256`。
4. 主包与配件包的打包链路不再内置该固件副本；自用下载、局域网喂入、私有网盘私有目录留存可接受，
   public 仓、公开 Release、匿名分享链接、二次分发违规。

## 来源与固定身份

| 项目 | 固定值 |
| --- | --- |
| 上游仓库 | https://github.com/dikeckaan/mu300-audio-firmware |
| 原始提交 / v1.0 | `8a3fbd2cd0556796d0ffef5be05b4c21a743f868` |
| 文件 | `agdsp.bin`，6,291,456 字节 |
| SHA-256 | `378ceea78dbf013e10625e71efd6fab487d727e371884ae8daf078da2828b937` |
| 固定下载地址 | https://github.com/dikeckaan/mu300-audio-firmware/releases/download/v1.0/agdsp.bin |

副本从公开上游提交提取，不从用户设备导出。保留上游原始 Git 对象、提交和作者记录；不改称本项目自研固件。该二进制与内核/音频模块仍是不同组件，备用下载机制不扩大硬件支持范围。

## 本地留存

固件本地归档在 kits 仓 `archive/firmware/`（含 `agdsp.bin`、`PROVENANCE`、`SHA256SUMS`），
仅供私有备份/自用恢复，排除于公开代码提交之外；不再有随主包分发的副本，也不提供离线打包脚本。
需要时先核对上表 SHA-256，再落到设备 `/lib/firmware/agdsp.bin`。

## 许可记录

2026-11-02 核查，上游仓库和 v1.0 发布没有 LICENSE 或明确再分发许可文件。原作者及固件权利人的权利保留；本项目的 PolyForm/GPL/MIT 许可不覆盖此二进制，也不将公开可下载视为获得开源许可。早期 R18–R23 曾把固定副本纳入签名 APK 离线回退候选，该 complete-install 机制已退役；现行打包不再内置固件副本。此记录不代表已经取得或确认独立固件的再分发许可，本文件不授予任何额外权利。
