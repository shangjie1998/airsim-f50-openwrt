---
name: embedded-reserved-phys-range-claim-failure
description: 嵌入式设备上运行时 KO 开机抢固定物理内存段失败、导致依赖服务静默不启动时，怎么判断能否运行中恢复并按风险分级选解法
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [嵌入式, openwrt, 内核模块, 物理内存, dsp, 静默失败, 启动参数]
---
## 适用场景
设备上某个内核模块（KO）在开机时要把一段固定物理地址窗口从页分配器里"抢"出来给 DSP / 共享内存用，抢不到时依赖它的服务不启动（如声卡不注册、服务 `active with no instances`），且现象是偶发的、重启一次可能就好。

## 关键做法
1. 先读模块源码，看清它怎么占页：是 `alloc_pages` 扫块，还是有 `alloc_contig_range()` 迁移；迁移那段是否被 `#if LINUX_VERSION_CODE < ...` 条件编译掉。被编译掉就说明该内核上"已被占用的页永远要不回来"。
2. 查内核开关：`zcat /proc/config.gz | grep -E 'CONFIG_COMPACTION|CONFIG_CONTIG_ALLOC'`。没开则确认无迁移能力，运行中恢复基本无望。
3. 找模块自带的 sysfs 运行时入口（形如 `/sys/module/<mod>/parameters/retry`、`missing`），实测判断能不能救：先 `sync; echo 3 > /proc/sys/vm/drop_caches`，再写 retry，连续多轮看 `missing` 是否收敛。停在某个数就说明剩余是被真占用，分类行里的 slab/anon/file/other 计数就是证据。
4. 分级给方案：① 开机脚本里多轮 drop_caches + retry，仍失败限次重启（可逆、零风险）；② 把抢占挪到更早的 preinit 阶段（提高成功率，注意此时挂载/配置未必就绪）；③ 给启动 cmdline 加 `memmap=<SIZE>$<ADDR>`，让内核开机直接标 PageReserved，模块对全 reserved 窗口直接算已占，可 100% 成功——但要动启动镜像，必须先确认可回滚备份在位；④ 重编内核开 COMPACTION/CONTIG_ALLOC 代价最大，一般不做。

## 踩过的坑
- 只看到"清缓存后 retry 有效、数字掉了"就以为能救回来；实际掉的那部分只是当时的空闲页，一步就停。判断必须看连续多轮是否最终到 0。
- 不要先怀疑断电、服务顺序或驱动坏了，先读 `missing` 与 dmesg 那行分类计数，能直接区分是被占用还是压根没跑。
- 这类模块常是 `[permanent]` 不可卸载的，中途 `insmod` 重载会把缺失数翻得更大，反而更糟。
- 设备运行时改内存布局属高危，重启动作留给用户按。

## 可复用的产物或模板
```sh
cat /sys/module/<mod>/parameters/missing          # 必须 0
zcat /proc/config.gz | grep -E 'CONFIG_COMPACTION|CONFIG_CONTIG_ALLOC'
for i in 1 2 3 4 5; do sync; echo 3 >/proc/sys/vm/drop_caches; \
  echo 1 >/sys/module/<mod>/parameters/retry 2>/dev/null; \
  cat /sys/module/<mod>/parameters/missing; sleep 2; done
```