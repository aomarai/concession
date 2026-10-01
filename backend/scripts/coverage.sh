#!/usr/bin/env bash
# Runs the backend tests with cross-package coverage and enforces the ratchet
# in .coverage-threshold. Usage: scripts/coverage.sh [--summary]
#
#   --summary   also write a Markdown table to $GITHUB_STEP_SUMMARY (CI)
#
# internal/testutil is test scaffolding and is excluded from the metric.
set -euo pipefail
cd "$(dirname "$0")/.."

profile="${COVERPROFILE:-coverage.out}"
pkgs=$(go list ./... | grep -v '/internal/testutil$' | paste -sd, -)

go test ./... -race -count=1 -coverpkg="$pkgs" -coverprofile="$profile"

total=$(go tool cover -func="$profile" | awk '/^total:/ {gsub("%","",$3); print $3}')
threshold=$(tr -d '[:space:]' < .coverage-threshold)

echo
echo "Total coverage: ${total}% (minimum: ${threshold}%)"
echo "Functions below 100%:"
go tool cover -func="$profile" | awk '$3+0 < 100 && $1 != "total:" {print "  " $0}'

if [[ "${1:-}" == "--summary" && -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  {
    echo "## Backend test coverage"
    echo
    echo "**Total: ${total}%** (minimum ${threshold}%)"
    echo
    echo "| Function | Coverage |"
    echo "|---|---|"
    go tool cover -func="$profile" | awk '$3+0 < 100 && $1 != "total:" {printf "| `%s` %s | %s |\n", $1, $2, $3}'
  } >> "$GITHUB_STEP_SUMMARY"
fi

if awk -v t="$total" -v m="$threshold" 'BEGIN {exit !(t+0 < m+0)}'; then
  echo "FAIL: coverage ${total}% is below the minimum ${threshold}%" >&2
  exit 1
fi
