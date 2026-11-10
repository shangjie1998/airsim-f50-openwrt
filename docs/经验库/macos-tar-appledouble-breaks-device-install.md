---
name: macos-tar-appledouble-breaks-device-install
description: macOS 上打的 tar.gz 发到设备安装时被判「包结构异常/找不到 install.sh」，查是 bsdtar 塞进 ._ 影子文件而不是安装逻辑坏了
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [macos, tar, bsdtar, 打包, openwrt, 嵌入式, 安装包, 静默失败]
---
## 适用场景
在 macOS 上为 OpenWrt 或嵌入式设备制作 tar.gz 安装包，设备侧安装器会检查包内目录结构（顶层只能有一个目录、或要在某层找到 install.sh）时；或设备报「安装包结构异常/没有 install.sh」，本机 `tar -tzf` 看着却完全正常。

## 关键做法
1. 先在设备上（不是本机）原样解一遍：`curl -sL -H 'Accept: application/octet-stream' <asset-url> -o /tmp/t.tgz; tar -tzf /tmp/t.tgz`。
2. 看输出里有没有 `._<名>`、`._<目录名>` 这类条目，或根下多出 `._xxx` 的影子文件——那就是 macOS bsdtar 把扩展属性打成 AppleDouble 条目塞进包里了，条目数会比本机看到的少。
3. 打包时关掉元数据：`COPYFILE_DISABLE=1 tar --no-xattrs --no-mac-metadata -czf out.tar.gz -C stage dir`。
4. 打出后核验条目：`tar -tzf out.tar.gz | grep -c '^\._'` 必须为 0，再对比条目总数与本机文件数一致。
5. 重传资产时必须先删旧资产再上传同名文件，否则设备比对 sha256 会因同名同标签而混淆。

## 踩过的坑
设备上看到 `._airsim-f50-…` 被当成顶层目录之一，安装器判「顶层不唯一」直接拒绝，报的还是「安装包结构异常」，看起来像安装逻辑退化成旧版本，实际是打包污染。只在本机 `tar -tzf` 检查永远发现不了，因为本机解析时会把这些影子条目处理掉。本机 sha256 与上传后的资产一致，也不能证明设备解出来干净。

## 可复用的产物或模板
```sh
COPYFILE_DISABLE=1 tar --no-xattrs --no-mac-metadata -czf "$OUT" -C "$STAGE" "$NAME"
tar -tzf "$OUT" | grep -c '^\._'   # 期望 0
```