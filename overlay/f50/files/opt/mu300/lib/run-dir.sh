#!/bin/sh
# run-dir.sh 兼容实现 —— /run 下私有运行目录的创建/接管。
#
# 厂商原版是 /opt/mu300/lib/run-dir.sh，但这台机器的固件比音频配件包旧，
# 那份库里没有这个文件。音频配件包的 mu300-voice 会 . 它并调用 run_dir_take，
# 缺了就直接启动失败，PCM 后端永远 not ready —— 表现就是"电话接通、全程静音"。
# 与 mu300-at / mu300-sms 属于同一类：我们的件依赖了厂商固件里的某份契约，
# 而那份契约在这台机器上不存在。
#
# 契约（依据 mu300-voice 注释与会话反推）：
#   run_dir_take <dir> <uid> <name>
#   - /run 是 1777，任何用户都能先占住同名目录；而里面放的 pid 文件、undo log、
#     mixer 回写都是我们以 root 身份写入的，所以只接受 uid 自己拥有的目录；
#   - 外来目录直接替换掉，并且说一声（不是干脆起不来）；
#   - 成功返回 0，失败返回非 0。
#
# 坑：这台设备的 busybox 里**没有 stat 命令**（ash: stat: not found）。
# 早先写的是 stat -c %u，永远拿到空值，于是每次都判"被 uid=? 占用"→ rm -rf，
# 把 scene.pid / output / scene.log 一起删了，scene_running 就永远返回 false。
# 改用 ls -ldn 取第三个字段（数字 uid），这个 busybox 是支持的。
_dir_owner() {
	ls -ldn "$1" 2>/dev/null | awk '{print $3; exit}'
}

run_dir_take() {
	dir=$1
	uid=${2:-0}
	name=${3:-$1}

	[ -n "$dir" ] || return 1

	if [ -e "$dir" ]; then
		owner=$(_dir_owner "$dir")
		if [ -n "$owner" ] && [ "$owner" != "$uid" ]; then
			echo "run_dir_take: ${name}: $dir 被 uid=$owner 占用，替换" >&2
			rm -rf "$dir" 2>/dev/null || true
		elif [ -z "$owner" ]; then
			# 连属主都读不出来，宁可不动它 —— 删了会连带把我们自己的状态文件清掉。
			echo "run_dir_take: ${name}: 读不出 $dir 的属主，原样沿用" >&2
		fi
	fi

	mkdir -p "$dir" 2>/dev/null || return 1
	chmod 700 "$dir" 2>/dev/null || true
	if [ "$uid" != "0" ]; then
		chown "$uid" "$dir" 2>/dev/null || true
	fi
	return 0
}
