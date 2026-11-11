#!/bin/sh
set -eu
SRC=/opt/mu300/bin/mu300-at
BAK=/opt/mu300/bin/mu300-at.bak-20261107
[ -f "$BAK" ] || cp -p "$SRC" "$BAK"

if grep -q 'if [ "$cmd" = - ]' "$SRC"; then
    echo 'already patched'
else
    awk '
        { print }
        /^cmd=\$\{1:-\}$/ && !done {
            print "if [ \"$cmd\" = - ]; then"
            print "    cmd="
            print "    IFS= read -r cmd || [ -n \"$cmd\" ] || cmd="
            print "fi"
            done = 1
        }
    ' "$SRC" > /tmp/mu300-at.new
    chmod 755 /tmp/mu300-at.new
    sh -n /tmp/mu300-at.new || { echo 'syntax error; not installed'; exit 1; }
    cp /tmp/mu300-at.new "$SRC"
    cp /tmp/mu300-at.new /usr/bin/mu300-at 2>/dev/null || true
    echo 'patched'
fi

echo '--- verify ---'
sed -n '48,58p' "$SRC"
echo '--- stdin mode test ---'
printf 'AT+CLCC\n' | /usr/bin/mu300-at -t 6 -
echo "rc=$?"
echo '--- argv mode test ---'
/usr/bin/mu300-at 'AT+CLCC'
echo "rc=$?"
