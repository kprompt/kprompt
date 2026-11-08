#!/usr/bin/env bash
# Local release-assurance helper. See docs/assurance.md.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

mode="${1:-all}"

run_unit() {
  echo "==> go test ./..."
  go test ./...
}

run_jq() {
  echo "==> PlanResult hard-deny jq"
  "$ROOT/scripts/planresult-jq-gate.sh"
}

run_e2e_smoke() {
  if ! command -v kind >/dev/null 2>&1 || ! command -v kubectl >/dev/null 2>&1; then
    echo "==> skip kind e2e smoke (kind/kubectl not on PATH)"
    return 0
  fi
  if ! docker info >/dev/null 2>&1; then
    echo "==> skip kind e2e smoke (docker daemon not reachable)"
    return 0
  fi
  echo "==> kind e2e smoke (CI subset)"
  go test -tags=e2e ./test/e2e/ \
    -run 'TestGenericReadMatrixOnKind|TestDeployRedisOnKind|TestAgentWatchPodsEvents' \
    -count=1 -v -timeout 10m
}

case "$mode" in
  -h|--help|help)
    cat <<'EOF'
Usage: ./scripts/assure.sh [all|unit|jq|e2e]

  all   (default) unit + jq + kind smoke if available
  unit  go test ./...
  jq    build + PlanResult hard-deny jq gates
  e2e   kind e2e smoke only (requires kind/kubectl/docker)

Pre-release (remote Actions):
  gh workflow run ci.yml -f run_full_e2e_tests=full
  gh workflow run magic-moment.yml -f demo_seconds=28 -f run_learn_second_hit=true

Docs: docs/assurance.md
EOF
    ;;
  unit|--unit)
    run_unit
    ;;
  jq|--jq)
    run_jq
    ;;
  e2e|--e2e)
    run_e2e_smoke
    ;;
  all|"")
    run_unit
    run_jq
    run_e2e_smoke
    echo
    echo "Local assure OK. For magic-moment / full e2e see docs/assurance.md (Actions)."
    ;;
  *)
    echo "unknown mode: $mode (try --help)" >&2
    exit 2
    ;;
esac
