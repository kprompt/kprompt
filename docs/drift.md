# Drift / cluster vs Git

`drift` compares live cluster state to GitOps desired state (S-008 · T-086):

```bash
kprompt "check cluster drift"
kprompt "what is out of sync" -n flux-system
kprompt "show drift vs git"
kprompt "check flux drift" -n flux-system --output json
```

It reads Flux `Kustomization` and/or Argo CD `Application` sync + health via the
same path as `gitops` status (T-043) and emits an ADR-0014 `Investigation`.
The scan itself never mutates.

When apps are out of sync, `drift` may offer **reviewable GitOps sync plans**
(TTY `y/N` or `--approve`) — the same gate as `gitops sync`. Nothing applies
silently, and `-o json` stays report-only.

## MVP findings

| Code | What it flags |
|------|----------------|
| `Drift.OutOfSync` | Flux Ready=False / Argo `sync.status=OutOfSync` on an app |
| `Drift.Unhealthy` | Synced app with non-Healthy health (guidance; not auto-synced) |
| `Drift.ResourceOutOfSync` | Argo `status.resources[]` not Synced, or Flux `status.inventory.entries` while the Kustomization is OutOfSync (capped) |
| `Drift.GitOpsMissing` | Neither Flux nor Argo CD CRDs detected (honest degrade) |

```bash
kprompt "check drift" --output json | jq '.result'
```

## Sync plans (approve-required)

For each `Drift.OutOfSync` app, suggest may offer a single-app Flux reconcile or
Argo CD sync plan. That is live reconcile toward Git — **not** a PR.

Opening or updating a GitHub PR instead of cluster mutate is **`--gitops`**
([docs/gitops-pr.md](./gitops-pr.md) · T-072). Drift documents that path as
guidance when you want Git review before reconcile.

## Honest limits

- Drift is GitOps controller truth (sync/health inventory), not a full
  live-vs-manifest deep diff of every object field.
- **Argo:** per-resource rows come from `status.resources` (non-Synced only).
- **Flux Kustomization:** when OutOfSync, expands `status.inventory.entries` (managed set) and labels them OutOfSync — Flux has no per-resource sync bit. Missing inventory → `degraded: flux-inventory`.
- **Flux HelmRelease:** no Kustomization-style inventory API — app-level OutOfSync/Unhealthy only (degrade if expanded).
- Manual changes that GitOps has already overwritten will not appear as drift.
- Prefer `kprompt "gitops status"` for a compact table without Investigation codes.
