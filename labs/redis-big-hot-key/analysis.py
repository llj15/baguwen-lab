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

    return f"""# Redis Big Key and Hot Key Lab Result

This result document is generated from a real Docker Compose run. The dataset is created by the experiment program itself, written to Redis, then measured from Redis with commands such as `MEMORY USAGE`, `HLEN`, `LLEN`, `HGETALL`, `HSCAN`, and real `GET` workloads. The checked-in numbers are not hand-written fixtures.

## Dataset

| Dimension | Value |
| --- | ---: |
| Normal string keys | {cfg['normal_string_count']} |
| Normal string payload | {cfg['normal_value_bytes']} bytes |
| Big string payload | {cfg['big_string_bytes']} bytes |
| Big hash fields | {cfg['big_hash_fields']} |
| Big list items | {cfg['big_list_items']} |
| Hot-key keyspace | {cfg['hot_keyspace']} keys |
| Hot-key requests | {cfg['hot_requests']} |
| Hot-key skew | {cfg['hot_ratio_percent']}% to one logical key |

## Big Key Results

![Big key memory](big-key-memory.png)

| Scenario | Measurement |
| --- | ---: |
| Average normal string memory | {normal['normal_avg_memory']} bytes |
| Big string memory | {big_string['memory_bytes']} bytes |
| Big string logical length | {big_string['strlen']} bytes |
| Big hash memory | {big_hash['memory_bytes']} bytes |
| Big hash field count | {big_hash['fields']} |
| Big list memory | {big_list['memory_bytes']} bytes |
| Big list item count | {big_list['items']} |

The big string, hash, and list are all valid Redis objects, but they concentrate much more memory and command work on one key than ordinary keys. That is the core risk of a big key: not that Redis cannot store it, but that single commands and single-key ownership become too heavy.

The full hash read returned `{hash_read['hgetall_fields']}` fields in one command. The cursor scan also read `{hash_read['hscan_fields']}` fields, but split the work into `{hash_read['hscan_iterations']}` calls. Timings are host-dependent, but the command shape is stable: `HGETALL` creates one large response, while `HSCAN` lets the caller slice the work.

The split-hash mitigation preserved all `{split_hash['total_fields']}` fields while reducing the largest bucket to `{split_hash['max_bucket_fields']}` fields across `{split_hash['bucket_count']}` keys. Splitting does not reduce total data volume; it reduces maximum per-key and per-command pressure.

## Hot Key Results

![Hot key distribution](hot-key-distribution.png)

| Scenario | Top key | Top count | Top share |
| --- | --- | ---: | ---: |
| Uniform access | `{uniform['top_key']}` | {uniform['top_count']} | {uniform['top_ratio']:.4f} |
| Single hot key | `{hot['top_key']}` | {hot['top_count']} | {hot['top_ratio']:.4f} |
| Sharded hot key | `{sharded['top_key']}` | {sharded['top_count']} | {sharded['top_ratio']:.4f} |

The uniform workload spreads reads across `{uniform['unique_keys']}` keys, so no key dominates. The hot-key workload sends `{hot['top_count']}` of `{hot['requests']}` requests to `{hot['top_key']}`, proving the traffic-skew problem directly from access counts.

The read-copy mitigation keeps the same logical hot item but fans it out over `{sharded['hot_copy_count']}` physical keys. The hottest copy receives `{sharded['max_copy_gets']}` reads instead of `{hot['top_count']}`, reducing pressure on any one Redis key.

![Summary](summary.png)

## Conclusions

- Big keys create large per-key memory and large per-command payloads; split structures and cursor scans reduce maximum per-command work.
- Hot keys are a traffic distribution problem; the evidence is the top-key share, not just latency.
- Read-copy sharding reduces Redis pressure for read hot keys. Write hot keys need separate counter sharding or aggregation patterns.
- Durations are recorded as observations. The verifier checks stable invariants: generated data size, Redis cardinality, memory lower bounds, access distribution, and mitigation effects.
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
