# Coordinator Shared Knowledge

Cross-namespace **handoff memory** on the thin Coordinator — edges from recent handoffs, optionally **restart-safe**.

| Surface | How | Role |
|---------|-----|------|
| Handoff merge | `POST /v1/handoff` → `CoordinatorReply` | Origin + optional suspect probe |
| Recent ring | `GET /v1/recent` | Last N handoffs |
| Knowledge summary | `GET /v1/knowledge` | Namespace edges + latest summaries |
| Durable store (AG-060) | `--knowledge-backend file\|configmap` | Survive Coordinator restarts |
| CLI | `kprompt agent coordinator knowledge` | Human-readable Shared Knowledge view |

```bash
# Run Coordinator with durable ConfigMap store (Helm default)
kprompt agent coordinator --addr :9090 --probe-kube \
  --knowledge-backend configmap --in-cluster --knowledge-namespace kprompt-system

# Laptop file backend
kprompt agent coordinator --addr :9090 --knowledge-backend file

# Inspect
kprompt agent coordinator knowledge --url http://127.0.0.1:9090
kprompt agent coordinator knowledge --url http://127.0.0.1:9090 --json
```

Helm (`charts/kprompt-coordinator`): `knowledge.enabled=true` (default) writes ConfigMap `kprompt-coordinator-knowledge` in the release namespace.

Kind demo: `make coordinator-e2e` in [kprompt-examples](https://github.com/kprompt/kprompt-examples) asserts `/v1/knowledge` durable + restore after restart (AG-061).

## What this is

1. Namespaces observed on handoffs
2. `from → suspect` edges with counts
3. Latest merged InvestigationReport summaries
4. `durable: true` when a Store is configured (file/ConfigMap)

Still **no Coordinator mutate** ([ADR-0017](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0017-coordinator.md)).

## What this is not

- Full continuous blast-radius / mesh product graph
- Replacement for per-namespace Incident Memory ([agent.md](./agent.md))
- Replacement for read-only Knowledge Graph MVP ([graph.md](./graph.md))
- Cluster-wide Secret/PVC topology

## Related

- Coordinator ops: [agent-ops.md](./agent-ops.md)
- Namespace Agent modes: [namespace-agent.md](./namespace-agent.md)
- Simulation / plan blast radius: [simulation.md](./simulation.md)
- Impact reverse deps: [impact.md](./impact.md)
