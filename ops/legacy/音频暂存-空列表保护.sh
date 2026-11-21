#!/bin/sh
# 【已退役 · 历史留档】依赖 S97 airsim-audio-setup 状态机与 .airsim-complete-install 目录，
# 该机制随 complete-install 候选一并退役；现行音频走独立配件包 audio-opt，勿再照此脚本执行。
set -eu
W=/tmp/aud
SAVE=/mnt/mu300-disk/extra/.airsim-audio-recovery-backup
rm -rf "$SAVE"

sed -e 's|^tar -cpf "$SAVE/previous.tar" -C / -T "$SAVE/present"$|: > "$SAVE/previous.tar"; if [ -s "$SAVE/present" ]; then tar -cpf "$SAVE/previous.tar" -C / -T "$SAVE/present"; fi|' \
    -e 's|^    tar -xpf "$SAVE/previous.tar" -C /$|    if [ -s "$SAVE/previous.tar" ]; then tar -xpf "$SAVE/previous.tar" -C /; fi|' \
  "$W/integration/stage-device.sh" > /tmp/stage-device-fixed.sh
chmod 755 /tmp/stage-device-fixed.sh
echo "--- patched lines ---"
grep -n 'previous.tar' /tmp/stage-device-fixed.sh

/tmp/stage-device-fixed.sh --check "$W"
/tmp/stage-device-fixed.sh --stage "$W"

S=/mnt/mu300-disk/extra/.airsim-complete-install
mkdir -p "$S"; chmod 700 "$S"
printf '%s\n' 378ceea78dbf013e10625e71efd6fab487d727e371884ae8daf078da2828b937 > "$S/downloaded-firmware.sha256"
printf '%s\n' bundled_backup > "$S/firmware-source"
cat /proc/sys/kernel/random/boot_id > "$S/staged-boot"
printf '%s\n' reboot_required > "$S/status.new"; mv "$S/status.new" "$S/status"
/etc/init.d/airsim-audio-setup enable
echo "state=$(cat "$S/status")"
echo "--- EXT ---"
ls /mnt/mu300-disk/extra/airsim-audio | head -10
