#!/usr/bin/env bash
# benchmark.sh — Run all Go benchmarks and save formatted results.
# Usage: ./scripts/benchmark.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUTPUT_DIR="${REPO_ROOT}/benchmarks"
OUTPUT_FILE="${OUTPUT_DIR}/latest.txt"

mkdir -p "${OUTPUT_DIR}"

BENCH_PKGS=(
  ./llm/providers/openaicompat/
  ./llm/capabilities/tools/
  ./agent/memorycore/
)

echo "=== AgentFlow Benchmark Suite ==="
echo "Date : $(date -u '+%Y-%m-%d %H:%M:%S UTC')"
echo "Go   : $(go version)"
echo "================================="
echo ""

cd "${REPO_ROOT}"

go test -bench=. -benchmem -count=3 -timeout 120s "${BENCH_PKGS[@]}" | tee "${OUTPUT_FILE}"

echo ""
echo "Results saved to: ${OUTPUT_FILE}"

# If benchstat is available and a baseline exists, show comparison
if command -v benchstat &>/dev/null; then
  baseline=$(ls -t "${OUTPUT_DIR}"/baseline_*.txt 2>/dev/null | head -1)
  if [[ -n "${baseline}" ]]; then
    echo ""
    echo "=== Comparison vs ${baseline} ==="
    benchstat "${baseline}" "${OUTPUT_FILE}"
  fi
fi
