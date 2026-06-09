# Validation Record

Last verified: 2026-06-09T21:28:32+08:00

## Commands Run

```powershell
docker compose config --quiet
.\run.ps1 -ResultsDir .\tmp-results
.\run.ps1
docker run --rm -v "${PWD}:/repo" -w /repo docker.io/library/bash:5.2 bash -n ./scripts/run-lab.sh
docker run --rm -v "${PWD}:/repo" -w /repo docker.io/library/bash:5.2 bash ./scripts/run-lab.sh --list
.\scripts\run-lab.ps1 -List
.\scripts\run-lab.ps1 redis-big-hot-key -ResultsDir .\tmp-results\entrypoint-big-hot
```

## Results

| Check | Evidence |
| --- | --- |
| Docker Compose config | Passed |
| Docker startup | `redis` became healthy before `experiment` started |
| Experiment reproduction | `.\run.ps1 -ResultsDir .\tmp-results` completed with exit code 0 |
| Tracked sample generation | `.\run.ps1` completed with exit code 0 and wrote `results/` |
| Root launcher | `.\scripts\run-lab.ps1 redis-big-hot-key` completed with exit code 0 |
| macOS/Linux launcher syntax | `scripts/run-lab.sh` passed `bash -n` in a Linux bash container |
| Analysis artifacts | `results.json`, `result.md`, `report.md`, `big-key-memory.png`, `hot-key-distribution.png`, and `summary.png` generated |
| Theory verifier | `OK: /data/results.json satisfies Redis big key and hot key lab invariants.` |

## Sample Data From Tracked Results

| Scenario | Stable measurement | Conclusion |
| --- | ---: | --- |
| Normal strings | 20,000 keys, average memory 224 bytes | Baseline keys are small |
| Big string | 8,388,608-byte logical payload, 10,485,816 Redis memory bytes | Single string is far larger than normal keys |
| Big hash | 50,000 fields, 6,786,544 Redis memory bytes | Full-key operations produce large per-command work |
| Big list | 50,000 items, 3,679,800 Redis memory bytes | List cardinality creates big-key pressure |
| Split hash | 100 buckets, largest bucket 500 fields | Splitting preserves data while reducing max per-key load |
| Uniform access | Top-key ratio 0.00023 across 100,000 reads | No dominant key in baseline traffic |
| Single hot key | `item:0000` received 60,000 of 100,000 reads | Hot key traffic skew is explicit |
| Sharded hot key | Hottest copy ratio 0.0375, max copy gets 3,750 | Read fanout reduces pressure on one physical key |

## Expected Artifacts

The runner should generate the following files in the selected results directory:

```text
results.json
result.md
report.md
big-key-memory.png
hot-key-distribution.png
summary.png
```

Durations are recorded in JSON and reports but are not treated as exact pass/fail values because they depend on host CPU and Docker scheduling.
