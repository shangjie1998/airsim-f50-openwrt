---
name: no-docker-openwrt-go-plugin-selfbuild
description: 没有 Docker / apk-tools 3.x 时，如何自己交叉编译 OpenWrt Go 插件并出一个可装可回滚的覆盖包
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [openwrt, go, 交叉编译, 无docker, 覆盖安装包, 版本同步]
---
## 适用场景
要给自己维护的 OpenWrt 设备插件出"自己编的包"，但手头没有 Docker、没有 apk-tools 3.x（出不了签名 apk v3）。插件是 Go 写的、随包附带的源码目录是 tarball 拷贝而非 git 克隆。

## 关键做法
1. 二进制只用 Go 就能出，不必等 Docker：`CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=<上游版本>"`，用 `file` 与 `strings` 回读确认是 static aarch64、新串确实进了二进制。
2. 主程序与配套 agent 往往是两个模块，agent 源码由打包脚本从上游仓库 `git archive` 导出，先跑该脚本拿到导出目录再编。
3. 出包改用覆盖式 tar：二进制 + 需要替换的页面/配置 + `VERSION` + `MANIFEST`（首行写本仓库发布号）+ `install.sh`/`uninstall.sh`。install 先把旧文件备份到 `backup-<版本>/` 再替换并重启服务，uninstall 原样回滚；装完用 `head -1 MANIFEST` 与接口返回值核对，而不是只看进程在跑。

## 踩过的坑
- 随包放进源码目录的上游源码是 tarball 拷贝，**没有 `.git`**，导出脚本里 `git -C <dir> cat-file -e <commit>^{commit}` 会报 `fatal: Not a valid object name`。要把环境变量（如 `AIRSIM_UPSTREAM_REPO`）指到真正含该 commit 的克隆，不能指望包里那份。
- 打包目录自带一份 `VERSION`，`MANIFEST` 首行还写一遍版本。只改仓库根那份，打包时按旧值算，装完 MANIFEST 就报出已发布的旧版本。把"计数 +1 并写三处"做成一个脚本当唯一入口，别手改。
- 覆盖包只换二进制与页面，插件自己的 `/usr/lib/.../VERSION` 若被启动前的预检按值校验，要写回上游原值，别被本地计数污染。
- 设备端校验 `curl`/`pgrep`/`nohup` 常有缺失（busybox 精简），安装脚本里的校验要用 `uclient-fetch`、`ps | grep -v grep` 这类稳妥写法。

## 可复用的产物或模板
```sh
export PATH=<本地 go>/bin:$PATH GOPROXY=https://goproxy.cn,direct
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -mod=vendor -buildvcs=false -trimpath \
  -ldflags "-s -w -X main.version=<up> -X main.agentVersion=<agent.ver>" -o out/ ./cmd/...
tar czf dist/<name>-<ver>.tar.gz -C pkg/<name>-<ver> .
```