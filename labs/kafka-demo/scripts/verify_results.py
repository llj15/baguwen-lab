#!/usr/bin/env python3
"""Validate Kafka demo lab outputs."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


EXPECTED_MD5 = "d4bd9ce833f217e95ffb3fd958138827"
EXPECTED_EVENTS = 153_619
PARTITIONS = 12


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


def experiment_scenarios(data: dict[str, Any], experiment_name: str) -> list[dict[str, Any]]:
    for experiment in data.get("experiments", []):
        if experiment.get("name") == experiment_name:
            return [item.get("metrics", {}) for item in experiment.get("scenarios", [])]
    fail(f"missing experiment: {experiment_name}")


def main() -> int:
    path = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("results/results.json")
    data = load(path)
    dataset = data.get("dataset", {})

    require(data.get("schema_version") == 1, "schema_version should be 1")
    require(data.get("lab") == "kafka-demo", "lab name should be kafka-demo")
    require(dataset.get("url") == "https://data.gharchive.org/2024-01-01-0.json.gz", "dataset URL mismatch")
    require(dataset.get("md5") == EXPECTED_MD5, "dataset MD5 mismatch")
    require(dataset.get("event_count") == EXPECTED_EVENTS, "event count mismatch")
    require(dataset.get("compressed_bytes", 0) > 70_000_000, "compressed data should be large enough")
    require(dataset.get("distinct_event_types", 0) >= 10, "dataset should have at least 10 event types")
    require(dataset.get("distinct_repos", 0) >= 50_000, "dataset should include many repositories")
    require(dataset.get("distinct_actors", 0) >= 10_000, "dataset should include many actors")
    require(dataset.get("required_fields_ok") is True, "required event fields should be present")

    production = experiment_scenarios(data, "production")
    require(len(production) == 3, "production should include three topic scenarios")
    for metrics in production:
        require(metrics.get("records") == EXPECTED_EVENTS, f"{metrics.get('topic')} production count mismatch")

    rr = scenario(data, "partitioning_and_ordering", "round_robin")
    repo = scenario(data, "partitioning_and_ordering", "repo_keyed")
    actor = scenario(data, "partitioning_and_ordering", "actor_keyed")
    for name, metrics in [("round_robin", rr), ("repo_keyed", repo), ("actor_keyed", actor)]:
        require(metrics.get("records") == EXPECTED_EVENTS, f"{name} consumed count mismatch")
        require(metrics.get("partitions_used") == PARTITIONS, f"{name} should use all partitions")
        require(len(metrics.get("partition_counts", [])) == PARTITIONS, f"{name} partition count vector mismatch")
        require(sum(metrics.get("partition_counts", [])) == EXPECTED_EVENTS, f"{name} partition counts should add up")
        require(metrics.get("final_lag_records") == 0, f"{name} should drain lag to zero")

    require(rr.get("max_partition_count") - rr.get("min_partition_count") <= 1, "round-robin partitioning should be balanced")
    require(repo.get("key_partition_violations") == 0, "repo key should stay on one partition")
    require(repo.get("per_key_order_violations") == 0, "repo key order should be preserved")
    require(actor.get("key_partition_violations") == 0, "actor key should stay on one partition")
    require(actor.get("per_key_order_violations") == 0, "actor key order should be preserved")
    require(actor.get("max_min_ratio", 0) >= repo.get("max_min_ratio", 0), "actor-key partitioning should show at least repo-level skew")

    groups = experiment_scenarios(data, "consumer_group_parallelism")
    require(len(groups) == 5, "consumer group experiment should include five group sizes")
    for metrics in groups:
        requested = metrics.get("requested_consumers")
        expected_active = min(requested, PARTITIONS)
        require(metrics.get("active_consumers") == expected_active, f"group size {requested} active consumer count mismatch")
        require(metrics.get("partition_count") == PARTITIONS, "group partition count mismatch")
        require(metrics.get("records") == 1200, "group sample should consume 1200 real event records")

    lag = scenario(data, "consumer_lag_drain", "repo_keyed_backlog")
    require(lag.get("initial_lag_records") == EXPECTED_EVENTS, "initial lag should equal produced record count")
    require(lag.get("checkpoint_consumed") == 5000, "lag checkpoint should be taken at 5000 records")
    require(lag.get("checkpoint_lag_records") == EXPECTED_EVENTS - 5000, "checkpoint lag mismatch")
    require(lag.get("final_lag_records") == 0, "final lag should be zero after draining")

    print(f"OK: {path} satisfies Kafka demo lab invariants.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
