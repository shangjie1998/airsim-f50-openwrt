---
name: rsh-wrapped-output-poisons-remote-config
description: 用本机封装的 ssh 辅助脚本从远端取值再写回远端 uci 配置时，抓到的输出混有 ssh 横幅导致配置被写坏，如何避免并校验
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [ssh, rsh, 封装脚本, uci, 配置污染, openwrt, 静默失败]
---
## 适用场景
远程设备上要读一个配置值、加工后再写回该设备（改 uci option、写配置文件），而读写都走本机那种封装了 `ssh` 的辅助脚本（会打印 `spawn ssh ...` 和密码提示）时。

## 关键做法
需要写回的值在本机生成（heredoc / python 写成文件），`scp` 到远端后用
`uci set pkg.sec.opt="$(cat /tmp/value.txt)" && uci commit pkg` 写入。
写完回读校验：`uci -q get pkg.sec.opt` 和配置文件首行（应为 `config ...`）都要干净。

## 踩过的坑
- 用 `./dev-tools/rsh 'cmd' > local.txt` 取值时，文件里除结果外还有 `spawn ssh -o StrictHostKeyChecking=...`、`Warning: Permanently added ...`、`root@host's password:` 三行杂讯；直接拼进 uci option，整个配置项变垃圾。
- 这类污染的后果是静默的：HTTP 仍回 200，只有服务端业务字段报错（本次是推送服务回 `{"code":999,"msg":"服务端验证错误"}`、`token不能为空`）才露出来；若只按状态码判成功会误判。
- 所以对远端命令输出做解析前，先确认取回的原文；对写回后的配置，先回读首行和值再继续。

## 可复用的产物或模板
```sh
# 好：本机生成干净值再推上去
python3 - <<'PY'
open('/tmp/value.txt','w').write('...')
PY
./dev-tools/scp /tmp/value.txt /tmp/value.txt
./dev-tools/rsh 'cp /etc/config/pkg /etc/config/pkg.bak; uci set pkg.sec.opt="$(cat /tmp/value.txt)" && uci commit pkg; head -3 /etc/config/pkg'
```