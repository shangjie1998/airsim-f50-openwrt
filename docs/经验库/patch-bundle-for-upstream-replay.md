---
name: patch-bundle-for-upstream-replay
description: 要把一次性的源码修复变成能随上游更新反复重放的补丁包时，如何选对基线、固化构建的隐式外部依赖与签名，并在全新 clone 上端到端验证补丁可用
metadata:
    type: work_experience
    source: harness
    created_at: "2026-11-07"
    tags:
        - 补丁包
        - 源码补丁
        - 构建脚本
        - 依赖缓存
        - 签名密钥
        - 上游更新
        - git apply
        - release tag
---
## 适用场景
已经在一个仓库上改好一处修复，用户要求把它单独留存，等上游更新后再套到新版本上重新编译（而不是只在当前工作副本里生效）。

## 关键做法
1. 用 `git diff > fix.patch` 导出补丁并记录基线 commit。**补丁里不要包含版本号（versionCode、build number、manifest 版本字段）这类每次发布都会变的 hunk**，版本号一律交给脚本按设备已装版本自动抬——否则每换一个发布版都必然在这一处冲突。
2. 先枚举远程 tag 与分支再定基线：`git ls-remote --tags origin`、`git ls-remote --heads origin`。仓库的默认分支不一定是最新发布版，存在 tag 比默认分支更新（release 从 tag 点往前走）的情况；必要时 `git merge-base --is-ancestor` 判断两者关系，用 `git log --oneline <a>..<b>` 看各自独有提交。
3. 脚本接受「工作目录 + 可选 git ref」两个参数，ref 为空才取默认分支最新，否则 `git checkout -B <分支> <ref>`，这样同一套脚本既能跟发布版也能跟开发分支。
4. 打补丁用 `git apply --check -3` 试探，干净则 `git apply -3`，否则退 `git apply --reject` 并打印冲突文件与"重点看哪几个函数还在"。
5. 打完补丁 grep 校验核心代码片段是否真在位——apply 成功不等于修复还在，上游可能已重构。
6. 把构建脚本会下载的依赖全部打进补丁目录，脚本构建前铺到脚本期望的路径。仓库构建脚本常从公网 maven/google 直拉，换网络环境就整条构建失败。
7. 把签名 keystore 一起打进包里并在构建前植入脚本期望的位置：多数仓库的 debug 构建会**每次重新生成** keystore，导致每个包签名都不同，升级覆盖安装直接被拒。
8. 版本号按设备已装版本自动抬（`dumpsys package <包名> | grep versionCode`），避免降级安装失败。
9. 验证：在全新 clone 上跑一遍完整脚本，确认补丁干净套用、编译打包成功、产物 dex 里关键特征串各命中，并与已装 APK 用 `apksigner verify --print-certs` 比对证书指纹一致。

## 踩过的坑
- bash 中 `echo "... $VAR（中文全角括号..."`：全角字符会被当成变量名一部分，`set -u` 下报 `unbound variable`。凡变量引用紧跟非 ASCII 字符一律写 `${VAR}`；改完用正则批量扫一遍 `$VAR` 后接非 ASCII 的位置。
- 默认分支不等于最新发布版：只跟默认分支会默默构建出比 release 旧的功能。看到用户贴 release 链接时，先核对 tag 与默认分支的先后关系。
- 补丁含 versionCode hunk 会在每个新 tag 上冲突，且这处改动本身没有价值（脚本会重算）。
- 仓库 build.sh 从 `repo1.maven.org` / `dl.google.com` 拉依赖，在有网环境里也可能 404（路径级被墙）。不要在脚本里加重试，直接把文件缓存进包。
- build.sh 每次都新建 debug.keystore → 每个产物签名都不同 → `adb install -r` 报签名不符，逼用户卸载重装（会清掉应用内配对/登录状态）。必须固化 keystore。
- 签名字段容易看错：`dumpsys package` 里的 `signatures=[xxxxxxxx]` 是哈希片段，判断同签名要用 `apksigner verify --print-certs` 比对完整 SHA-256。
- `git apply -3` 的冲突分支在没有真正前进过的上游上测不出来；上游确实前进后才需手工并 hunk，并完重新导出补丁覆盖旧的。

## 可复用的产物或模板
```bash
# 枚举可用基线
git ls-remote --tags origin | tail -20
git ls-remote --heads origin
# 判断 tag 与默认分支关系
git merge-base --is-ancestor <tag-commit> <branch-head> && echo "tag 是祖先" || echo "tag 在分支之外"
git log --oneline <branch-head>..<tag-commit>   # tag 独有提交
# 在全新 clone 上重放（支持指定 ref）
git clone <repo> work && cd work
git checkout -B apply-$(date +%Y%m%d) "${REF:-origin/main}"
git apply --check -3 ../fix.patch && git apply -3 ../fix.patch || git apply --reject ../fix.patch
# 抬版本号
CUR=$(grep -o 'android:versionCode="[0-9]*"' AndroidManifest.xml | grep -o '[0-9]*' | head -1)
INSTALLED=$(adb shell "dumpsys package <pkg> | grep -m1 versionCode" | grep -o '[0-9]*' | head -1)
sed -i '' "s/android:versionCode=\"${CUR}\"/android:versionCode=\"$(( ${INSTALLED:-0} + 1 ))\"/" AndroidManifest.xml
# 校验签名是否与已装一致
apksigner verify --print-certs new.apk | grep 'certificate SHA-256'
```