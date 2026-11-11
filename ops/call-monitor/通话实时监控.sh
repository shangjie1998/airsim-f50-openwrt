#!/bin/sh
i=0
while [ "$i" -lt 100 ]; do
    ts=$(date +%H:%M:%S)
    st=$(wget -q -T 2 -O - http://127.0.0.1:7581/status 2>/dev/null)
    au=$(printf '%s' "$st" | grep -o '"audio":{[^}]*}')
    gw=$(printf '%s' "$st" | grep -o '"gateway":{[^}]*}')
    ca=$(printf '%s' "$st" | grep -o '"call_active":[a-z]*')
    lv=$(cat /proc/loadavg | cut -d' ' -f1)
    vo=$(/opt/mu300/bin/mu300-voice 2>&1 | tr '\n' '|')
    printf '%s %s %s %s load=%s\n  %s\n  %s\n' "$ts" "$ca" "$au" "$gw" "$lv" "$vo" "$au"
    i=$((i + 1))
    sleep 2
done
