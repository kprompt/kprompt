# Learn / cluster tool profile

`learn` detects which cluster integrations are present and saves a **local**
profile (S-009 · T-087). It never mutates the cluster.

```bash
kprompt learn
kprompt learn --context kind-dev
kprompt learn --show
kprompt learn --json

# Natural language (same path)
kprompt "learn cluster tools"
kprompt "detect tools"
```

`kprompt learn --show` is read-only: it shows the saved profile without re-detecting or rewriting it.

Profile path: `~/.kprompt/profiles/<context>.json` (or `$KPROMPT_HOME/profiles/`).

## What it detects

Extends `kprompt tools` / `tools.Detect`:

| Tool | How |
|------|-----|
| Helm | `helm` on PATH |
| Linkerd | `policy.linkerd.io/Server` CRD |
| Prometheus / Grafana / OTel | configured URLs |
| Gateway API | `gateway.networking.k8s.io/Gateway` CRD |
| cert-manager | `cert-manager.io/Certificate` CRD |
| GitOps | Flux Kustomization and/or Argo CD Application CRDs |
| Istio / KEDA / Tekton / Crossplane / Argo Workflows | existing detectors |

## Downstream use

- **`kprompt doctor`** shows a “Cluster tool profile” row (skip until first learn).
- Later **intent extraction** injects a short stack bias (prefer Gateway over Ingress
  when Gateway API is available, GitOps for sync/health, Helm for charts, Prometheus
  for latency) — never invents APIs that were not detected.

## Honest limits

- Profile is evidence of CRDs / PATH / config URLs, not proof the controllers are
  healthy or that operators use them.
- Architecture narrative (`explain architecture`, S-012) is a separate deferred task.
- Re-run `kprompt learn` after installing controllers.
