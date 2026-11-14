#!/bin/sh
# 本地入口：一条命令把真机的音频配件包自测跑完，不用开浏览器。
#
# 自测脚本是开发调试工具，不随主包发布（它会 rm -rf 模块目录，不能给终端用户）。
# 所以这里先把它传上去，再跑，设备上装没装都不影响。
#
# 用法：
#   sh build/tools/selftest-audio.sh           # 清模块 → 装回来 → 验 ready
#   sh build/tools/selftest-audio.sh --keep    # 不清模块，只验当前状态
#
# 为什么要单独造一个 expect：~/Desktop/f50ssh.exp 的 timeout 是 90 秒，
# 走 GitHub 下载慢的时候会话到点就被掐，安装器跟着被 SIGHUP 杀掉，
# 看上去像「卡在下载」其实是被中断。这里给 600 秒，够装两遍。
#
# 退出码跟远端一致：0 通过，1 没到 ready，2 前置不满足

set -u
HERE=$(cd "$(dirname "$0")" && pwd)
REMOTE_SCRIPT="$HERE/remote/airsim-audioopt-selftest"
REMOTE_PATH=/usr/libexec/airsim-audioopt-selftest
EXP_HOST="${F50_HOST:-192.168.77.1}"
EXP_PASS="${F50_PASS:-aassd11223}"
EXP_TIMEOUT="${F50_TIMEOUT:-600}"

[ -f "$REMOTE_SCRIPT" ] || {
	echo "找不到 $REMOTE_SCRIPT" >&2
	exit 2
}

# 造一个长超时的 expect：命令由参数传进去，避免转义地狱
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

run_remote() {
	expect -f "$TMP" "$1"
}

echo "== 1/2 把自测脚本传到设备 =="
# 用 ~/Desktop/f50scp.exp（单向上传，硬编码 192.168.77.1，跟 EXP_HOST 默认一致）
SCP_EXP="${F50_SCP_EXP:-$HOME/Desktop/f50scp.exp}"
if [ -f "$SCP_EXP" ]; then
	expect "$SCP_EXP" "$REMOTE_SCRIPT" "$REMOTE_PATH" || { echo "!! 传输失败" >&2; exit 2; }
else
	echo "!! 找不到 $SCP_EXP（可用 F50_SCP_EXP 指定）" >&2
	exit 2
fi
run_remote "chmod 755 $REMOTE_PATH && ls -l $REMOTE_PATH"

CMD="$REMOTE_PATH${1:+ $1}"
echo "== 2/2 跑自测: ${CMD} (超时 ${EXP_TIMEOUT}s) =="
run_remote "$CMD"
