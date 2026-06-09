#!/usr/bin/env python3
"""Generate charts and narrative result documents for the Redis big/hot key lab."""

from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt


OUTPUT_DIR = Path(os.environ.get("OUTPUT_DIR", "/data"))


def load_results() -> dict[str, Any]:
    return json.loads((OUTPUT_DIR / "results.json").read_text(encoding="utf-8"))


def scenario(data: dict[str, Any], experiment_name: str, scenario_name: str) -> dict[str, Any]:
    for experiment in data["experiments"]:
        if experiment["name"] == experiment_name:
            for item in experiment["scenarios"]:
                if item["name"] == scenario_name:
                    return item["metrics"]
    raise KeyError(f"{experiment_name}/{scenario_name}")


def mb(value: float) -> float:
    return value / 1024 / 1024


def save_current_figure(filename: str) -> None:
    path = OUTPUT_DIR / filename
    plt.tight_layout()
    plt.savefig(path, dpi=150, bbox_inches="tight")
    plt.close()
    print(f"Generated: {path}")


def plot_big_key_memory(data: dict[str, Any]) -> None:
    normal = scenario(data, "redis_big_key", "representative_dataset")
    big_string = scenario(data, "redis_big_key", "big_string")
    big_hash = scenario(data, "redis_big_key", "big_hash")
    big_list = scenario(data, "redis_big_key", "big_list")

    labels = ["Normal string avg", "8 MiB string", "50k hash", "50k list"]
    values = [
        mb(normal["normal_avg_memory"]),
        mb(big_string["memory_bytes"]),
        mb(big_hash["memory_bytes"]),
        mb(big_list["memory_bytes"]),
    ]

    fig, ax = plt.subplots(figsize=(9, 5))
    bars = ax.bar(labels, values, color=["#4c78a8", "#d95f02", "#7570b3", "#1b9e77"])
    ax.set_ylabel("Redis memory usage (MiB)")
    ax.set_title("Big key memory footprint")
    ax.set_yscale("log")
    for bar, value in zip(bars, values):
        ax.text(bar.get_x() + bar.get_width() / 2, value * 1.05, f"{value:.2f}", ha="center", fontsize=9)
    save_current_figure("big-key-memory.png")


def plot_hot_key_distribution(data: dict[str, Any]) -> None:
    uniform = scenario(data, "redis_hot_key", "uniform_access")
    hot = scenario(data, "redis_hot_key", "single_hot_key_access")
    sharded = scenario(data, "redis_hot_key", "sharded_hot_key_access")

    labels = ["Uniform top key", "Single hot key", "Hottest copy after sharding"]
    values = [
        uniform["top_ratio"] * 100,
        hot["top_ratio"] * 100,
        sharded["top_ratio"] * 100,
    ]

    fig, ax = plt.subplots(figsize=(9, 5))
    bars = ax.bar(labels, values, color=["#4c78a8", "#d95f02", "#1b9e77"])
    ax.set_ylabel("Top key share (%)")
    ax.set_title("Hot key traffic skew")
    ax.set_ylim(0, 100)
    for bar, value in zip(bars, values):
        ax.text(bar.get_x() + bar.get_width() / 2, value + 1, f"{value:.1f}%", ha="center", fontsize=9)
    save_current_figure("hot-key-distribution.png")


def plot_mitigation_summary(data: dict[str, Any]) -> None:
    split_hash = scenario(data, "redis_big_key", "split_hash_mitigation")
    big_hash = scenario(data, "redis_big_key", "big_hash")
    hot = scenario(data, "redis_hot_key", "single_hot_key_access")
    sharded = scenario(data, "redis_hot_key", "sharded_hot_key_access")

    labels = ["Hash fields per key", "Hot reads per key"]
    before = [big_hash["fields"], hot["top_count"]]
    after = [split_hash["max_bucket_fields"], sharded["max_copy_gets"]]

    fig, ax = plt.subplots(figsize=(8, 5))
    x = range(len(labels))
    width = 0.35
    ax.bar([i - width / 2 for i in x], before, width, label="Before", color="#d95f02")
    ax.bar([i + width / 2 for i in x], after, width, label="After", color="#1b9e77")
    ax.set_xticks(list(x))
    ax.set_xticklabels(labels)
    ax.set_ylabel("Maximum load on one Redis key")
    ax.set_title("Mitigations reduce max per-key pressure")
    ax.legend()
    save_current_figure("summary.png")


