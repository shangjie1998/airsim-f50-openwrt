#!/bin/sh
# 本地入口：一条命令把真机的音频配件包自测跑完，不用开浏览器。
#
# 用法：
#   sh build/tools/selftest-audio.sh           # 清模块 → 装回来 → 验 ready
#   sh build/tools/selftest-audio.sh --keep    # 不清模块，只验当前状态
#
# 为什么要单独造一个 expect：~/Desktop/f50ssh.exp 的 timeout 是 90 秒，
# 走 GitHub 下载慢的时候会话到点就被掐，安装器跟着被 SIGHUP 杀掉，
# 看上去像「卡在下载」其实是被中断。这里给 600 秒，够装两遍。
#
# 前置：设备上已有 /usr/libexec/airsim-audioopt-selftest（主包带的）
#
# 退出码跟远端一致：0 通过，1 没到 ready，2 前置不满足

set -u
EXP_HOST="${F50_HOST:-192.168.77.1}"
EXP_PASS="${F50_PASS:-aassd11223}"
EXP_TIMEOUT="${F50_TIMEOUT:-600}"

CMD="/usr/libexec/airsim-audioopt-selftest${1:+ $1}"

TMP=$(mktemp /tmp/f50-selftest-XXXXXX.exp)
trap 'rm -f "$TMP"' EXIT

cat > "$TMP" <<EOF
#!/usr/bin/expect -f
set timeout $EXP_TIMEOUT
set cmd [lindex \$argv 0]
spawn ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null root@$EXP_HOST \$cmd
expect {
  "assword:" { send "$EXP_PASS\r"; exp_continue }
  eof
}
catch wait result
exit [lindex \$result 3]
EOF
chmod +x "$TMP"

echo "== 真机自测: ${CMD} (超时 ${EXP_TIMEOUT}s) =="
expect -f "$TMP" "$CMD"
