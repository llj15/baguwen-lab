# Validation Record

Last verified: 2026-06-09T17:08:01+08:00

## Commands Run

```powershell
docker compose config
docker compose build experiment
docker compose build analysis
.\run.ps1 -ResultsDir .\tmp-results
.\run.ps1
$env:RESULTS_DIR = '.\tmp-results'; docker compose run --rm --no-deps analysis python /app/scripts/verify_results.py /data --baseline /baseline; Remove-Item Env:RESULTS_DIR
docker compose down --remove-orphans
```

## Results

| Check | Evidence |
| --- | --- |
| Docker Compose config | Passed |
| Experiment image build | Passed |
| Analysis image build | Passed |
| Docker startup | `redis-1`, `redis-2`, and `redis-3` became healthy |
| Experiment reproduction | `.\run.ps1` completed with exit code 0 |
| Analysis artifacts | `report.md`, `01_basic_lock.png`, `02_redlock.png`, and `03_watchdog.png` generated |
| Theory verifier | `OK: /data satisfies Redis distributed lock lab invariants.` |
| Reproduction-vs-sample comparison | `tmp-results` passed comparison against tracked `results` |

## Sample Data From Tracked Results

| Experiment | Scenario | Expected | Actual | Stable conclusion |
| --- | --- | ---: | ---: | --- |
| Basic lock | `no_lock` | 1000 | 180 | Lost updates occur without locking |
| Basic lock | `set_nx_ex` | 1000 | 1000 | Correct lock keeps the counter exact |
| Basic lock | `short_ttl` | 100 | 100 | TTL is unsafe: `wrong_release=100` |
| Redlock | `redlock_3_node` | 500 | 500 | 3-node quorum keeps the counter exact |
| Redlock | `redlock_1_node_down` | 500 | 500 | 2/3 quorum tolerates one dead node |
| Watchdog | `no_watchdog` | 25 | 13 | Expired locks cause lost updates |
| Watchdog | `with_watchdog` | 25 | 25 | Renewal keeps the counter exact |
| Watchdog | `watchdog_timeline` | n/a | n/a | TTL renewed 20 times before unlock |

Durations and retry counts are recorded in JSON but are not treated as exact pass/fail values because they depend on host CPU and Docker scheduling.
