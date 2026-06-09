#!/usr/bin/env python3
"""Validate Redis distributed lock lab outputs."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any


def fail(message: str) -> None:
    print(f"FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def load_result(results_dir: Path, filename: str) -> dict[str, Any]:
    path = results_dir / filename
    if not path.exists():
        fail(f"missing result file: {path}")
    return json.loads(path.read_text(encoding="utf-8"))


def by_mode(data: dict[str, Any], mode: str) -> dict[str, Any]:
    for item in data.get("results", []):
        if item.get("mode") == mode:
            return item
    fail(f"missing mode: {mode}")


def require(condition: bool, message: str) -> None:
    if not condition:
        fail(message)


def validate_invariants(results_dir: Path) -> None:
    basic = load_result(results_dir, "01_basic_lock.json")
    redlock = load_result(results_dir, "02_redlock.json")
    watchdog = load_result(results_dir, "03_watchdog.json")

    no_lock = by_mode(basic, "no_lock")
    require(no_lock["expected"] == 1000, "no_lock should run 1000 increments")
    require(0 < no_lock["actual"] < no_lock["expected"], "no_lock should lose updates")
    require(no_lock["lost"] == no_lock["expected"] - no_lock["actual"], "no_lock lost count mismatch")

    set_nx_ex = by_mode(basic, "set_nx_ex")
    require(set_nx_ex["expected"] == 1000, "set_nx_ex should run 1000 increments")
    require(set_nx_ex["actual"] == set_nx_ex["expected"], "set_nx_ex should preserve every increment")
    require(set_nx_ex["lost"] == 0, "set_nx_ex should not lose updates")

    short_ttl = by_mode(basic, "short_ttl")
    require(short_ttl["expected"] == 100, "short_ttl should run 100 increments")
    require(short_ttl["wrong_release"] >= 80, "short_ttl should show ownership release failures")
    require(short_ttl["actual"] <= short_ttl["expected"], "short_ttl actual should not exceed expected")

    healthy = by_mode(redlock, "redlock_3_node")
    require(healthy["nodes"] == 3 and healthy["quorum"] == 2, "redlock healthy case should use 3 nodes with quorum 2")
    require(healthy["expected"] == 500, "redlock healthy case should run 500 increments")
    require(healthy["actual"] == healthy["expected"], "redlock healthy case should preserve every increment")
    require(healthy["lost"] == 0, "redlock healthy case should not lose updates")

    failed = by_mode(redlock, "redlock_1_node_down")
    require(failed["total_nodes"] == 3 and failed["alive_nodes"] == 2, "redlock failure case should simulate 2/3 live nodes")
    require(failed["quorum"] == 2, "redlock failure quorum should be 2")
    require(failed["actual"] == failed["expected"] == 500, "redlock failure case should preserve every increment")
    require(failed["lost"] == 0, "redlock failure case should not lose updates")

    no_watchdog = by_mode(watchdog, "no_watchdog")
    require(no_watchdog["expected"] == 25, "no_watchdog should run 25 increments")
    require(0 < no_watchdog["actual"] < no_watchdog["expected"], "no_watchdog should lose updates")
    require(no_watchdog["violations"] >= no_watchdog["expected"], "no_watchdog should show TTL ownership violations")

    with_watchdog = by_mode(watchdog, "with_watchdog")
    require(with_watchdog["expected"] == 25, "with_watchdog should run 25 increments")
    require(with_watchdog["actual"] == with_watchdog["expected"], "with_watchdog should preserve every increment")
    require(with_watchdog["lost"] == 0, "with_watchdog should not lose updates")
    require(with_watchdog["total_renewals"] >= with_watchdog["expected"], "with_watchdog should renew during long work")

    timeline = by_mode(watchdog, "watchdog_timeline")
    events = timeline.get("events", [])
    require(timeline["lock_ttl_ms"] == 300, "timeline TTL should be 300ms")
    require(timeline["renewals"] >= 5, "timeline should include repeated renewals")
    require(len(events) >= 20, "timeline should include enough TTL samples")
    require(events[-1]["event"] == "unlocked", "timeline should end with unlock")


def compare_to_baseline(results_dir: Path, baseline_dir: Path) -> None:
    if not baseline_dir.exists():
        fail(f"baseline directory does not exist: {baseline_dir}")

    current_basic = load_result(results_dir, "01_basic_lock.json")
    current_redlock = load_result(results_dir, "02_redlock.json")
    current_watchdog = load_result(results_dir, "03_watchdog.json")
    baseline_basic = load_result(baseline_dir, "01_basic_lock.json")
    baseline_redlock = load_result(baseline_dir, "02_redlock.json")
    baseline_watchdog = load_result(baseline_dir, "03_watchdog.json")

    for mode in ("set_nx_ex", "short_ttl"):
        current = by_mode(current_basic, mode)
        baseline = by_mode(baseline_basic, mode)
        require(current["expected"] == baseline["expected"], f"{mode} expected changed from baseline")
        require(current["actual"] == baseline["actual"], f"{mode} actual changed from baseline")

    no_lock = by_mode(current_basic, "no_lock")
    require(50 <= no_lock["actual"] <= 500, "no_lock actual is outside the accepted baseline range 50..500")

    for mode in ("redlock_3_node", "redlock_1_node_down"):
        current = by_mode(current_redlock, mode)
        baseline = by_mode(baseline_redlock, mode)
        require(current["expected"] == baseline["expected"], f"{mode} expected changed from baseline")
        require(current["actual"] == baseline["actual"], f"{mode} actual changed from baseline")
        require(current["quorum"] == baseline["quorum"], f"{mode} quorum changed from baseline")

    no_watchdog = by_mode(current_watchdog, "no_watchdog")
    require(5 <= no_watchdog["actual"] <= 20, "no_watchdog actual is outside the accepted baseline range 5..20")

    with_watchdog = by_mode(current_watchdog, "with_watchdog")
    baseline_with_watchdog = by_mode(baseline_watchdog, "with_watchdog")
    require(with_watchdog["actual"] == baseline_with_watchdog["actual"], "with_watchdog actual changed from baseline")
    require(with_watchdog["expected"] == baseline_with_watchdog["expected"], "with_watchdog expected changed from baseline")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("results_dir", nargs="?", default="results", help="Directory containing generated JSON outputs")
    parser.add_argument("--baseline", help="Optional tracked sample results directory for compatibility comparison")
    args = parser.parse_args()

    results_dir = Path(args.results_dir)
    validate_invariants(results_dir)
    if args.baseline:
        compare_to_baseline(results_dir, Path(args.baseline))
    print(f"OK: {results_dir} satisfies Redis distributed lock lab invariants.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
