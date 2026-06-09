#!/usr/bin/env bash
set -Eeuo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

RESULTS_DIR="${RESULTS_DIR:-./results}"
COMPOSE_CMD="${COMPOSE_CMD:-docker compose}"
read -r -a COMPOSE <<< "$COMPOSE_CMD"

cleanup() {
  local status=$?
  if [[ "${KEEP_CONTAINERS:-0}" != "1" ]]; then
    "${COMPOSE[@]}" down --remove-orphans >/dev/null 2>&1 || true
  fi
  exit "$status"
}
trap cleanup EXIT

mkdir -p "$RESULTS_DIR"

printf '%s\n' "=========================================="
printf '%s\n' "  Redis big key and hot key lab run"
printf '%s\n' "=========================================="
printf 'Results directory: %s\n\n' "$RESULTS_DIR"

printf '%s\n' "[1/4] Cleaning previous compose containers..."
"${COMPOSE[@]}" down --remove-orphans >/dev/null 2>&1 || true

printf '\n%s\n' "[2/4] Building and running experiment..."
RESULTS_DIR="$RESULTS_DIR" "${COMPOSE[@]}" up --build \
  --abort-on-container-exit \
  --exit-code-from experiment \
  experiment

printf '\n%s\n' "[3/4] Generating analysis artifacts..."
RESULTS_DIR="$RESULTS_DIR" "${COMPOSE[@]}" up --build --no-deps \
  --abort-on-container-exit \
  --exit-code-from analysis \
  analysis

printf '\n%s\n' "[4/4] Verifying theory invariants..."
RESULTS_DIR="$RESULTS_DIR" "${COMPOSE[@]}" run --rm --no-deps \
  analysis python /app/scripts/verify_results.py /data/results.json

printf '\n%s\n' "=========================================="
printf '%s\n' "  Done. Generated files:"
printf '  - %s/results.json\n' "$RESULTS_DIR"
printf '  - %s/result.md\n' "$RESULTS_DIR"
printf '  - %s/report.md\n' "$RESULTS_DIR"
printf '  - %s/big-key-memory.png\n' "$RESULTS_DIR"
printf '  - %s/hot-key-distribution.png\n' "$RESULTS_DIR"
printf '  - %s/summary.png\n' "$RESULTS_DIR"
printf '%s\n' "=========================================="
