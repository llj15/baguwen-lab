# Redis Distributed Lock

Hands-on Redis distributed lock experiments for learning `SET NX EX`, Redlock quorum, and watchdog renewal by running the failure modes instead of memorizing answers.

The original experiment logic is preserved and wrapped in the same reproducible lab harness used by the other Baguwen Lab experiments: Docker Compose, pinned container images, tracked sample results, generated charts, and a machine-checkable verifier.

## What It Demonstrates

| Experiment | Failure mode | Safe mechanism shown |
| --- | --- | --- |
| Basic lock | Concurrent read-modify-write without locking loses updates | `SET key value NX EX ttl` plus owner-checked Lua unlock |
| Redlock | A single Redis lock node is a single point of failure | Majority quorum across 3 independent Redis instances |
| Watchdog | A lock can expire before long business logic finishes | Auto-renewal every `ttl/3` while the owner is alive |

## Reproducible Environment

- Compose platform defaults to `linux/amd64` so Windows, macOS, Linux, and ARM hosts run the same container target.
- Redis, Go, Alpine, and Python base images are pinned by digest in `docker-compose.yml` and `Dockerfile`.
- Redis uses `conf/redis.conf` with fixed persistence and memory behavior.
- Python chart dependencies are pinned in `conf/python-requirements.txt`.
- Generated artifacts can be written outside tracked sample results with `RESULTS_DIR`.
- The verifier checks stable theory invariants and bounded outcomes; timings and retry counts are recorded but not used as exact pass/fail values.

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
RESULTS_DIR=./tmp-results docker compose run --rm --no-deps analysis python /app/scripts/verify_results.py /data --baseline /baseline
docker compose down --remove-orphans
```

The full run takes about 90 seconds on a typical laptop because the Redlock node-failure case intentionally waits on a dead Redis endpoint and the watchdog case uses real TTL sleeps.

## Expected Stable Results

| Experiment | Scenario | Expected stable result |
| --- | --- | --- |
| Basic lock | No lock | 1000 requested increments, actual counter is lower |
| Basic lock | `SET NX EX` | 1000 requested increments, actual counter is exactly 1000 |
| Basic lock | Short TTL | 100 requested increments, many owner-checked unlock failures |
| Redlock | 3 healthy nodes | 500 requested increments, actual counter is exactly 500 |
| Redlock | 1 node down | 500 requested increments, actual counter is exactly 500 with higher latency |
| Watchdog | No watchdog | 25 requested increments, actual counter is lower and TTL violations occur |
| Watchdog | With watchdog | 25 requested increments, actual counter is exactly 25 |
| Watchdog | Timeline | TTL samples show repeated renewal before unlock |

Existing files under `results/` are sample output from one real run. Use `tmp-results/` for local reruns when comparing without touching the checked-in sample.

## Repository Layout

```text
labs/redis-distributed-lock
|-- 01_basic_lock/
|-- 02_redlock/
|-- 03_watchdog/
|-- conf/
|-- scripts/
|-- results/
|-- Dockerfile
|-- docker-compose.yml
|-- run.sh
|-- run.ps1
|-- analysis.py
`-- Makefile
```

## License

MIT
