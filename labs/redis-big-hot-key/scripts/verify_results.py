#!/usr/bin/env python3
"""Validate Redis big key and hot key lab outputs."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


def fail(message: str) -> None:
    print(f"FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def require(condition: bool, message: str) -> None:
    if not condition:
        fail(message)


def load(path: Path) -> dict[str, Any]:
    if not path.exists():
        fail(f"missing result file: {path}")
    return json.loads(path.read_text(encoding="utf-8"))


def scenario(data: dict[str, Any], experiment_name: str, scenario_name: str) -> dict[str, Any]:
    for experiment in data.get("experiments", []):
        if experiment.get("name") == experiment_name:
            for item in experiment.get("scenarios", []):
                if item.get("name") == scenario_name:
                    return item.get("metrics", {})
    fail(f"missing scenario: {experiment_name}/{scenario_name}")


def main() -> int:
    path = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("results/results.json")
    data = load(path)
    cfg = data.get("dataset", {})

    require(data.get("schema_version") == 1, "schema_version should be 1")
    require(data.get("lab") == "redis-big-hot-key", "lab name should be redis-big-hot-key")
    require(data.get("seed") == 20260609, "seed should be fixed")
    require(cfg.get("normal_string_count") == 20000, "normal string count should be 20000")
    require(cfg.get("big_string_bytes") == 8 * 1024 * 1024, "big string payload should be 8 MiB")
    require(cfg.get("big_hash_fields") == 50000, "big hash should have 50000 fields")
    require(cfg.get("big_hash_buckets") == 100, "split hash should use 100 buckets")
    require(cfg.get("big_list_items") == 50000, "big list should have 50000 items")
    require(cfg.get("hot_keyspace") == 10000, "hot keyspace should have 10000 keys")
    require(cfg.get("hot_requests") == 100000, "hot workload should have 100000 requests")
    require(cfg.get("hot_ratio_percent") == 60, "hot workload should target 60 percent skew")
    require(cfg.get("hot_shards") == 16, "hot read copies should use 16 shards")

    normal = scenario(data, "redis_big_key", "representative_dataset")
    big_string = scenario(data, "redis_big_key", "big_string")
    big_hash = scenario(data, "redis_big_key", "big_hash")
    big_list = scenario(data, "redis_big_key", "big_list")
    split_hash = scenario(data, "redis_big_key", "split_hash_mitigation")
    hash_read = scenario(data, "redis_big_key", "hash_full_read_vs_scan")

    require(normal["normal_avg_memory"] > 0, "normal key memory should be measured")
    require(big_string["strlen"] == cfg["big_string_bytes"], "big string length should match generated payload")
    require(big_string["memory_bytes"] >= cfg["big_string_bytes"], "big string memory should be at least payload size")
    require(big_string["memory_vs_normal"] >= 10000, "big string should be much larger than normal keys")
    require(big_hash["fields"] == cfg["big_hash_fields"], "big hash field count mismatch")
    require(big_hash["memory_bytes"] >= 2 * 1024 * 1024, "big hash should use at least 2 MiB")
    require(big_list["items"] == cfg["big_list_items"], "big list item count mismatch")
    require(big_list["memory_bytes"] >= 2 * 1024 * 1024, "big list should use at least 2 MiB")
    require(split_hash["bucket_count"] == cfg["big_hash_buckets"], "split hash bucket count mismatch")
    require(split_hash["total_fields"] == cfg["big_hash_fields"], "split hash should preserve every field")
    require(split_hash["max_bucket_fields"] <= 500, "split hash max bucket should be <= 500 fields")
    require(hash_read["hgetall_fields"] == cfg["big_hash_fields"], "HGETALL should return all fields")
    require(hash_read["hscan_fields"] == cfg["big_hash_fields"], "HSCAN should see all fields")
    require(hash_read["hscan_iterations"] >= 50, "HSCAN should split work across many iterations")

    uniform = scenario(data, "redis_hot_key", "uniform_access")
    hot = scenario(data, "redis_hot_key", "single_hot_key_access")
    sharded = scenario(data, "redis_hot_key", "sharded_hot_key_access")

    require(uniform["requests"] == cfg["hot_requests"], "uniform workload request count mismatch")
    require(uniform["unique_keys"] >= 9900, "uniform workload should touch nearly all keys")
    require(uniform["top_ratio"] <= 0.0005, "uniform workload should not have a dominant key")
    require(hot["requests"] == cfg["hot_requests"], "hot workload request count mismatch")
    require(hot["top_key"] == "item:0000", "hot workload should target item:0000")
    require(hot["top_count"] == 60000, "hot workload should send exactly 60000 reads to the hot key")
    require(hot["top_ratio"] >= 0.60, "hot key top ratio should be at least 60 percent")
    require(sharded["requests"] == cfg["hot_requests"], "sharded workload request count mismatch")
    require(sharded["hot_copy_count"] == cfg["hot_shards"], "hot copy count mismatch")
    require(sharded["hot_copy_gets"] == hot["top_count"], "sharded copies should receive the same logical hot reads")
    require(sharded["max_copy_gets"] <= 3750, "read-copy sharding should cap reads per copy at 3750")
    require(sharded["top_ratio"] <= 0.04, "sharded hot key should reduce top-key share")

    print(f"OK: {path} satisfies Redis big key and hot key lab invariants.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
