# Cleanup / unused resources

`cleanup` scans for unused or stale resources (S-007 · T-085):

```bash
kprompt "cleanup payments namespace" -n payments
kprompt "find unused configmaps and secrets" -n production
kprompt "prune my cluster"
kprompt "cleanup" -n shop --output json
```

It reports candidates as an ADR-0014 `Investigation`. The scan itself never
mutates. When stale Jobs / ReplicaSets are found, `cleanup` may offer a
**reviewable delete plan** (TTY `y/N` or `--approve`). ConfigMap/Secret orphans
stay guidance-only unless the prompt explicitly confirms orphans (see below).
Nothing applies silently, and `-o json` stays report-only.

## MVP candidates

| Code | What it flags | Follow-up |
|------|----------------|-----------|
| `Cleanup.UnusedConfigMap` | ConfigMap not referenced by any Pod, workload template, or ServiceAccount | Guidance, or approve-gated delete when orphans are confirmed |
| `Cleanup.UnusedSecret` | Secret not referenced by env/envFrom/volumes/imagePullSecrets/ServiceAccount | Guidance, or approve-gated delete when orphans are confirmed |
| `Cleanup.UnusedPVC` | PersistentVolumeClaim is not in the Bound phase | Guidance-only |
| `Cleanup.EmptyService` | Service has a selector but zero active Endpoint addresses | Guidance-only |
| `Cleanup.CompletedJob` | Job finished more than 24h ago (no `ttlSecondsAfterFinished`) | Approve-gated delete |
| `Cleanup.OldReplicaSet` | Deployment-owned ReplicaSet scaled to zero (superseded revision) | Approve-gated delete |

References scanned include container `env` (`configMapKeyRef` / `secretKeyRef`),
`envFrom`, volumes (`configMap`, `secret`, projected sources), `imagePullSecrets`,
and ServiceAccount `secrets` / `imagePullSecrets`.

```bash
kprompt "cleanup payments" -n payments --output json | jq '.result'
```

## Delete plans (approve-required)

`cleanup` may offer **separate** delete plans:

1. **Jobs / ReplicaSets** — named stale Jobs and superseded ReplicaSets already
   flagged by the scan. TTY `y/N` or `--approve`.
2. **ConfigMap / Secret orphans** — only when the prompt includes a confirm
   phrase such as `confirm orphans`, `confirm orphan`, `delete orphans`,
   `delete unused configmaps`, or `delete unused secrets`. That plan sets
   `Intent.Params.confirm_orphans=true`. Safety hard-denies ConfigMap/Secret
   deletes without that param.

```bash
kprompt "cleanup payments and confirm orphans" -n payments
# Interactive: type DELETE-ORPHANS to apply the orphan plan
# Non-TTY: --approve is enough when the confirm phrase is already in the prompt
```

Orphan detection can miss CRD/GitOps references — review carefully. Safety still
hard-denies Namespace / wipe-class deletes and unscoped names.

## Honest limits

Service-account tokens, `kube-root-ca.crt`, and docker-config Secrets are
skipped. The MVP does **not** inspect CRD owners, annotations from GitOps
controllers, or cross-namespace references — review candidates before deleting.
A ConfigMap/Secret consumed only by a tool outside the scanned kinds may show
as unused.
