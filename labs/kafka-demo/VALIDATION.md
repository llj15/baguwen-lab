# Validation Record

Last verified: 2026-06-12T00:24:47+08:00

## Commands Run

```powershell
docker compose config
.\run.ps1 -ResultsDir .\tmp-results
.\run.ps1
docker run --rm -v "${PWD}:/repo" -w /repo docker.io/library/bash:5.2 bash -n ./scripts/run-lab.sh
docker run --rm -v "${PWD}:/repo" -w /repo docker.io/library/bash:5.2 bash ./scripts/run-lab.sh --list
.\scripts\run-lab.ps1 -List
.\scripts\run-lab.ps1 kafka-demo -ResultsDir .\tmp-results\entrypoint-kafka
python labs\kafka-demo\scripts\verify_results.py labs\kafka-demo\results\results.json
```

## Results

| Check | Evidence |
| --- | --- |
| Docker Compose config | `docker compose config` completed with exit code 0 |
| Docker startup | `kafka` became healthy before `experiment` started |
| Experiment reproduction | `.\run.ps1 -ResultsDir .\tmp-results` completed with exit code 0 |
| Tracked sample generation | `.\run.ps1` completed with exit code 0 and wrote `results/` |
| Root launcher | `.\scripts\run-lab.ps1 kafka-demo -ResultsDir .\tmp-results\entrypoint-kafka` completed with exit code 0 |
| macOS/Linux launcher syntax | `scripts/run-lab.sh` passed `bash -n` in a Linux bash container |
| Root lab list | Bash and PowerShell `--list`/`-List` include `kafka-demo` |
| Analysis artifacts | `results.json`, `result.md`, `report.md`, `partition-skew.svg`, `consumer-groups.svg`, and `lag-drain.svg` generated |
| Theory verifier | `OK: /data/results.json satisfies Kafka demo lab invariants.` |
| Tracked verifier | `OK: labs\kafka-demo\results\results.json satisfies Kafka demo lab invariants.` |

## Sample Data From Tracked Results

| Scenario | Stable measurement | Conclusion |
| --- | ---: | --- |
| Dataset | 153,619 events, 15 event types, 59,010 repos, 18,973 actors | The dataset is large enough and event-shaped |
| Checksum | MD5 `d4bd9ce833f217e95ffb3fd958138827` | The input file is the fixed GH Archive hour |
| Round-robin topic | 12 partitions used, min 12,801, max 12,802 | No-key round-robin production is balanced |
| `repo.id` keyed topic | 12 partitions used, max/min ratio 1.9860 | Repository keys preserve order with moderate real skew |
| `actor.id` keyed topic | 12 partitions used, max/min ratio 13.2042 | Actor keys preserve order but expose hot-key skew |
| Key partition checks | 0 key partition violations | Same key stayed on one partition |
| Per-key order checks | 0 per-key order violations | Kafka preserved per-key order |
| Consumer group 16 workers | 12 active consumers for 12 partitions | Consumer group parallelism is capped by partition count |
| Lag drain | Initial 153,619, checkpoint 148,619, final 0 | Lag drains when consumers catch up |

## Expected Artifacts

The runner should generate the following files in the selected results directory:

```text
results.json
result.md
report.md
partition-skew.svg
consumer-groups.svg
lag-drain.svg
```

Durations and throughput are recorded in JSON and reports but are not treated as exact pass/fail values because they depend on host CPU, network, and Docker scheduling.

The local Windows host does not have a usable Bash runtime (`bash --version` enters WSL and fails because `/bin/bash` is missing), so the macOS/Linux launcher was validated in the same Linux `bash:5.2` container pattern used by the repository's existing validation records.
