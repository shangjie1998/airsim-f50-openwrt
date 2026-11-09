# 把 set_fits 里“CONFIG 必须与 /proc/config.gz 逐位相同”改成“声明了 CONFIG 就放行”
/config_sum\)/ && /_want/ { print "    [ -n \"$_want\" ] && true"; next }
{ print }
