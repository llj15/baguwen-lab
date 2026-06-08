#!/usr/bin/env python3
"""
Redis 缓存三大问题实验 v2 - 分析报告生成
增强版: 布隆过滤器 / 安全分布式锁 / 多级缓存
"""

import json
import os
import re
import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import numpy as np

plt.rcParams['axes.unicode_minus'] = False

DATA_DIR = '/data'

LABEL_MAP = {
    '无保护': 'No Protection',
    '缓存空值_首轮': 'Empty Cache\n(1st round)',
    '缓存空值_二轮': 'Empty Cache\n(2nd round)',
    '布隆过滤器': 'Bloom Filter',
    'singleflight': 'singleflight',
    '朴素分布式锁': 'Naive Lock',
    '安全分布式锁': 'Safe Lock\n(owner+Lua+WD)',
    '分布式锁': 'Dist Lock',
    '相同TTL': 'Same TTL',
    '随机TTL': 'Random TTL',
    '多级缓存': 'Multi-Level\n(L1+L2)',
    '缓存穿透': 'Penetration',
    '缓存击穿': 'Breakdown',
    '缓存雪崩': 'Avalanche',
}

def en(name):
    return LABEL_MAP.get(name, name)

def load_results():
    with open(os.path.join(DATA_DIR, 'results.json'), 'r') as f:
        return json.load(f)

def auto_colors(n):
    palettes = {
        2: ['#e74c3c', '#2ecc71'],
        3: ['#e74c3c', '#f39c12', '#2ecc71'],
        4: ['#e74c3c', '#f39c12', '#3498db', '#2ecc71'],
    }
    return palettes.get(n, ['#95a5a6'] * n)

def plot_experiment(exp, exp_num, extra_title=''):
    scenarios = exp['scenarios']
    n = len(scenarios)
    names = [en(s['name']) for s in scenarios]
    db_hits = [s['db_hits'] for s in scenarios]
    durations = [s['duration_ms'] for s in scenarios]
    colors = auto_colors(n)

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(max(12, n*2.5), 5))
    title = f"Exp {exp_num}: {en(exp['name'])}{extra_title}"
    fig.suptitle(title, fontsize=16, fontweight='bold', y=1.02)

    bars1 = ax1.bar(names, db_hits, color=colors, edgecolor='black', linewidth=0.5)
    ax1.set_title('DB Hits', fontsize=13)
    ax1.set_ylabel('DB Access Count')
    max_h = max(db_hits) if max(db_hits) > 0 else 1
    for bar, val in zip(bars1, db_hits):
        ax1.text(bar.get_x() + bar.get_width()/2., bar.get_height() + max_h*0.02,
                 str(val), ha='center', va='bottom', fontweight='bold', fontsize=11)

    bars2 = ax2.bar(names, durations, color=colors, edgecolor='black', linewidth=0.5)
    ax2.set_title('Duration', fontsize=13)
    ax2.set_ylabel('Duration (ms)')
    max_d = max(durations) if max(durations) > 0 else 1
    for bar, val in zip(bars2, durations):
        ax2.text(bar.get_x() + bar.get_width()/2., bar.get_height() + max_d*0.02,
                 f'{val:.0f}ms', ha='center', va='bottom', fontweight='bold', fontsize=10)

    plt.tight_layout()
    fname = ['penetration', 'breakdown', 'avalanche'][exp_num - 1]
    path = os.path.join(DATA_DIR, f'{fname}.png')
    plt.savefig(path, dpi=150, bbox_inches='tight')
    plt.close()

