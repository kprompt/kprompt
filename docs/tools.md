# kprompt tools

`kprompt tools` shows which integrations `kprompt` can use from your machine
and current cluster.

It probes local binaries, configured HTTP backends, and Kubernetes CRDs. The
command is **read-only** and does not call an LLM.

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

## Output

Default output is a table:

- `TOOL`
- `STATUS`
- `DETAIL`

Use JSON for scripting or debugging:

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
`DETAIL` column and may include a hint.
