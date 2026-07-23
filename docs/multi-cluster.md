# Multi-cluster (kubeconfig contexts)

kprompt’s multi-cluster story is **laptop-local**: aliases and fan-out over your kubeconfig contexts. It is **not** a hosted fleet SaaS, not Lens/Headlamp on `app.kprompt.ai`, and **never** uploads kubeconfig or cluster credentials to the control plane.

Architecture: [ADR-0012](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0012-multi-cluster.md).

## Inventory

```bash
kprompt contexts              # contexts + local aliases
kprompt contexts --check      # probe API reachability
kprompt contexts --json
```

## Aliases

```bash
kprompt config alias set prod gke_myproj_us-central1_prod
kprompt config alias set staging kind-staging
kprompt --context prod "list deployments"
kprompt config set require_alias_match true   # refuse mutate if kubectl current-context ≠ target
```

## Read fan-out

Explicit only — never “all contexts” by default.

```bash
kprompt --contexts staging,prod "list deployments"
kprompt "list pods across staging and prod"
kprompt --contexts staging,prod "optimize my cluster"
```

Supported today: get/list, explain, logs, describe, optimize. Unreachable contexts degrade; others still return.

JSON kind: `MultiContextResult` with per-context `steps`, `cluster_context` on each step, and `fleetSummary` for optimize.

## Mutate safety

| Mode | Behavior |
|------|----------|
| Interactive | Confirm **each** context (`Apply … to context "…"?`) |
| `--approve` alone | **Refused** across multiple contexts |
| `--approve-each-context` | Explicit consent to apply the same plan to every listed context |

```bash
kprompt --contexts staging,prod "scale api to 3"
kprompt --contexts staging,prod --approve-each-context "scale api to 3"
```

PlanResult / actions include `cluster_context` for audit and CI.

## What this is not

- Uploading kubeconfigs to `api.kprompt.ai` / `app.kprompt.ai`
- A hosted live multi-cluster browser (Lens/Headlamp clone)
- Silent `--approve` across every context
- Always-on in-cluster multi-cluster agent

Org **metadata** registry (display name, alias, which enrolled device can reach a cluster) is Tier 3 / deferred — still without credential upload.