def plot_summary(results):
    fig, ax = plt.subplots(figsize=(18, 6))

    experiments = results['experiments']
    all_names = []
    all_hits = []
    all_colors = []

    for exp in experiments:
        n = len(exp['scenarios'])
        colors = auto_colors(n)
        for i, s in enumerate(exp['scenarios']):
            label = f"{en(exp['name'])}\n{en(s['name'])}"
            all_names.append(label)
            all_hits.append(s['db_hits'])
            all_colors.append(colors[i])

    x = np.arange(len(all_names))
    bars = ax.bar(x, all_hits, color=all_colors, edgecolor='black', linewidth=0.5, width=0.6)

    max_h = max(all_hits) if max(all_hits) > 0 else 1
    for bar, val in zip(bars, all_hits):
        ax.text(bar.get_x() + bar.get_width()/2., bar.get_height() + max_h*0.01,
                str(val), ha='center', va='bottom', fontweight='bold', fontsize=9)

    ax.set_xticks(x)
    ax.set_xticklabels(all_names, fontsize=7)
    ax.set_ylabel('DB Access Count', fontsize=12)
    ax.set_title('Redis Cache Problems v2 - Defense Effectiveness Summary',
                 fontsize=15, fontweight='bold')

    exp_sizes = [len(e['scenarios']) for e in experiments]
    pos = 0
    for size in exp_sizes[:-1]:
        pos += size
        ax.axvline(x=pos - 0.5, color='gray', linestyle='--', alpha=0.5)

    plt.tight_layout()
    path = os.path.join(DATA_DIR, 'summary.png')
    plt.savefig(path, dpi=150, bbox_inches='tight')
    plt.close()


def parse_extra(extra_str):
    """Parse extra field like 'bloom_rejected=998' into dict"""
    if not extra_str:
        return {}
    return dict(item.split('=') for item in extra_str.split(',') if '=' in item)


