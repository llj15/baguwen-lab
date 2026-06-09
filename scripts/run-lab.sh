#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/run-lab.sh --list
  scripts/run-lab.sh [options] <lab>

Labs:
  redis-cache-failure       Cache penetration, breakdown, avalanche
  redis-distributed-lock    SET NX EX, Redlock, watchdog renewal

Aliases:
  cache                     redis-cache-failure
  lock                      redis-distributed-lock
  distributed-lock          redis-distributed-lock

Options:
  --results-dir DIR         Output directory. Relative paths resolve from repo root.
  --compose CMD             Compose command, default: docker compose
  --keep-containers         Keep Compose containers after the run.
  -h, --help                Show this help.
EOF
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
repo_root="$(cd "$script_dir/.." && pwd -P)"

lab=""
results_dir=""
compose_cmd="${COMPOSE_CMD:-docker compose}"
keep_containers="${KEEP_CONTAINERS:-0}"

canonical_lab() {
  case "$1" in
    cache|redis-cache-failure)
      printf '%s\n' "redis-cache-failure"
      ;;
    lock|distributed-lock|redis-distributed-lock)
      printf '%s\n' "redis-distributed-lock"
      ;;
    *)
      return 1
      ;;
  esac
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --list)
      printf '%s\n' "redis-cache-failure"
      printf '%s\n' "redis-distributed-lock"
      exit 0
      ;;
    --results-dir)
      if [ "$#" -lt 2 ]; then
        printf '%s\n' "Missing value for --results-dir" >&2
        exit 2
      fi
      results_dir="$2"
      shift 2
      ;;
    --compose)
      if [ "$#" -lt 2 ]; then
        printf '%s\n' "Missing value for --compose" >&2
        exit 2
      fi
      compose_cmd="$2"
      shift 2
      ;;
    --keep-containers)
      keep_containers="1"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      printf 'Unknown option: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [ -n "$lab" ]; then
        printf 'Unexpected extra argument: %s\n' "$1" >&2
        usage >&2
        exit 2
      fi
      lab="$1"
      shift
      ;;
  esac
done

if [ -z "$lab" ]; then
  usage >&2
  exit 2
fi

if ! lab="$(canonical_lab "$lab")"; then
  printf 'Unknown lab: %s\n' "$lab" >&2
  usage >&2
  exit 2
fi

if [ -z "$results_dir" ]; then
  results_dir="$repo_root/tmp-results/$lab"
else
  case "$results_dir" in
    /*|[A-Za-z]:/*|[A-Za-z]:\\*)
      ;;
    *)
      results_dir="$repo_root/$results_dir"
      ;;
  esac
fi

mkdir -p "$results_dir"

printf 'Running lab: %s\n' "$lab"
printf 'Results directory: %s\n' "$results_dir"

cd "$repo_root/labs/$lab"
RESULTS_DIR="$results_dir" COMPOSE_CMD="$compose_cmd" KEEP_CONTAINERS="$keep_containers" bash ./run.sh
