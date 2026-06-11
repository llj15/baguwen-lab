# Kafka Demo

Hands-on Kafka experiments for learning partitioning, per-key ordering, consumer group parallelism, and consumer lag with a real event stream. The lab downloads a fixed GH Archive hour, verifies its checksum, produces every event to Kafka, consumes the events back, and verifies stable theory invariants.

## What It Demonstrates

| Area | Scenario | Lesson |
| --- | --- | --- |
| Real event stream | GH Archive `2024-01-01-0.json.gz` | Kafka workloads are naturally modeled as append-only events |
| Partitioning | No key, `repo.id` key, `actor.id` key | Partition choice controls load distribution and ordering scope |
| Ordering | Keyed topics consumed from Kafka | Kafka preserves order within one partition, so one key stays ordered |
| Consumer groups | 1, 3, 6, 12, and 16 consumers | Active parallelism is capped by topic partition count |
| Lag | Produced backlog then full drain | Lag is produced-but-unconsumed records and falls only when consumers catch up |

## Dataset

The experiment downloads the public GH Archive file at run time:

```text
https://data.gharchive.org/2024-01-01-0.json.gz
```

The file is verified before use:

| Dimension | Value |
| --- | ---: |
| MD5 | `d4bd9ce833f217e95ffb3fd958138827` |
| Compressed size | 74,527,660 bytes |
| Event records | 153,619 |
| Event time range | `2024-01-01T00:00:00Z` to `2024-01-01T00:59:59Z` |
| Topic partitions | 12 |

This is real public GitHub event data, not hand-written fixture data. The dataset is representative for Kafka because it has event ids, event types, actor ids, repository ids, a natural event-time order, and real key skew.

## Reproducible Environment

- Compose platform defaults to `linux/amd64` so Windows, macOS, Linux, and ARM hosts run the same container target.
- Apache Kafka, Go, Alpine, and Python base images are pinned by digest in `docker-compose.yml` and `Dockerfile`.
- The experiment downloads a fixed immutable GH Archive hour and checks its MD5 before parsing.
- The Go Kafka client dependency is pinned in `go.mod` and `go.sum`.
- Generated artifacts can be written outside tracked sample results with `RESULTS_DIR`.
- The verifier checks stable theory invariants. Durations and throughput are recorded as observations because they depend on host CPU, network, and Docker scheduling.

## Run

Windows PowerShell:

```powershell
.\run.ps1 -ResultsDir .\tmp-results
```

macOS/Linux/Git Bash:

```bash
RESULTS_DIR=./tmp-results bash ./run.sh
```

Manual Compose flow:

```bash
docker compose config
RESULTS_DIR=./tmp-results docker compose up --build --abort-on-container-exit --exit-code-from experiment experiment
RESULTS_DIR=./tmp-results docker compose up --build --no-deps --abort-on-container-exit --exit-code-from analysis analysis
RESULTS_DIR=./tmp-results docker compose run --rm --no-deps analysis python /app/scripts/verify_results.py /data/results.json
docker compose down --remove-orphans
```

## Generated Files

The selected results directory contains:

```text
results.json
result.md
report.md
partition-skew.svg
consumer-groups.svg
lag-drain.svg
```

`result.md` and `report.md` contain the same narrative result so the lab keeps the requested `result.md` name while matching the existing lab convention of `report.md`.

## Expected Stable Results

| Area | Scenario | Expected stable result |
| --- | --- | --- |
| Dataset | GH Archive checksum | MD5 equals `d4bd9ce833f217e95ffb3fd958138827` |
| Dataset | Parsed events | Exactly 153,619 events with required Kafka key fields |
| Partitioning | No key / round-robin | All 12 partitions are used with max-min count difference <= 1 |
| Partitioning | `repo.id` key | Same repository id stays on one partition with no per-key ordering violation |
| Partitioning | `actor.id` key | Same actor id stays on one partition with no per-key ordering violation |
| Consumer group | 1, 3, 6, 12, 16 consumers | Active consumers equal `min(requested_consumers, 12)` |
| Lag | Produced backlog | Initial lag equals produced records and final lag is 0 after drain |

Use `tmp-results/` for local reruns when comparing without touching checked-in sample output.

## Repository Layout

```text
labs/kafka-demo
|-- scripts/
|-- results/
|-- Dockerfile
|-- docker-compose.yml
|-- run.sh
|-- run.ps1
|-- main.go
|-- analysis.py
|-- go.mod
|-- go.sum
`-- Makefile
```

## License

MIT
