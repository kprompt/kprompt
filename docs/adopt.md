# Adopt on an existing cluster (brownfield)

Greenfield (kind + [`demo`](./demo.md)) is the easy path. This page is for
clusters you already run: bind what exists, read first, install last.

**Canonical checklist (website):**
[https://kprompt.ai/docs/adopt](https://kprompt.ai/docs/adopt)

**Narrative:**
[brownfield kprompt in 15 minutes](https://kprompt.ai/blog/brownfield-kprompt-in-15-minutes)

## Fast path

```bash
# 1. Binary + context (staging / sandbox first)
kprompt version
kubectl config current-context

# 2. Provider
kprompt init --ollama          # or --provider … + BYOK env
kprompt doctor

# 3. Detect, then bind (prefer config over setup install)
kprompt tools
kprompt config set tools.prometheus.url http://prometheus.monitoring:9090
# optional: tools.grafana.url / tools.otel.backend
kprompt tools

# 4. Read-only insight
kprompt "list deployments" -n <ns>
kprompt "explain why <workload> is failing" -n <ns>
kprompt "why is my api slow?" -n <ns>
kprompt "optimize my cluster"

# 5. Optional IDE — read/plan only (never applies)
# see docs/mcp.md → kprompt mcp serve

# 6. Mutation: plan on TTY; --approve only after you trust the shape
kprompt "scale <workload> to 3" -n <ns>
```

## Rules of thumb

| Prefer | Avoid |
|--------|--------|
| `config set` for Prom / Grafana / OTel URLs you already run | `kprompt setup --profile platform --approve` as day-0 on a working platform |
| Read prompts first (explain / why / optimize / graph) | First session on production with `--approve` |
| [`kprompt mcp serve`](./mcp.md) for IDE investigate / plan | Expecting MCP to apply mutations |
| [`kprompt setup`](./setup.md) only for true gaps (dry-run → approve) | Installing a second Prometheus “so the demo is green” |

## Related

- [init.md](./init.md) — provider only
- [doctor.md](./doctor.md) — health report
- [tools.md](./tools.md) — capability detect + config keys
- [setup.md](./setup.md) — approve-gated bootstrap when something is truly missing
- [mcp.md](./mcp.md) — editor interop (no apply)
- [approval.md](./approval.md) — plan → safety → approve
