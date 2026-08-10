# kprompt tools

`kprompt tools` shows which integrations `kprompt` can use from your machine
and current cluster.

It probes local binaries, configured HTTP backends, and Kubernetes CRDs. The
command is **read-only** and does not call an LLM.

When a tool is missing, a **Next steps (unavailable)** section lists install or
config hints: **`kprompt setup`** for components that setup can plan (Helm, Argo
Workflows, Prometheus) or config-lane URL steps (Grafana / OTel), plus
copy-pasteable defaults for operators setup does not install (GitOps, Tekton,
KEDA, …). On an existing cluster, prefer bind-over-install — see
[adopt.md](./adopt.md) and [setup.md](./setup.md).

## What it detects

The current CLI reports tools such as:

- Kubernetes
- Helm
- Argo Workflows
- Tekton
- KEDA
- Istio
- Linkerd
- Gateway API
- cert-manager
- Crossplane
- GitOps
- Prometheus
- Grafana
- OpenTelemetry

## Examples

```bash
kprompt tools
kprompt tools --json
kprompt tools --context staging
```

## Closing gaps with setup

```bash
# Dry-run plan for the default platform profile
kprompt setup

# Host Helm only
kprompt setup --profile minimal --approve

# Prometheus stack only (within platform)
kprompt setup --profile platform --only prometheus --approve
```

Honest limits: setup does **not** install Tekton/KEDA/Istio/Crossplane/GitOps,
does **not** create clusters, and does **not** auto-write Grafana/OTel config
(those are config-lane hints only).

## Output

Default output is a table:

- `TOOL`
- `STATUS`
- `DETAIL`

Then, when any tool is `unavailable` with a hint:

- **Next steps (unavailable)** — one line per missing tool (`Name: hint`)
- **Try: kprompt setup** — only if a setup-backed gap exists (Helm / Argo Workflows / Prometheus / Grafana / OTel)
- URL/env footer — only if Prometheus / Grafana / OTel is missing

JSON includes a `hint` field per tool:

```bash
kprompt tools --json
```

## Context override

Cluster and CRD checks use the active context by default. To inspect another
context:

```bash
kprompt tools --context prod
```

## URL and config knobs

Some integrations are enabled by URL/config rather than a local binary alone.

Common environment variables:

- `KPROMPT_PROMETHEUS_URL`
- `KPROMPT_GRAFANA_URL`
- `KPROMPT_GRAFANA_API_KEY`
- `KPROMPT_OTEL_ENDPOINT`
- `KPROMPT_OTEL_BACKEND`

Matching config keys:

- `tools.prometheus.url`
- `tools.grafana.url`
- `tools.otel.endpoint`
- `tools.otel.backend`

Example:

```bash
kprompt config set tools.prometheus.url http://prometheus.monitoring:9090
kprompt config set tools.otel.backend tempo
kprompt tools
```

If a tool is disabled or not configured, `kprompt tools` reports that in the
`DETAIL` column and lists actionable install/config guidance under **Next steps**.
