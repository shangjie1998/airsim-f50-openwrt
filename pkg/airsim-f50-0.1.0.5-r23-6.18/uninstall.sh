#!/usr/bin/env sh
# 回滚到安装本包之前的版本
set -eu
VER=$(cat "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/VERSION")
BK=/usr/lib/airsim/backup-$VER
[ -d "$BK" ] || { echo "没有备份 $BK" >&2; exit 1; }
/etc/init.d/airsim stop >/dev/null 2>&1 || true
[ -f "$BK/airsim-mu300d" ] && cp -p "$BK/airsim-mu300d" /usr/lib/airsim/airsim-mu300d
[ -f "$BK/airsim-agent" ] && cp -p "$BK/airsim-agent" /usr/lib/airsim/airsim-agent
[ -f "$BK/VERSION" ] && cp -p "$BK/VERSION" /usr/lib/airsim/VERSION
rm -f /usr/lib/airsim/MANIFEST
for j in overview.js manage.js sms.js; do [ -f "$BK/$j" ] && cp -p "$BK/$j" /www/luci-static/resources/view/airsim/$j; done
[ -f "$BK/luci-app-airsim.json" ] && cp -p "$BK/luci-app-airsim.json" /usr/share/rpcd/acl.d/
/etc/init.d/airsim start; /etc/init.d/rpcd restart >/dev/null 2>&1 || true
echo "已回滚"
