# Redis Big Key and Hot Key

Hands-on Redis experiments for learning two common interview topics: big keys and hot keys. The lab creates a deterministic dataset inside Redis, measures it with real Redis commands, generates charts, and verifies stable theory invariants.

## What It Demonstrates

| Area | Scenario | Lesson |
| --- | --- | --- |
| Big key | 8 MiB string, 50k-field hash, 50k-item list | Redis can store the object, but single-key memory and command payload become unsafe |
| Big key mitigation | Split hash buckets and cursor scan | Split data and incremental reads reduce maximum per-command work |
| Hot key | 100k reads with 60% traffic to one logical key | Hot keys are traffic skew, visible in top-key share |
| Hot key mitigation | 16 read copies | Read fanout lowers pressure on any one physical Redis key |

## Dataset

The experiment program generates the dataset during the Docker run with fixed seed `20260609`.

| Dimension | Value |
| --- | ---: |
| Normal string keys | 20,000 |
| Normal string payload | 128 bytes |
| Big string payload | 8 MiB |
| Big hash fields | 50,000 |
| Big list items | 50,000 |
| Hot-key keyspace | 10,000 keys |
| Hot-key workload | 100,000 reads |
| Hot-key skew | 60% of reads to one logical key |
| Read-copy mitigation | 16 physical copies |

This is synthetic data, but it is not hand-written output. Each run writes the dataset to Redis, executes the workloads, and records measurements from Redis and the client.

## Reproducible Environment

- Compose platform defaults to `linux/amd64` so Windows, macOS, Linux, and ARM hosts run the same container target.
- Redis, Go, Alpine, and Python base images are pinned by digest in `docker-compose.yml` and `Dockerfile`.
- Redis uses `conf/redis.conf` with fixed persistence and memory behavior.
- Python chart dependencies are pinned in `conf/python-requirements.txt`.
- Generated artifacts can be written outside tracked sample results with `RESULTS_DIR`.
- The verifier checks stable theory invariants. Timings are recorded as observations because they depend on host CPU and Docker scheduling.

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
big-key-memory.png
hot-key-distribution.png
summary.png
```

`result.md` and `report.md` contain the same narrative result so the lab keeps the requested `result.md` name while matching the existing lab convention of `report.md`.

## Expected Stable Results

| Area | Scenario | Expected stable result |
| --- | --- | --- |
| Big key | Big string | `strlen` is exactly 8 MiB and Redis memory is at least the payload size |
| Big key | Big hash | Redis stores exactly 50,000 fields and full read returns all fields |
| Big key | Big list | Redis stores exactly 50,000 items |
| Big key | Split hash | 100 buckets preserve all 50,000 fields and cap each bucket at 500 fields |
| Hot key | Uniform access | 100,000 requests touch nearly all 10,000 keys without a dominant key |
| Hot key | Single hot key | One key receives exactly 60,000 of 100,000 reads |
| Hot key | Sharded hot key | 16 copies preserve the logical hot reads while capping reads per copy at 3,750 |

Use `tmp-results/` for local reruns when comparing without touching any future checked-in sample output.

## Repository Layout

```text
labs/redis-big-hot-key
|-- conf/
|-- scripts/
|-- results/
|-- Dockerfile
|-- docker-compose.yml
|-- run.sh
|-- run.ps1
|-- main.go
|-- analysis.py
`-- Makefile
```

## License

MIT
