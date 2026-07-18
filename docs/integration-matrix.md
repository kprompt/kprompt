# Integration E2E matrix (T-046)

Deterministic coverage index for shipped integration families. All tests use a **stub LLM** (no live model). Kind-backed rows skip when `kind`/`helm`/Argo CRD fixtures are missing.

```bash
go test -tags=e2e ./test/e2e/ -run TestIntegrationMatrix -count=1 -v -timeout 15m
```

| Family | Test | Fixture | Asserts |
|--------|------|---------|---------|
| Kubernetes | `kubernetes_get` | kind | list deployments table |
| Helm | `helm_install_plan` | kind + `helm` on PATH | install plan, no apply without approve |
| Argo Workflows | `argo_workflow_plan` | kind + Workflow CRD | workflow plan (skips if CRD absent) |
| Prometheus | `prometheus_performance` | stub Querier | performance report |
| OpenTelemetry | `otel_trace_walk` | stub Querier | trace + bottleneck narration |
| Grafana | `grafana_dashboard` | stub Querier | dashboard summary |
| Generic K8s read | `generic_kubernetes_read` | kind + discovery | list nodes |

Related: deeper generic-read coverage in `TestGenericReadMatrixOnKind` ([kubernetes-reads.md](./kubernetes-reads.md)); unit/pipeline stubs in `internal/pipeline`.
