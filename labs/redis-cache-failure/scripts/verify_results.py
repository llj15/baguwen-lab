#!/usr/bin/env python3
"""Validate the stable Redis cache experiment invariants."""

from __future__ import annotations

import json
import sys
from pathlib import Path


def fail(message: str) -> None:
    print(f"FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def scenario(exp: dict, name: str) -> dict:
    for item in exp.get("scenarios", []):
        if item.get("name") == name:
            return item
    fail(f"missing scenario: {exp.get('name')} / {name}")


def extra_value(item: dict, key: str) -> int:
    extra = item.get("extra", "")
    values = {}
    for part in extra.split(","):
        if "=" in part:
            k, v = part.split("=", 1)
            values[k.strip()] = v.strip()
    if key not in values:
        fail(f"missing extra value {key!r} in scenario {item.get('name')!r}")
    try:
        return int(values[key])
    except ValueError:
        fail(f"extra value {key!r} is not an integer: {values[key]!r}")


def main() -> int:
    path = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("results/results.json")
    data = json.loads(path.read_text(encoding="utf-8"))
    experiments = {item["name"]: item for item in data.get("experiments", [])}

    for name in ("缓存穿透", "缓存击穿", "缓存雪崩"):
        if name not in experiments:
            fail(f"missing experiment: {name}")

    penetration = experiments["缓存穿透"]
    no_protection = scenario(penetration, "无保护")
    empty_first = scenario(penetration, "缓存空值_首轮")
    empty_second = scenario(penetration, "缓存空值_二轮")
    bloom = scenario(penetration, "布隆过滤器")
    expected_penetration_requests = no_protection["request_count"]
    if expected_penetration_requests != 1000:
        fail("cache penetration should use 1000 requests")
    if no_protection["db_hits"] != expected_penetration_requests:
        fail("penetration/no protection should hit DB for every request")
    if empty_first["db_hits"] != expected_penetration_requests:
        fail("penetration/empty cache first round should hit DB once per missing key")
    if empty_second["db_hits"] != 0:
        fail("penetration/empty cache second round should not hit DB")
    if bloom["db_hits"] != 0:
        fail("penetration/bloom filter should not hit DB for rejected IDs")
    if extra_value(bloom, "bloom_rejected") != expected_penetration_requests:
        fail("bloom filter should reject all invalid IDs")

    breakdown = experiments["缓存击穿"]
    breakdown_requests = scenario(breakdown, "无保护")["request_count"]
    if breakdown_requests != 200:
        fail("cache breakdown should use 200 concurrent requests")
    if scenario(breakdown, "无保护")["db_hits"] != breakdown_requests:
        fail("breakdown/no protection should hit DB for every concurrent request")
    for name in ("singleflight", "朴素分布式锁", "安全分布式锁"):
        if scenario(breakdown, name)["db_hits"] != 1:
            fail(f"breakdown/{name} should collapse DB hits to exactly one")

    avalanche = experiments["缓存雪崩"]
    same_ttl = scenario(avalanche, "相同TTL")
    random_ttl = scenario(avalanche, "随机TTL")
    multi_level = scenario(avalanche, "多级缓存")
    expected_avalanche_requests = same_ttl["request_count"]
    if expected_avalanche_requests != 200:
        fail("cache avalanche should use 200 product keys")
    if same_ttl["db_hits"] != expected_avalanche_requests:
        fail("avalanche/same TTL should hit DB for every expired key")
    if not 80 <= random_ttl["db_hits"] <= 120:
        fail("avalanche/random TTL should expire about half of the keys")
    if multi_level["db_hits"] != 0:
        fail("avalanche/multi-level cache should not hit DB")
    if extra_value(multi_level, "l1_hits") != expected_avalanche_requests:
        fail("multi-level cache should serve every request from L1")

    print(f"OK: {path} satisfies Redis cache failure theory invariants.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
