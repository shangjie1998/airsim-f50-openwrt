---
name: no-build-env-patch-verification
description: 源码仓库要求高版本 JDK 或 Android SDK 但本机不具备、无法整仓编译时，如何给出可交付且能自证质量的多文件补丁
metadata:
  type: work_experience
  source: harness
  created_at: 2026-11-07
  tags: [java, android, 补丁交付, 无构建环境, 桩测试, dry-run]
---
## 适用场景
改动了 Java/Android 等多文件源码，但本机 JDK 版本低于仓库要求（如仓库用 record、switch 表达式需 17+，本机仅 11），或缺少 android.jar、build-tools、d8，无法整仓编译，仍要交付补丁并说明可信度。
## 关键做法
先如实判定编译不可做，再去争"部分可证"：用 `git archive HEAD` 导出干净副本，`patch -p1 --dry-run` 干跑补丁，确认全部文件可干净应用；把新增的核心逻辑（降级阶梯、判据表达式、字段解析）抽出来用桩类写成独立可跑的测试，输入尽量用线上真实数据串，逐场景断言；对改动文件做括号配平粗查排除手误；交付时明确声明"整仓编译未做"，把编译这一步交回具备环境的用户。
## 踩过的坑
用低版本 javac 直接编译整个源码树，会报出大量 `需要')'`、`非法的表达式开始` 之类的语法错误，这些其实是 JDK 不支持新语法（record、switch 表达式）造成的，极易被误判成自己改错了括号；判断时要先按报错文件名过滤，凡是只涉及仓库原有的新语法文件的报错都不算回归，只需关注自己改动的文件里是否有真正的语法错误。干跑补丁必须在干净 HEAD 副本上做，在工作区里 dry-run 会被现有改动干扰而给出假结论。
## 可复用的产物或模板
```bash
mkdir -p /tmp/patchcheck && git -C <repo> archive HEAD | tar -x -C /tmp/patchcheck
cd /tmp/patchcheck && patch -p1 --dry-run < <patch-file>
# 桩测试：只编译单个测试文件，内部用静态桩类复刻目标逻辑
javac -d . XxxTest.java && java XxxTest
```