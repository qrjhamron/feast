#!/usr/bin/env bash
# run_paper_smoke.sh - Runs the full Paper 1.20.4 smoke validation suite.
#
# Usage:
#   ./test/smoke/scripts/run_paper_smoke.sh [host] [port] [username]
#
# Defaults to environment variables MC_HOST, MC_PORT, MC_USERNAME.

set -euo pipefail

HOST="${1:-${MC_HOST:-127.0.0.1}}"
PORT="${2:-${MC_PORT:-25565}}"
USER="${3:-${MC_USERNAME:-FeastGoBot}}"

export MC_HOST="$HOST"
export MC_PORT="$PORT"
export MC_USERNAME="$USER"

SMOKE="go run ./cmd/smoke"
PASS=0
FAIL=0

run_test() {
    local label="$1"
    shift
    echo "=== $label ==="
    if $SMOKE "$@" 2>&1 | tee /dev/stderr | grep -q "result=PASS"; then
        echo "  ✓ PASS"
        PASS=$((PASS+1))
    else
        echo "  ✗ FAIL"
        FAIL=$((FAIL+1))
    fi
    echo
}

run_test "World"              --smoke-world
run_test "HPA*"              --hpa-test
run_test "Break block"       --smoke-break-block
run_test "Place block"       --smoke-place-block
run_test "Invalid placement" --smoke-place-block-invalid
run_test "World mutate"      --smoke-world-mutate
run_test "HPA mutation"      --hpa-mutation-test
run_test "Inventory dump"    --inventory-dump
run_test "Survival place"    --smoke-place-survival
run_test "Entity metadata"   --entity-metadata-test
run_test "Entity hitboxes"   --entity-hitbox-test

echo "==================================================="
echo "Results: PASS=$PASS FAIL=$FAIL"
echo "==================================================="

[ "$FAIL" -eq 0 ]
