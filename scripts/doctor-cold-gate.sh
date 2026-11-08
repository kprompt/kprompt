#!/usr/bin/env bash
# Cold doctor JSON contract: empty HOME + no kube → required llm/kubernetes fail,
# overall exit 1, never invent a healthy cluster. No LLM key required.
# Docs: docs/assurance.md
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
KPROMPT="${KPROMPT:-$ROOT/bin/kprompt}"
WORKDIR="${WORKDIR:-$(mktemp -d "${TMPDIR:-/tmp}/kprompt-doctor-cold.XXXXXX")}"
mkdir -p "$WORKDIR/home"

if [[ ! -x "$KPROMPT" ]]; then
  echo "==> build $KPROMPT"
  (cd "$ROOT" && go build -o "$KPROMPT" ./cmd/kprompt)
fi

command -v jq >/dev/null 2>&1 || {
  echo "MISS jq — install jq to run doctor cold gate" >&2
  exit 1
}

export HOME="$WORKDIR/home"
export KPROMPT_HOME="${KPROMPT_HOME:-$HOME/.kprompt}"
export KUBECONFIG=/var/empty/kprompt-no-kube
mkdir -p "$KPROMPT_HOME"

json="$WORKDIR/doctor.json"
err="$WORKDIR/doctor.stderr"

echo "==> doctor --json (cold HOME, no kube)"
set +e
"$KPROMPT" doctor --json >"$json" 2>"$err"
status=$?
set -e

if [[ "$status" -eq 0 ]]; then
  echo "FAIL: cold doctor must exit non-zero" >&2
  cat "$json" >&2 || true
  exit 1
fi
if [[ ! -s "$json" ]]; then
  echo "FAIL: empty doctor JSON (exit=$status)" >&2
  cat "$err" >&2 || true
  exit 1
fi

jq -e '(.checks | type) == "array" and (.checks | length) > 0' "$json" >/dev/null
jq -e '[.checks[] | select(.id == "llm" and .required == true)] | length == 1' "$json" >/dev/null
jq -e '[.checks[] | select(.id == "llm")][0].status == "fail"' "$json" >/dev/null
jq -e '[.checks[] | select(.id == "kubernetes" and .required == true)] | length == 1' "$json" >/dev/null
jq -e '[.checks[] | select(.id == "kubernetes")][0].status == "fail"' "$json" >/dev/null
# No required check may falsely pass kubernetes when cold.
jq -e '[.checks[] | select(.required == true and .id == "kubernetes")][0].status != "pass"' "$json" >/dev/null

jq -c '{
  llm: [.checks[] | select(.id=="llm")][0].status,
  kubernetes: [.checks[] | select(.id=="kubernetes")][0].status,
  required_fails: [.checks[] | select(.required==true and .status=="fail") | .id]
}' "$json"

echo "PASS doctor cold JSON gate"
echo "artifacts: $WORKDIR"
