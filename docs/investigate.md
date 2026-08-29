# investigate (S-002)

On-demand multi-hop RCA that emits an ADR-0014 **`Investigation`** document — not chat scroll.

**Shape of the work:** investigate is one path through the **Investigation Graph** — signal hops → findings → optional PlanResult → approve → apply → verify. See [investigation-graph.md](./investigation-graph.md) ([S-017](https://github.com/kprompt/kprompt-architecture/issues/209)).

## Usage

```bash
kprompt "investigate api" -n payments
kprompt "investigate api" -n payments -o json
```

Walks:

1. **VirtualService** (Istio destinations whose hosts match Services selecting the workload) — when a dynamic client is available
2. **Ingress** (backends whose Services select the workload) — in parallel with Explain / Service / Prom / mesh
3. **Service** (selectors matching the workload) — in **parallel** with the Explain chain (T-090)
4. **Endpoints** (ready / notReady counts) — fan-out per matching Service
5. **Deployment → ReplicaSet → Pods** (T-024 explain chain)
6. **Events ∥ Logs** on the worst pod (independent after pods are known)
7. **Prometheus** metrics (CPU / memory / restart rate) when `tools.prometheus.url` / `KPROMPT_PROMETHEUS_URL` is set and queries succeed

Root cause + confidence come from findings (CrashLoop / ImagePull / OOM / no ready endpoints). Optional suggested fix still goes through PlanResult → approve (never auto-apply).

Prefer a **loop** (this sequential walk) for one Service/workload. Prefer graph width (fan-out / Coordinator) when signals or namespaces are independent — see [investigation-graph.md](./investigation-graph.md#loop-vs-graph). Confidence and suggested fixes are still bound by [reality anchors](./reality-anchors.md) (hard deny, EvidenceRef, PlanResult — not chat vibes). **Pre-trust (T-089):** after the walk, `internal/pretrust` clamps high confidence without EvidenceRef / contradicting re-read and can withhold approve UX for suggested fixes.

**Edge audit (S-019):** hops with no data edge run concurrently (Explain ∥ Service ∥ Ingress ∥ VirtualService ∥ Prometheus; Endpoints per Service; Events ∥ Logs). True chains (Deployment → RS → Pods) stay sequential.

## Honest gaps (`degraded`)

| Signal | When listed in `degraded` |
|--------|---------------------------|
| `ingress` | Ingress API list failed (or walk skipped) — not when list succeeded with zero matches |
| `prometheus` | No URL configured, client build failed, or all PromQL queries failed — never invents metrics |
| `mesh` | No dynamic client, or VirtualService list failed (not when CRD absent / zero matches) |

Configure Prometheus:

```bash
kprompt config set tools.prometheus.url http://prometheus.monitoring.svc:9090
# or: export KPROMPT_PROMETHEUS_URL=…
```

## vs `explain` / `why`

| | `explain` | `why` | `investigate` |
|--|-----------|-------|----------------|
| Focus | Deployment → Pods → Events → Logs | Cause tree on one pod/workload | + VS / Ingress / Service / Endpoints / optional Prom ahead of that chain |
| Artifact | explain-lite JSON | `Investigation` (`kprompt.io/v1`) | `Investigation` (`kprompt.io/v1`) |
| Trigger | generic diagnosis | “why is X pending/crashing” | “investigate X” / root cause / RCA |
| Shape | short chain | **loop** (usually) | chain + independent fan-out (T-090) |

See also [docs/why.md](./why.md) · [investigation-graph.md](./investigation-graph.md).

Try against [kprompt-examples](https://github.com/kprompt/kprompt-examples):

```bash
make break SCENARIO=01-crashloop
kprompt "investigate api" -n payments
```
