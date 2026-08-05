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
| `result` | optional payload for `get` / `explain` / `logs` / `describe` / `optimize` |
| `cluster_context` | kubeconfig context used for this plan (also on each `plan.actions[]`) |
| `blastRadius` | optional mutate review aid: namespaces, labels/owners, related HPA/Service/NetworkPolicy (T-069) |
| `verify` | optional post-apply outcome: `ok` / `pending` / `failed` / `skipped` (T-070) |

Manifests and API keys are never included.

**Reality anchors:** `risk.denied`, schemaVersion, and `verify` are frozen gates the model cannot waive — [reality-anchors.md](./reality-anchors.md).

## Multi-context (`MultiContextResult`)

When `--contexts a,b` (or NL “across …”) fans out a **read** (or optimize):

| Field | Notes |
|-------|--------|
| `kind` | `MultiContextResult` |
| `contexts` | resolved kube context names |
| `steps` | per-context `PlanResult` (each with `cluster_context`) |
| `fleetSummary` | optimize only: ok/failed contexts + merged findings |
| `applied` | false if any step failed or was skipped |

Mutating multi-context runs still use per-context approval (or `--approve-each-context`). Plain `--approve` across multiple contexts is refused. Read fan-out covers get/list, explain, investigate, why, timeline, impact, audit, cleanup, search, score, architecture, logs, describe, and optimize. See [multi-cluster.md](./multi-cluster.md).

## Approval vocabulary

Interactive confirms use `Apply …? [y/N]`. CI / non-TTY mutates use **`--approve`** (primary flag). Multi-context mutates need `--approve-each-context`. Full table: [approval.md](./approval.md).

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

# Fail if blast radius spans more than one namespace
echo "$json" | jq -e '(.blastRadius.namespaces // []) | length <= 1'

# After --approve --wait, require verify ok
echo "$json" | jq -e '.verify.status == "ok"'
```

## Team org ingest (this repo’s Actions)

GitHub Actions on `kprompt/kprompt` can upload a **stub** PlanResult to Team after tests pass (optional job `report-plan` in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml)). This does **not** apply anything to a cluster.

| Requirement | Notes |
|-------------|--------|
| GitHub App | Install the kprompt App on the **`kprompt` org** (not only a personal account), then bind that installation in [Integrations](https://app.kprompt.ai/integrations) |
| Bound repo | Bind `kprompt/kprompt` (and pipeline metadata for `.github/workflows/ci.yml` if you want status timestamps) |
| Repo secret | `KPROMPT_ORG_TOKEN` = org API token (`kp_…` from Team after `kprompt login` / org tokens UI) |
| Optional var | `KPROMPT_API_URL` (default `https://api.kprompt.ai`) |

Without the secret, CI still passes and skips the upload. With it: Audit `reported`, app `/ci` artifact, and (when App JWT + Checks write are configured) a Check Run named `kprompt` via optional `head_sha`.

See also: [GitOps PR mode](./gitops-pr.md) (CLI laptop lane; separate from Team org connect).
