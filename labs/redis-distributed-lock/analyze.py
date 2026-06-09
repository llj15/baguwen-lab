#!/usr/bin/env python3
"""Generate charts and a report for the Redis distributed lock lab."""

from __future__ import annotations

import json
import os
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np


OUTPUT_DIR = Path(os.environ.get("OUTPUT_DIR", "/data"))


def load_json(filename: str) -> dict | None:
    path = OUTPUT_DIR / filename
    if not path.exists():
        return None
    return json.loads(path.read_text(encoding="utf-8"))


def save_current_figure(filename: str) -> None:
    path = OUTPUT_DIR / filename
    plt.tight_layout()
    plt.savefig(path, dpi=150, bbox_inches="tight")
    plt.close()
    print(f"Generated: {path}")


def add_bar_labels(axis, values, suffix: str = "") -> None:
    max_value = max(values) if values else 0
    offset = max(max_value * 0.02, 1)
    for index, value in enumerate(values):
        axis.text(index, value + offset, f"{value}{suffix}", ha="center", fontweight="bold", fontsize=9)


def plot_basic_lock(data: dict | None) -> None:
    if not data:
        return
    results = data["results"]
    labels = ["No lock", "SET NX EX", "Short TTL"]
    expected = [item["expected"] for item in results]
    actual = [item["actual"] for item in results]
    loss_pct = [item["lost_pct"] for item in results]
    durations = [item["duration_ms"] for item in results]

    fig, axes = plt.subplots(1, 3, figsize=(18, 5))
    x = np.arange(len(labels))
    width = 0.35

    axes[0].bar(x - width / 2, expected, width, label="Expected", color="#2f6f9f")
    axes[0].bar(x + width / 2, actual, width, label="Actual", color="#31a354")
    axes[0].set_title("Counter correctness", fontweight="bold")
    axes[0].set_xticks(x)
    axes[0].set_xticklabels(labels)
    axes[0].set_ylabel("Counter value")
    axes[0].legend()

    colors = ["#c0392b" if value > 0 else "#31a354" for value in loss_pct]
    axes[1].bar(labels, loss_pct, color=colors, alpha=0.85)
    axes[1].set_title("Lost update rate", fontweight="bold")
    axes[1].set_ylabel("Loss (%)")
    add_bar_labels(axes[1], [round(value, 1) for value in loss_pct], "%")

    axes[2].bar(labels, durations, color="#756bb1", alpha=0.85)
    axes[2].set_title("Runtime", fontweight="bold")
    axes[2].set_ylabel("Milliseconds")
    add_bar_labels(axes[2], durations, "ms")

    fig.suptitle("Experiment 1: Basic Redis Lock", fontsize=15, fontweight="bold")
    save_current_figure("01_basic_lock.png")


def plot_redlock(data: dict | None) -> None:
    if not data:
        return
    results = data["results"]
    labels = ["3 nodes healthy", "1 node down"]
    expected = [item["expected"] for item in results]
    actual = [item["actual"] for item in results]
    retries = [item["retries"] for item in results]
    durations = [item["duration_ms"] for item in results]

    fig, axes = plt.subplots(1, 3, figsize=(18, 5))
    x = np.arange(len(labels))
    width = 0.35

    axes[0].bar(x - width / 2, expected, width, label="Expected", color="#2f6f9f")
    axes[0].bar(x + width / 2, actual, width, label="Actual", color="#31a354")
    axes[0].set_title("Quorum lock correctness", fontweight="bold")
    axes[0].set_xticks(x)
    axes[0].set_xticklabels(labels)
    axes[0].set_ylabel("Counter value")
    axes[0].legend()

    axes[1].bar(labels, retries, color=["#3182bd", "#e6550d"], alpha=0.85)
    axes[1].set_title("Contention retries", fontweight="bold")
    axes[1].set_ylabel("Retries")
    add_bar_labels(axes[1], retries)

    axes[2].bar(labels, durations, color=["#756bb1", "#d94801"], alpha=0.85)
    axes[2].set_title("Runtime", fontweight="bold")
    axes[2].set_ylabel("Milliseconds")
    add_bar_labels(axes[2], durations, "ms")

    fig.suptitle("Experiment 2: Redlock Quorum", fontsize=15, fontweight="bold")
    save_current_figure("02_redlock.png")


