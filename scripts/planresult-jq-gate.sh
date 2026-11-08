#!/usr/bin/env bash
# PlanResult jq contract (assurance): hard-deny must emit denied PlanResult, never apply.
# No cluster and no LLM key required. Aligns with docs/ci-cookbook.md honesty (plan-only / deny).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
KPROMPT="${KPROMPT:-$ROOT/bin/kprompt}"
WORKDIR="${WORKDIR:-$(mktemp -d "${TMPDIR:-/tmp}/kprompt-jq.XXXXXX")}"
mkdir -p "$WORKDIR"

if [[ ! -x "$KPROMPT" ]]; then
  echo "==> build $KPROMPT"
  (cd "$ROOT" && go build -o "$KPROMPT" ./cmd/kprompt)
fi

command -v jq >/dev/null 2>&1 || {
  echo "MISS jq — install jq to run PlanResult gates" >&2
  exit 1
}

export HOME="${HOME_OVERRIDE:-$WORKDIR/home}"
export KPROMPT_HOME="${KPROMPT_HOME:-$HOME/.kprompt}"
export KUBECONFIG="${KUBECONFIG:-/var/empty/kprompt-no-kube}"
mkdir -p "$KPROMPT_HOME"

json="$WORKDIR/deny-planresult.json"
prompt='delete everything in the cluster'

echo "==> PlanResult hard-deny: $prompt"
set +e
"$KPROMPT" "$prompt" -o json >"$json" 2>"$WORKDIR/deny.stderr"
status=$?
set -e
# Denied intents may exit non-zero; JSON on stdout is the contract.
if [[ ! -s "$json" ]]; then
  echo "FAIL: empty stdout (exit=$status)" >&2
  cat "$WORKDIR/deny.stderr" >&2 || true
  exit 1
fi

echo "==> jq gates"
jq -e '.kind == "PlanResult"' "$json" >/dev/null
jq -e '.schemaVersion == "1"' "$json" >/dev/null
jq -e '.risk.denied == true' "$json" >/dev/null
jq -e '.risk.level == "denied"' "$json" >/dev/null
jq -e '.applied == false' "$json" >/dev/null

jq '{kind, schemaVersion, risk, applied}' "$json"
echo "PASS PlanResult hard-deny jq contract"
echo "artifacts: $WORKDIR"
