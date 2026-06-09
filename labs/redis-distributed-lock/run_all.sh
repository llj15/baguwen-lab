#!/bin/sh
set -eu

OUTPUT_DIR="${OUTPUT_DIR:-/data}"
mkdir -p "$OUTPUT_DIR"

echo "=========================================="
echo "  Redis distributed lock experiments"
echo "=========================================="
echo "Output directory: $OUTPUT_DIR"

echo ""
echo "=========================================="
echo " Experiment 1: Basic Distributed Lock"
echo "=========================================="
/bin/basic_lock 2>&1 | tee "$OUTPUT_DIR/01_basic_lock.log"

echo ""
echo "=========================================="
echo " Experiment 2: Redlock Algorithm"
echo "=========================================="
/bin/redlock 2>&1 | tee "$OUTPUT_DIR/02_redlock.log"

echo ""
echo "=========================================="
echo " Experiment 3: Watchdog Auto-Renewal"
echo "=========================================="
/bin/watchdog 2>&1 | tee "$OUTPUT_DIR/03_watchdog.log"

echo ""
echo "All experiments completed."
