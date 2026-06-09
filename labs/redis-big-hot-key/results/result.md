# Redis Big Key and Hot Key Lab Result

This result document is generated from a real Docker Compose run. The dataset is created by the experiment program itself, written to Redis, then measured from Redis with commands such as `MEMORY USAGE`, `HLEN`, `LLEN`, `HGETALL`, `HSCAN`, and real `GET` workloads. The checked-in numbers are not hand-written fixtures.

## Dataset

| Dimension | Value |
| --- | ---: |
| Normal string keys | 20000 |
| Normal string payload | 128 bytes |
| Big string payload | 8388608 bytes |
| Big hash fields | 50000 |
| Big list items | 50000 |
| Hot-key keyspace | 10000 keys |
| Hot-key requests | 100000 |
| Hot-key skew | 60% to one logical key |

## Big Key Results

![Big key memory](big-key-memory.png)

| Scenario | Measurement |
| --- | ---: |
| Average normal string memory | 224 bytes |
| Big string memory | 10485816 bytes |
| Big string logical length | 8388608 bytes |
| Big hash memory | 6786544 bytes |
| Big hash field count | 50000 |
| Big list memory | 3679800 bytes |
| Big list item count | 50000 |

The big string, hash, and list are all valid Redis objects, but they concentrate much more memory and command work on one key than ordinary keys. That is the core risk of a big key: not that Redis cannot store it, but that single commands and single-key ownership become too heavy.

The full hash read returned `50000` fields in one command. The cursor scan also read `50000` fields, but split the work into `100` calls. Timings are host-dependent, but the command shape is stable: `HGETALL` creates one large response, while `HSCAN` lets the caller slice the work.

The split-hash mitigation preserved all `50000` fields while reducing the largest bucket to `500` fields across `100` keys. Splitting does not reduce total data volume; it reduces maximum per-key and per-command pressure.

## Hot Key Results

![Hot key distribution](hot-key-distribution.png)

| Scenario | Top key | Top count | Top share |
| --- | --- | ---: | ---: |
| Uniform access | `item:7732` | 23 | 0.0002 |
| Single hot key | `item:0000` | 60000 | 0.6000 |
| Sharded hot key | `item:hot:00` | 3750 | 0.0375 |

The uniform workload spreads reads across `9999` keys, so no key dominates. The hot-key workload sends `60000` of `100000` requests to `item:0000`, proving the traffic-skew problem directly from access counts.

The read-copy mitigation keeps the same logical hot item but fans it out over `16` physical keys. The hottest copy receives `3750` reads instead of `60000`, reducing pressure on any one Redis key.

![Summary](summary.png)

## Conclusions

- Big keys create large per-key memory and large per-command payloads; split structures and cursor scans reduce maximum per-command work.
- Hot keys are a traffic distribution problem; the evidence is the top-key share, not just latency.
- Read-copy sharding reduces Redis pressure for read hot keys. Write hot keys need separate counter sharding or aggregation patterns.
- Durations are recorded as observations. The verifier checks stable invariants: generated data size, Redis cardinality, memory lower bounds, access distribution, and mitigation effects.
