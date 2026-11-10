---
name: probe-script-silent-hang-timeout-pidfile
description: 远端长驻采样探针进程还在但日志不再增长时，如何判定是轮询命令卡死，以及用什么写法与存活校验避免丢现场数据
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [探针, shell, wget超时, pid文件, 静默失败, 远程调试, ssh]
---
## 适用场景
在远端设备上挂长驻 shell 探针（每 N 秒采音频/进程/HTTP 计数）抓偶发现场。这类任务一次采样失败就没法重来，探针必须在长时间无人看管下既不卡死，也能自证还在干活。

## 关键做法
- 探针里每个外部命令都要有上限：`wget -q -T 2 -t 1 -O- URL`；只读本地 `/proc`、`/sys` 节点一般不会挂，可放心直读。
- 存活校验看日志是否在增长（间隔几秒两次 `wc -l` 或看末尾时间戳），不要只看 `pgrep` 有没有进程。
- 后台实例自己落 pid：`echo $$ > /tmp/x.pid`，要停就 `kill $(cat /tmp/x.pid)`。
- 跨 ssh 启动必须彻底脱离：单独一条 ssh 命令里执行 `setsid sh /tmp/x.sh < /dev/null > /dev/null 2>&1 &`，别和后续查询挤在同一条命令里。

## 踩过的坑
- `wget -qO- http://127.0.0.1:PORT/status` 没加超时。对端某次不响应时整个采样循环永久阻塞，日志停在某一秒不再增长，而 `pgrep` 照样能查到进程，看上去一切正常——结果是整通电话的现场数据全丢，事后才发现。判断只能靠日志时间戳/行数。
- `pgrep -f x.sh` 会匹配到执行它的那条 ssh 命令行本身，用它杀进程时会把刚启动的实例一并清掉，越杀越乱，用 pid 文件才稳。

## 可复用的产物或模板
```
LOG=/tmp/x.log; echo $$ > /tmp/x.pid; : > "$LOG"
while :; do
  echo "=== $(date '+%H:%M:%S')" >> "$LOG"
  cat /proc/asound/card0/pcmXp/sub0/status 2>/dev/null >> "$LOG"
  wget -q -T 2 -t 1 -O- http://127.0.0.1:PORT/status | head -c 400 >> "$LOG"
  sleep 2
done
# 启动（单独一条 ssh）：
# setsid sh /tmp/x.sh < /dev/null > /dev/null 2>&1 &
# 存活：两次 wc -l /tmp/x.log 对比行数是否增长
# 停止：kill $(cat /tmp/x.pid)
```