def generate_report(results):
    exps = {e['name']: e for e in results['experiments']}

    report = f"""# Redis 缓存三大问题 - 实验报告 v2 (增强版)

> 实验时间: {results['timestamp']}
> 环境: Docker (Redis 7 Alpine + Go 1.25)
> 增强: 布隆过滤器 / 安全分布式锁(owner+Lua+Watchdog) / 多级缓存(L1+L2)

---

## 概述

| 问题 | 含义 | 基础方案 | 增强方案 |
|------|------|----------|----------|
| 缓存穿透 | 查询不存在的数据，请求直达DB | 缓存空值 | **布隆过滤器前置拦截** |
| 缓存击穿 | 热点key过期瞬间，并发请求打DB | singleflight / 朴素锁 | **安全锁(owner+Lua+看门狗)** |
| 缓存雪崩 | 大批key同时过期 | 随机TTL | **多级缓存(L1本地+L2 Redis)** |

---

## 实验1: 缓存穿透 (Cache Penetration)

**实验设计**: 1000个不存在的用户ID请求

![穿透实验](penetration.png)

| 方案 | DB访问 | 耗时(ms) | 说明 |
|------|--------|----------|------|
"""
    pen = exps.get('缓存穿透', {})
    for s in pen.get('scenarios', []):
        extra = parse_extra(s.get('extra', ''))
        note = ""
        if s['db_hits'] == 0 and 'bloom' not in s['name'].lower():
            note = "全部走缓存"
        elif s['db_hits'] == 0:
            note = "布隆过滤器全部拦截"
        elif 'bloom_rejected' in extra:
            note = f"布隆拦截{extra['bloom_rejected']}个"
        elif s['db_hits'] > 100:
            note = "全部穿透"
        report += f"| {s['name']} | {s['db_hits']} | {s['duration_ms']:.0f} | {note} |\n"

    report += """
**布隆过滤器原理**: 多个哈希函数映射到位数组。如果任一位为0，数据**一定不存在**；全部为1，数据**可能存在**（有误判率）。

**生产实践**: 合法数据写入DB时同步 `BF.ADD`，查询前先 `BF.EXISTS`。误判率可控在 0.1%。

**关键区别**: 缓存空值需要每个key穿透一次DB才能建立缓存；布隆过滤器在**应用层内存**中直接拦截，连Redis都不查。

---

## 实验2: 缓存击穿 (Cache Breakdown)

**实验设计**: 200个goroutine并发查询同一个已过期的热点key

![击穿实验](breakdown.png)

| 方案 | DB访问 | 耗时(ms) | 安全性 |
|------|--------|----------|--------|
"""
    brk = exps.get('缓存击穿', {})
    safety = {
        '无保护': 'N/A',
        'singleflight': '进程内安全',
        '朴素分布式锁': '有隐患: 可能误删别人的锁',
        '安全分布式锁': '安全: owner标识+Lua原子释放+看门狗续期',
    }
    for s in brk.get('scenarios', []):
        safe = safety.get(s['name'], '')
        report += f"| {s['name']} | {s['db_hits']} | {s['duration_ms']:.0f} | {safe} |\n"

    report += """
**朴素锁 vs 安全锁**:

```
朴素锁 (有bug):
  SET lockKey "1" NX EX 5
  defer DEL lockKey          // 危险! 可能删别人的锁

安全锁 (生产级):
  ownerID = random()
  SET lockKey ownerID NX EX 5
  启动看门狗(每TTL/3续期)
  Lua: if GET(lockKey)==ownerID then DEL   // 原子操作, 只删自己的
```

**朴素锁的三个隐患**:
1. 无owner标识 → A的锁过期后被B抢到，A执行完DEL删的是B的锁
2. 无续期机制 → 业务耗时 > 锁TTL 时，锁提前过期，多个请求同时进入临界区
3. DEL非原子 → GET+判断+DEL之间可能有时间窗口

---

## 实验3: 缓存雪崩 (Cache Avalanche)

**实验设计**: 200个商品key，对比不同防御策略

![雪崩实验](avalanche.png)

| 方案 | DB访问 | 耗时(ms) | 说明 |
|------|--------|----------|------|
"""
    ava = exps.get('缓存雪崩', {})
    for s in ava.get('scenarios', []):
        extra = parse_extra(s.get('extra', ''))
        if s['name'] == '相同TTL':
            note = "全部同时过期, DB压力最大"
        elif s['name'] == '随机TTL':
            note = "部分key存活, DB压力减半"
        elif 'l1_hits' in extra:
            note = f"L1命中{extra['l1_hits']}个, 进一步降低DB压力"
        else:
            note = ""
        report += f"| {s['name']} | {s['db_hits']} | {s['duration_ms']:.0f} | {note} |\n"

    report += """
**多级缓存架构**:
```
请求 → L1(进程内sync.Map, TTL=2s) → L2(Redis, TTL=3s+jitter) → DB
```
- L1 TTL **必须 < L2 TTL**，避免本地缓存比Redis更旧
- L1 即使Redis全挂也能兜底，防止雪崩直接打到DB
- 生产环境用 ristretto/freecache 替代 sync.Map（有容量限制、LRU淘汰）

---

## 综合对比

![综合对比](summary.png)

## 技术选型建议

| 场景 | 推荐方案 | 复杂度 | 面试关注点 |
|------|----------|--------|------------|
| 防穿透(基础) | 缓存空值 | 低 | 空值TTL设多久？占用Redis内存 |
| 防穿透(严格) | 布隆过滤器 + 缓存空值 | 中 | 误判率、数据更新时布隆过滤器怎么同步 |
| 防击穿(单机) | singleflight | 低 | 多实例部署时失效 |
| 防击穿(多实例) | 安全分布式锁 | 中 | owner+Lua+看门狗三件套 |
| 防雪崩(基础) | TTL随机化 | 低 | 随机范围怎么定 |
| 防雪崩(严格) | 随机TTL + 多级缓存 | 中 | L1/L2 TTL关系、容量控制 |

---

*Generated by redis-cache-demo v2 experiment*
"""
    return report


def main():
    print("=" * 50)
    print("  Redis Cache Experiment v2 - Analysis Report")
    print("=" * 50)

    results = load_results()

    print("\nGenerating charts...")
    for i, exp in enumerate(results['experiments']):
        plot_experiment(exp, i + 1)
        print(f"  - {['penetration', 'breakdown', 'avalanche'][i]}.png")

    plot_summary(results)
    print("  - summary.png")

    print("\nGenerating report...")
    report = generate_report(results)
    report_path = os.path.join(DATA_DIR, 'report.md')
    with open(report_path, 'w') as f:
        f.write(report)
    print("  - report.md")

    print(f"\nAll files saved to {DATA_DIR}/")
    print("Done!")


if __name__ == '__main__':
    main()
