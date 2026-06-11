# Validation Record

Last verified: pending local Docker run.

## Commands Run

```powershell
docker compose config
.\run.ps1 -ResultsDir .\tmp-results
.\run.ps1
docker run --rm -v "${PWD}:/repo" -w /repo docker.io/library/bash:5.2 bash -n ./scripts/run-lab.sh
docker run --rm -v "${PWD}:/repo" -w /repo docker.io/library/bash:5.2 bash ./scripts/run-lab.sh --list
.\scripts\run-lab.ps1 -List
.\scripts\run-lab.ps1 kafka-demo -ResultsDir .\tmp-results\entrypoint-kafka
```

## Results

| Check | Evidence |
| --- | --- |
| Docker Compose config | Pending |
| Docker startup | Pending |
| Experiment reproduction | Pending |
| Tracked sample generation | Pending |
| Root launcher | Pending |
| macOS/Linux launcher syntax | Pending |
| Analysis artifacts | Pending |
| Theory verifier | Pending |

## Sample Data From Tracked Results

Pending tracked sample generation.

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
