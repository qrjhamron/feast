#!/usr/bin/env bash
# run_local_validation.sh - Quick local smoke validation (unit + race tests only).
#
# Does NOT require a live server. Validates code quality only.

set -euo pipefail

echo "=== go fmt check ==="
gofmt -l . | (grep . && exit 1 || true)
echo "  ✓ formatted"

echo "=== go vet ==="
go vet ./...
echo "  ✓ clean"

echo "=== go test ./... ==="
go test ./...
echo "  ✓ all pass"

echo "=== race test (core packages) ==="
go test -race ./pkg/world ./pkg/state ./pkg/conn ./pkg/feast
echo "  ✓ no races"

echo "=== examples compile ==="
go test ./examples/...
echo "  ✓ compile ok"

echo
echo "Local validation PASS"
