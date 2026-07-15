# CI usage (`--output json`)

`kprompt` can emit a stable **PlanResult** document for gating in pipelines.

```bash
kprompt "scale api to 10" -n prod --output json
```

Stdout is a single JSON object (plus newline). Human confirmations / wait status go to stderr when JSON mode is on.

## Schema

| Field | Notes |
|-------|--------|
| `apiVersion` | always `kprompt.io/v1` |
| `kind` | always `PlanResult` |
| `schemaVersion` | `"1"` — bump only on breaking field changes |
| `plan.intent` | `scale`, `deploy`, `get`, … |
| `plan.actions` | ops without YAML manifests |
| `risk.level` | `low` / `medium` / `high` / `denied` |
| `risk.denied` | hard deny (wipe / unsafe) |
| `applied` | whether a mutation ran |
| `result` | optional payload for `get` / `explain` / `logs` / `describe` |

Manifests and API keys are never included.

## Gate on risk (example)

```bash
#!/usr/bin/env bash
set -euo pipefail
json="$(kprompt "scale api to 10" -n prod -o json)"
echo "$json" | jq -e '.risk.denied == false' >/dev/null
echo "$json" | jq -e '.plan.intent == "scale"' >/dev/null
# Optional: require human or bot to apply later
kprompt "scale api to 10" -n prod --approve --wait
```

## jq helpers

```bash
# Fail if any delete is planned without explicit allowlist
echo "$json" | jq -e '[.plan.actions[].op] | index("delete") | not'

# High-risk gate
echo "$json" | jq -e '.risk.level != "high"'
```
