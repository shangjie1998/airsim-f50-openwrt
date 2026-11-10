---
name: confirm-installed-version-by-manifest
description: 需要确认设备上实际跑的是哪一版自建包时，用 MANIFEST 首行加二进制时间戳与进程交叉核对，而不是指望 --version 或 VERSION 文件
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [openwrt, 版本核对, manifest, 装机验证, go-binary]
---
## 适用场景
自建安装包（自编 Go 二进制 + 脚本 + LuCI 资源）装到设备后，要回答"现在跑的是哪一版""某个改动到底上没上机"。规则：只查进程在不在、只看服务 status 为 running，都不足以证明版本。
## 关键做法
从四个地方交叉取证，全部对上才算确认：
1. 包内 MANIFEST 首行通常写着 `<包名> <版本>, installed <时间>`，这是安装器写的，最可靠；
2. `/opt/<包名>/bin` 下各二进制的 mtime 应与 MANIFEST 时间一致，对不上说明后来被单独替换过；
3. `pgrep -f <进程名>` 确认进程在用；
4. `/etc/init.d/<服务> status` 为 running。
再配合版本口径与 Release 包 md5 比对，就能确认设备跑的是不是发布的那一版。
## 踩过的坑
- 包里**没有** VERSION 文件，`find /opt -name VERSION` 一无所获，别按 M8 那条思路去找。
- 自编 Go 二进制不认 `--version`：`airsim-mu300d --version` 直接 `flag provided but not defined: -version` 然后打 usage；`airsim-agent --version` 更误导，它把参数当配置，报"需要有效的三星私网 PCM 地址"。看到这类报错不要当成版本 bug，是根本没有版本子命令。
- 结论：版本号只从 MANIFEST 读，二进制时间戳用于判断"是不是整包一起装的"，进程与服务状态只证明"在跑"。
## 可复用的产物或模板
```sh
head -2 /opt/<包名>/MANIFEST
ls -l /opt/<包名>/bin
pgrep -f <进程名> | head -5
/etc/init.d/<服务> status
```