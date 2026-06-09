# Baguwen Lab

Hands-on labs for learning classic backend interview topics by running reproducible experiments instead of memorizing answers.

`baguwen` is the pinyin for 八股文. The point of this monorepo is to turn each high-frequency interview topic into a small, runnable lab with:

- a focused experiment;
- containerized, reproducible setup;
- machine-checkable expected results;
- short notes that connect the experiment back to the underlying theory.

## Labs

| Lab | Topic | Status |
| --- | --- | --- |
| [Redis Cache Failure](labs/redis-cache-failure/) | Cache penetration, cache breakdown, cache avalanche | Ready |

## Run A Lab

Each lab owns its own runtime, scripts, and verification rules. For the Redis cache lab:

```bash
cd labs/redis-cache-failure
RESULTS_DIR=./tmp-results bash ./run.sh
```

Windows PowerShell:

```powershell
cd labs\redis-cache-failure
.\run.ps1 -ResultsDir .\tmp-results
```

## Monorepo Layout

```text
.
├── labs/
│   └── redis-cache-failure/
│       ├── main.go
│       ├── docker-compose.yml
│       ├── conf/
│       ├── scripts/
│       └── results/
├── .github/workflows/
└── README.md
```

## License

MIT