def plot_watchdog(data: dict | None) -> None:
    if not data:
        return
    results = data["results"]
    comparison = [item for item in results if item["mode"] != "watchdog_timeline"]
    timeline = next((item for item in results if item["mode"] == "watchdog_timeline"), None)

    fig, axes = plt.subplots(1, 3, figsize=(18, 5))
    labels = ["No watchdog", "With watchdog"]
    expected = [item["expected"] for item in comparison]
    actual = [item["actual"] for item in comparison]
    x = np.arange(len(labels))
    width = 0.35

    axes[0].bar(x - width / 2, expected, width, label="Expected", color="#2f6f9f")
    axes[0].bar(x + width / 2, actual, width, label="Actual", color="#31a354")
    axes[0].set_title("Long critical section safety", fontweight="bold")
    axes[0].set_xticks(x)
    axes[0].set_xticklabels(labels)
    axes[0].set_ylabel("Counter value")
    axes[0].legend()

    metrics = [comparison[0].get("violations", 0), comparison[1].get("total_renewals", 0)]
    axes[1].bar(["TTL violations", "Renewals"], metrics, color=["#c0392b", "#31a354"], alpha=0.85)
    axes[1].set_title("Safety metrics", fontweight="bold")
    axes[1].set_ylabel("Count")
    add_bar_labels(axes[1], metrics)

    if timeline and timeline.get("events"):
        events = timeline["events"]
        times = [event["time_ms"] for event in events]
        ttls = [event["ttl_ms"] for event in events]
        axes[2].plot(times, ttls, "o-", markersize=3, color="#2f6f9f", label="Remaining TTL")
        axes[2].axhline(y=0, color="#c0392b", linestyle="--", alpha=0.6, label="Expired")
        axes[2].axhline(y=timeline["lock_ttl_ms"], color="#31a354", linestyle="--", alpha=0.6, label="Initial TTL")
        axes[2].set_xlabel("Time (ms)")
        axes[2].set_ylabel("TTL remaining (ms)")
        axes[2].set_title("Watchdog renewal timeline", fontweight="bold")
        axes[2].legend(fontsize=8)
    else:
        axes[2].text(0.5, 0.5, "No timeline data", ha="center", va="center", transform=axes[2].transAxes)

    fig.suptitle("Experiment 3: Watchdog Renewal", fontsize=15, fontweight="bold")
    save_current_figure("03_watchdog.png")


def row(values: list[object]) -> str:
    return "| " + " | ".join(str(value) for value in values) + " |\n"