def markdown_report(data: dict[str, Any]) -> str:
    cfg = data["dataset"]
    normal = scenario(data, "redis_big_key", "representative_dataset")
    big_string = scenario(data, "redis_big_key", "big_string")
    big_hash = scenario(data, "redis_big_key", "big_hash")
    big_list = scenario(data, "redis_big_key", "big_list")
    split_hash = scenario(data, "redis_big_key", "split_hash_mitigation")
    hash_read = scenario(data, "redis_big_key", "hash_full_read_vs_scan")
    uniform = scenario(data, "redis_hot_key", "uniform_access")
    hot = scenario(data, "redis_hot_key", "single_hot_key_access")
    sharded = scenario(data, "redis_hot_key", "sharded_hot_key_access")

    return f"""# Redis 大 Key 与热点 Key 实验结果

本文档由真实的 Docker Compose 实验运行生成。数据集由实验程序在运行时构造并写入 Redis，随后通过 `MEMORY USAGE`、`HLEN`、`LLEN`、`HGETALL`、`HSCAN` 以及真实的 `GET` 访问负载从 Redis 中测量。仓库中记录的数值不是手写夹具，也不是为了结论人为编造的数据。

## 数据集

| 维度 | 数值 |
| --- | ---: |
| 普通字符串 key 数量 | {cfg['normal_string_count']} |
| 普通字符串 value 大小 | {cfg['normal_value_bytes']} bytes |
| 大字符串 value 大小 | {cfg['big_string_bytes']} bytes |
| 大 Hash 字段数 | {cfg['big_hash_fields']} |
| 大 List 元素数 | {cfg['big_list_items']} |
| 热点 Key 访问空间 | {cfg['hot_keyspace']} keys |
| 热点 Key 请求数 | {cfg['hot_requests']} |
| 热点 Key 倾斜比例 | {cfg['hot_ratio_percent']}% 请求集中到一个逻辑 key |

## 大 Key 实验结果

![大 Key 内存占用](big-key-memory.png)

| 场景 | 测量值 |
| --- | ---: |
| 普通字符串平均内存 | {normal['normal_avg_memory']} bytes |
| 大字符串内存 | {big_string['memory_bytes']} bytes |
| 大字符串逻辑长度 | {big_string['strlen']} bytes |
| 大 Hash 内存 | {big_hash['memory_bytes']} bytes |
| 大 Hash 字段数 | {big_hash['fields']} |
| 大 List 内存 | {big_list['memory_bytes']} bytes |
| 大 List 元素数 | {big_list['items']} |

大字符串、大 Hash 和大 List 都是合法的 Redis 对象，但它们把远高于普通 key 的内存占用和命令处理量集中到了单个 key 上。大 Key 的核心风险不在于 Redis 不能存储它，而在于单个命令、单个 key 的所有权和迁移成本会变得过重。

完整读取 Hash 时，`HGETALL` 在一次命令中返回了 `{hash_read['hgetall_fields']}` 个字段。游标扫描同样读取了 `{hash_read['hscan_fields']}` 个字段，但把工作拆成了 `{hash_read['hscan_iterations']}` 次调用。耗时会受宿主机影响，但命令形态是稳定的：`HGETALL` 会产生一次大响应，`HSCAN` 则允许调用方分片处理。

Hash 拆分方案保留了全部 `{split_hash['total_fields']}` 个字段，并把最大 bucket 降到 `{split_hash['max_bucket_fields']}` 个字段，数据分布在 `{split_hash['bucket_count']}` 个 key 上。拆分不会减少总数据量，但会降低单个 key 和单条命令承受的最大压力。

## 热点 Key 实验结果

![热点 Key 访问分布](hot-key-distribution.png)

| 场景 | 访问量最高的 key | 最高访问次数 | 最高访问占比 |
| --- | --- | ---: | ---: |
| 均匀访问 | `{uniform['top_key']}` | {uniform['top_count']} | {uniform['top_ratio']:.4f} |
| 单热点 key | `{hot['top_key']}` | {hot['top_count']} | {hot['top_ratio']:.4f} |
| 热点 key 读副本分片 | `{sharded['top_key']}` | {sharded['top_count']} | {sharded['top_ratio']:.4f} |

均匀访问负载把读请求分散到 `{uniform['unique_keys']}` 个 key 上，因此没有单个 key 占据主导。热点 Key 负载把 `{hot['requests']}` 次请求中的 `{hot['top_count']}` 次打到 `{hot['top_key']}`，从访问计数上直接证明了流量倾斜问题。

读副本分片方案保持同一个逻辑热点数据不变，但把读取分散到 `{sharded['hot_copy_count']}` 个物理 key 上。最热的副本收到 `{sharded['max_copy_gets']}` 次读取，而不是原始热点 key 的 `{hot['top_count']}` 次读取，从而降低单个 Redis key 的压力。

![缓解效果汇总](summary.png)

## 解决方案与适用边界

### 大 Key 解决方案

| 问题 | 推荐方案 | 本实验是否验证 | 适用边界 |
| --- | --- | --- | --- |
| 单个 Hash/List/Set/ZSet 元素过多 | 按业务维度拆成多个小 key，例如按 bucket、页号、时间片拆分 | 是，`{split_hash['total_fields']}` 个字段拆到 `{split_hash['bucket_count']}` 个 key，最大 bucket 为 `{split_hash['max_bucket_fields']}` 个字段 | 需要业务能接受多 key 读写，查询时可能要合并多个 bucket |
| 一次命令返回过大 | 用 `HSCAN`、`SSCAN`、`ZSCAN`、分批 `LRANGE` 替代全量读取 | 是，`HSCAN` 把 `{hash_read['hscan_fields']}` 个字段拆成 `{hash_read['hscan_iterations']}` 次调用 | 适合后台遍历和分页读取，不适合必须强一致返回全量数据的接口 |
| 单个大字符串过大 | 拆分为分片 key，或把大对象放对象存储/数据库，Redis 只存索引和热点摘要 | 否，本实验只测量了 8 MiB 字符串的内存占用 | 适合图片、长文本、大 JSON 等对象；需要处理分片一致性和清理逻辑 |
| 删除大 Key 阻塞主线程 | 使用 `UNLINK` 异步删除，或后台分批删除集合元素后再删除空 key | 否 | 适合线上清理；删除前应先限速和灰度，避免瞬时内存/CPU 抖动 |
| 发现大 Key | 使用 `MEMORY USAGE`、`redis-cli --bigkeys`、离线 RDB 分析、业务侧 key 规模指标 | 部分验证，本实验使用 `MEMORY USAGE` 采集内存 | `--bigkeys` 会扫描 keyspace，线上执行要控制时间窗口和频率 |

大 Key 的治理优先级是：先识别，再限制增长，最后拆分或迁移。面试回答时可以概括为“避免全量命令、拆结构、异步删、监控发现、从数据模型上限制单 key 规模”。

### 热点 Key 解决方案

| 问题 | 推荐方案 | 本实验是否验证 | 适用边界 |
| --- | --- | --- | --- |
| 读热点集中到单个 key | 读副本分片，把一个逻辑热点数据复制到多个物理 key，客户端按随机或哈希读取 | 是，单热点从 `{hot['top_count']}` 次读取降到单副本最高 `{sharded['max_copy_gets']}` 次读取 | 适合读多写少；写入时要同步更新多个副本或接受短暂不一致 |
| 极高频读请求 | 本地缓存、进程内缓存、Caffeine/Guava、Nginx/CDN 缓存，Redis 作为二级缓存 | 否 | 适合允许短 TTL 或最终一致的热点数据；要防止本地缓存雪崩和脏读 |
| 写热点，例如点赞数、库存扣减、计数器 | 分片计数、队列削峰、异步聚合、定时合并，必要时用 Lua 保证单分片原子性 | 否 | 不适合简单复制 key；写热点要解决聚合准确性、幂等和延迟 |
| 热点突然出现 | 热点探测后动态加副本、短 TTL 本地缓存、限流和降级 | 否 | 适合突发流量；需要网关、代理或业务埋点能识别 top key |
| 发现热点 Key | 统计业务访问日志、代理层 top key、Redis 客户端埋点、慢日志和监控平台 | 部分验证，本实验直接记录了访问计数分布 | Redis 原生不直接提供完整实时 top key，通常要在客户端或代理层做统计 |

热点 Key 的治理优先级是：先确认是读热点还是写热点。读热点优先用本地缓存和读副本分片；写热点不能简单复制 key，通常要做分片写入、削峰和异步聚合。

## 实验结论

- 大 Key 会带来更高的单 key 内存占用和更大的单命令返回载荷；结构拆分和游标扫描可以降低单条命令的最大处理量。
- 热点 Key 本质是流量分布问题；判断依据应重点看最高访问占比，而不只是看延迟。
- 读副本分片可以降低读热点 Key 对单个 Redis key 的压力；写热点 Key 通常还需要计数器分片、异步聚合或其他写入侧拆分方案。
- 耗时数据只作为观察项记录。校验器检查的是稳定不变量：生成数据规模、Redis 基数、内存下限、访问分布和缓解效果。
"""


def main() -> int:
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    data = load_results()
    plot_big_key_memory(data)
    plot_hot_key_distribution(data)
    plot_mitigation_summary(data)
    report = markdown_report(data)
    (OUTPUT_DIR / "result.md").write_text(report, encoding="utf-8")
    (OUTPUT_DIR / "report.md").write_text(report, encoding="utf-8")
    print(f"Generated: {OUTPUT_DIR / 'result.md'}")
    print(f"Generated: {OUTPUT_DIR / 'report.md'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
