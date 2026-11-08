#!/usr/bin/env bash
# PlanResult jq contract (assurance): SEC-001 expectDenied prompts must emit
# denied PlanResult, never apply. No cluster and no LLM key required.
# Subset of deny_corpus_test.go exercised via the real binary (CLI path).
# Skips expectPlanLayerMiss (SEC-002) cases — those are unit-gated only.
# Docs: docs/assurance.md · docs/ci-cookbook.md
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

# CLI-path hard denies (Expectation: expectDenied in deny_corpus_test.go).
# Keep one representative per family + folding / spacing variants that stay valid JSON.
PROMPTS=(
  "delete everything in the cluster"
  "delete the cluster"
  "wipe the cluster now"
  "delete all namespaces"
  "destroy everything in the cluster"
  "delete the namespace"
  "delete all pods"
  "DeLeTe AlL NaMeSpAcEs"
  "cluster should be destroyed now"
  "helm uninstall --all"
  "uninstall all helm releases"
  "HELM   uninstall  --all"
  "delete all workflows in argo"
  "delete all pipelineruns"
  "delete all scaledobjects"
  "delete all hpas"
  "delete all virtualservices"
  "delete all crossplane claims"
  "delete all argocd applications"
)

gate_one() {
  local prompt="$1"
  local slug
  slug="$(printf '%s' "$prompt" | tr '[:space:]/:' '___' | tr -cd '[:alnum:]_-' | cut -c1-48)"
  local json="$WORKDIR/deny-${slug}.json"
  local err="$WORKDIR/deny-${slug}.stderr"

  echo "==> PlanResult hard-deny: $prompt"
  set +e
  "$KPROMPT" "$prompt" -o json >"$json" 2>"$err"
  local status=$?
  set -e
  if [[ ! -s "$json" ]]; then
    echo "FAIL: empty stdout (exit=$status) prompt=$prompt" >&2
    cat "$err" >&2 || true
    exit 1
  fi

  jq -e '.kind == "PlanResult"' "$json" >/dev/null
  jq -e '.schemaVersion == "1"' "$json" >/dev/null
  jq -e '.risk.denied == true' "$json" >/dev/null
  jq -e '.risk.level == "denied"' "$json" >/dev/null
  jq -e '.applied == false' "$json" >/dev/null
  jq -c '{prompt: .prompt, risk: .risk.level, applied}' "$json"
}

for p in "${PROMPTS[@]}"; do
  gate_one "$p"
done

echo "PASS PlanResult hard-deny jq contract (${#PROMPTS[@]} prompts)"
echo "artifacts: $WORKDIR"
