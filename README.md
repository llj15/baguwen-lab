# Baguwen Lab

Hands-on labs for learning classic backend interview topics by running reproducible experiments instead of memorizing answers.

`baguwen` is the pinyin for the classic interview "eight-legged essay" style. The point of this monorepo is to turn each high-frequency interview topic into a small, runnable lab with:

- a focused experiment;
- containerized, reproducible setup;
- machine-checkable expected results;
- short notes that connect the experiment back to the underlying theory.

## Labs

| Lab | Topic | Status |
| --- | --- | --- |
| [Redis Cache Failure](labs/redis-cache-failure/) | Cache penetration, cache breakdown, cache avalanche | Ready |
| [Redis Distributed Lock](labs/redis-distributed-lock/) | SET NX EX, Redlock, watchdog renewal | Ready |
| [Redis Big/Hot Key](labs/redis-big-hot-key/) | Big key detection, split mitigation, hot key skew, read-copy sharding | Ready |

## Run A Lab

Each lab owns its own Docker runtime, scripts, and verification rules. Run commands from the repository root.

List available labs:

```bash
./scripts/run-lab.sh --list
```

Windows PowerShell:

```powershell
.\scripts\run-lab.ps1 -List
```

### macOS

```bash
./scripts/run-lab.sh redis-cache-failure
./scripts/run-lab.sh redis-distributed-lock
./scripts/run-lab.sh redis-big-hot-key
```

Prerequisite: Docker Desktop for Mac with Compose support.

### Linux

```bash
./scripts/run-lab.sh redis-cache-failure
./scripts/run-lab.sh redis-distributed-lock
./scripts/run-lab.sh redis-big-hot-key
```

Prerequisite: Docker Engine with the Compose plugin. If Docker requires sudo on your machine, run with your normal Docker group setup or pass a Compose wrapper:

```bash
./scripts/run-lab.sh --compose "sudo docker compose" redis-cache-failure
```

### Windows

```powershell
.\scripts\run-lab.ps1 redis-cache-failure
.\scripts\run-lab.ps1 redis-distributed-lock
.\scripts\run-lab.ps1 redis-big-hot-key
```

Prerequisite: Docker Desktop for Windows with Linux containers enabled.

By default, generated files go to `tmp-results/<lab>` at the repository root. Override that with `--results-dir` on macOS/Linux or `-ResultsDir` on Windows.

Aliases for the Redis Big/Hot Key lab: `big-hot`, `bigkey`, and `hotkey`.

## Monorepo Layout

```text
.
|-- scripts/
|   |-- run-lab.sh
|   `-- run-lab.ps1
|-- labs/
|   |-- redis-cache-failure/
|   |   |-- docker-compose.yml
|   |   |-- conf/
|   |   |-- scripts/
|   |   `-- results/
|   |-- redis-big-hot-key/
|   |   |-- docker-compose.yml
|   |   |-- conf/
|   |   |-- scripts/
|   |   `-- results/
|   `-- redis-distributed-lock/
|       |-- docker-compose.yml
|       |-- conf/
|       |-- scripts/
|       `-- results/
|-- .github/workflows/
`-- README.md
```

## License

MIT
