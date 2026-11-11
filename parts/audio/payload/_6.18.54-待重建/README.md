# 6.18.54 音频模块待重建（占位）

这个目录只有说明，没有 `.ko`。`make-opt.sh` 会跳过 `_` 开头的目录，所以它存在也不会影响现有的 6.18.60 包。

## 为什么要重建

设备现在跑 `6.18.54`，而手上这套语音驱动是给 `6.18.60` 编的。内核 ABI 层直接拒：

```
insmod: struct module size must match the kernel's built struct module size at run time
```

这个错误脚本兜不住，只有两个出路：**按 6.18.54 重编这一套**，或者**把系统换回 6.18.60 的镜像**。

## 怎么重建

```sh
# 1. 准备一份 6.18.54 的内核构建树（同一份源码 + 同样的 .config，至少跑过 make modules_prepare）
make -s -C /path/linux-6.18.54 kernelrelease   # 必须输出 6.18.54

# 2. 编译 + 收模块（明细步骤见 docs/音频/配件包.md）
sh parts/audio/build-krel.sh --krel 6.18.54 --tree /path/linux-6.18.54 --tag nightly-20261109

# 3. 校验（缺一个或 vermagic 不对都会失败）
sh parts/audio/build-krel.sh --check 6.18.54+nightly-20261109

# 4. 打音频可选件
sh parts/audio/make-opt.sh 6.18.54+nightly-20261109
```

产物会落在 `parts/audio/payload/6.18.54+nightly-20261109/`（25 个 `.ko` + `KERNEL`）。
