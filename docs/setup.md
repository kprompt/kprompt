# `kprompt setup`

Detect gaps via `tools.Detect` and print a bootstrap plan
([ADR-0018](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0018-kprompt-setup.md)).

```bash
kprompt setup
kprompt setup --profile minimal --approve
kprompt setup --profile platform --only prometheus --approve
kprompt setup --dry-run --json
kprompt setup --context kind-dev
```

## Flags (T-065)

| Flag | Default | Meaning |
|------|---------|---------|
| `--profile` | `platform` | `minimal` \| `platform` \| `full` |
| `--only` | (all in profile) | Comma-separated or repeatable filter: `helm`, `argo-workflows`, `prometheus`, `grafana`, `opentelemetry` (aliases: `argo`, `prom`, `otel`) |
| `--dry-run` | `true` | Print plan only |
| `--approve` | `false` | Apply host + cluster installs from the plan (never silent) |
| `--context` | config / current | Kubeconfig context for CRD / cluster checks |
| `--argo-namespace` | `argo` | Argo Workflows install namespace |
| `--prometheus-namespace` | `monitoring` | kube-prometheus-stack namespace |
| `--prometheus-release` | `kprompt-prom` | Helm release name |
| `--json` | `false` | Emit JSON plan |

`--only` must name components **inside** the selected profile (e.g. `--profile minimal --only grafana` errors).

## Profiles

| Profile | Components | Lanes |
|---------|------------|-------|
| `minimal` | Helm | host |
| `platform` (default) | Helm, Argo Workflows, Prometheus (kube-prometheus-stack) | host + cluster |
| `full` | platform + Grafana + OpenTelemetry URL steps | + config (manual `config set`; never auto-written) |

## What each needed step does on `--approve`

| Component | Lane | Apply |
|-----------|------|-------|
| Helm | host | brew / get-helm-3 (T-063) |
| Argo Workflows | cluster | `kubectl apply` install YAML → ns `argo` (T-064) |
| Prometheus | cluster | `helm install` kube-prometheus-stack → ns `monitoring` (T-064) |
| Grafana / OTel | config | **Not applied** — print `kprompt config set …` hints only |

## Namespace defaults

| Operator | Namespace | Notes |
|----------|-----------|-------|
| Argo Workflows | `argo` | Manifests pinned to release `v3.6.2` |
| kube-prometheus-stack | `monitoring` | Release name `kprompt-prom` |

After Prometheus install, set:

```bash
kprompt config set tools.prometheus.url http://kprompt-prom-kube-prometheus-stack-prometheus.monitoring.svc:9090
```

## Host install OS matrix (T-063)

| OS | Method |
|----|--------|
| macOS (darwin) | Homebrew: `brew install helm` |
| Linux | brew if present, else official get-helm-3 (`curl` required) |
| Other | Unsupported — install manually |

## Safety

- **Default is dry-run** (plan only).
- Apply needs `--approve` or interactive confirm.
- Cluster path: **plan → safety.EvaluatePlan → apply** (install-only).
- **Wipe-class denied:** `helm uninstall --all`, namespace delete, etc.
- Config-lane steps (Grafana/OTel) are never auto-written.
- Does **not** create clusters (kind/minikube/EKS) or replace Helmfile/Terraform.

## Related

- Prefer bind-over-install on existing clusters: [adopt.md](./adopt.md)
- `kprompt tools` · `kprompt doctor` · `kprompt learn`
- Product docs mirror: [kprompt.ai/docs/setup](https://kprompt.ai/docs/setup)