def generate_report() -> None:
    basic = load_json("01_basic_lock.json")
    redlock = load_json("02_redlock.json")
    watchdog = load_json("03_watchdog.json")

    report = """# Redis Distributed Lock Lab Report

This report is generated from a real Docker Compose run. It demonstrates three interview-grade Redis lock topics:

- Basic `SET key value NX EX ttl` locking and safe Lua unlock.
- Redlock quorum behavior with three independent Redis instances.
- Watchdog renewal for critical sections that run longer than the initial lock TTL.

Durations and retry counts are intentionally treated as host-dependent observations. The verifier checks stable theory invariants and bounded outcomes.

## Environment

| Component | Value |
| --- | --- |
| Runtime | Docker Compose |
| Redis | 3 independent Redis 7 instances |
| Language | Go |
| Client | go-redis/v9 |
| Analysis | Python, matplotlib, numpy |

## Experiment 1: Basic Lock

`SET NX EX` protects a read-modify-write counter. The short-TTL variant shows that an expired lock can lose ownership before the critical section finishes.

![Basic lock chart](01_basic_lock.png)

| Mode | Expected | Actual | Lost | Loss % | Extra | Duration |
| --- | ---: | ---: | ---: | ---: | --- | ---: |
"""
    if basic:
        for item in basic["results"]:
            extra = f"retries={item.get('retries', '-')}, wrong_release={item.get('wrong_release', '-')}"
            report += row(
                [
                    item["mode"],
                    item["expected"],
                    item["actual"],
                    item["lost"],
                    f"{item['lost_pct']:.1f}",
                    extra,
                    f"{item['duration_ms']}ms",
                ]
            )

    report += """
Expected conclusion: no lock loses updates, `SET NX EX` keeps the counter exact, and a TTL shorter than the critical section is unsafe even when a run happens not to lose counter increments.

## Experiment 2: Redlock

Redlock takes a majority vote across independent Redis instances. With 3 nodes, quorum is 2, so one unreachable node can be tolerated.

![Redlock chart](02_redlock.png)

| Mode | Alive nodes | Quorum | Expected | Actual | Lost | Retries | Duration |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
"""
    if redlock:
        for item in redlock["results"]:
            total = item.get("total_nodes", item.get("nodes", 3))
            alive = item.get("alive_nodes", total)
            report += row(
                [
                    item["mode"],
                    f"{alive}/{total}",
                    item["quorum"],
                    item["expected"],
                    item["actual"],
                    item["lost"],
                    item["retries"],
                    f"{item['duration_ms']}ms",
                ]
            )

    report += """
Expected conclusion: both Redlock scenarios keep the counter exact; the simulated failed node increases latency and retries but still satisfies quorum.

## Experiment 3: Watchdog

The watchdog renews the lock every `ttl/3` while the owner is alive, preventing the lock from expiring during long work.

![Watchdog chart](03_watchdog.png)

| Mode | Lock TTL | Work time | Expected | Actual | Lost | Safety metric | Duration |
| --- | ---: | ---: | ---: | ---: | ---: | --- | ---: |
"""
    if watchdog:
        for item in [entry for entry in watchdog["results"] if entry["mode"] != "watchdog_timeline"]:
            if "violations" in item:
                metric = f"violations={item['violations']}"
            else:
                metric = f"renewals={item['total_renewals']}"
            report += row(
                [
                    item["mode"],
                    f"{item['lock_ttl_ms']}ms",
                    f"{item['work_time_ms']}ms",
                    item["expected"],
                    item["actual"],
                    item["lost"],
                    metric,
                    f"{item['duration_ms']}ms",
                ]
            )
        timeline = next((entry for entry in watchdog["results"] if entry["mode"] == "watchdog_timeline"), None)
        if timeline:
            report += f"\nTimeline renewals: {timeline['renewals']}; sampled events: {len(timeline.get('events', []))}.\n"

    report += """
Expected conclusion: without watchdog renewal, the lock expires during work and multiple workers enter the critical section; with watchdog renewal, the counter stays exact.

## Interview Takeaways

- Correct Redis locks need atomic acquire, bounded TTL, a unique owner value, and Lua-based owner-checked release.
- Redlock improves availability over a single Redis node by requiring `N/2+1` successful lock writes, but it still depends on bounded timing assumptions.
- Watchdog renewal addresses long-running business logic; it is not a replacement for explicit unlock and only works while the lock owner process is alive.
"""

    path = OUTPUT_DIR / "report.md"
    path.write_text(report, encoding="utf-8")
    print(f"Generated: {path}")


def main() -> int:
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    print(f"Analyzing experiment results in {OUTPUT_DIR}...")
    plot_basic_lock(load_json("01_basic_lock.json"))
    plot_redlock(load_json("02_redlock.json"))
    plot_watchdog(load_json("03_watchdog.json"))
    generate_report()
    print("Analysis complete.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
