# Redis Cache Failure

Hands-on Redis cache experiments for learning cache penetration, cache breakdown, and cache avalanche by running the failure modes instead of memorizing interview answers.

This project keeps the original experiment logic intact and standardizes the reproducibility layer around it: Docker Compose, pinned container images, a fixed Redis configuration, deterministic Go random TTL behavior, pinned Python analysis dependencies, and a machine-checkable result verifier.

## What It Demonstrates

| Experiment | Failure mode | Baseline | Mitigation shown |
| --- | --- | --- | --- |
| Cache penetration | Requests for nonexistent data repeatedly hit the database | 1000 DB hits | Empty-value cache and Bloom filter rejection |
| Cache breakdown | A hot key expires while many requests arrive together | 200 DB hits | `singleflight` and Redis locks |
| Cache avalanche | Many keys expire at nearly the same time | 200 DB hits | Randomized TTL and L1/L2 multi-level cache |

The stable theoretical invariants are checked by `scripts/verify_results.py`. Durations are intentionally not checked as exact values because they depend on CPU, Docker scheduling, and host load. The randomized TTL case is checked as a bounded "about half" result because Redis key TTL countdown starts during warmup, so exact DB hits can move by a few while still proving the theory.

## Reproducible Environment

- Compose platform defaults to `linux/amd64` so Windows, macOS, Linux, and ARM hosts can run the same container target.
- Base images are pinned by digest in `docker-compose.yml` and `Dockerfile`.
- Redis uses `conf/redis.conf` with fixed memory, eviction, and persistence settings.
- Go runs with `GODEBUG=randautoseed=0` and `GOMAXPROCS=4` so the randomized TTL experiment follows the same sequence.
- Python chart dependencies are pinned in `conf/python-requirements.txt`.
- Generated artifacts can be written outside tracked sample results with `RESULTS_DIR`.

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
RESULTS_DIR=./tmp-results docker compose up --build --abort-on-container-exit --exit-code-from analysis analysis
python scripts/verify_results.py ./tmp-results/results.json
docker compose down --remove-orphans
```

## Expected Stable Results

| Experiment | Scenario | Expected DB hits |
| --- | --- | ---: |
| Cache penetration | No protection | 1000 |
| Cache penetration | Empty cache, first round | 1000 |
| Cache penetration | Empty cache, second round | 0 |
| Cache penetration | Bloom filter | 0 |
| Cache breakdown | No protection | 200 |
| Cache breakdown | `singleflight` | 1 |
| Cache breakdown | Naive distributed lock | 1 |
| Cache breakdown | Safe distributed lock | 1 |
| Cache avalanche | Same TTL | 200 |
| Cache avalanche | Random TTL | about 100, accepted range 80-120 |
| Cache avalanche | Multi-level cache | 0 |

Existing files under `results/` are sample output from one run. Use `tmp-results/` for local reruns when you want to compare without touching the checked-in sample.

## Repository Layout

```text
labs/redis-cache-failure
├── main.go                    # combined experiment runner
├── analysis.py                # chart and report generation
├── conf/
│   ├── redis.conf
│   └── python-requirements.txt
├── scripts/
│   └── verify_results.py
├── results/                   # sample generated output
├── 01_penetration/            # standalone experiment notes/code
├── 02_breakdown/
└── 03_avalanche/
```

## License

MIT
