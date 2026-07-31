# Knowledge Graph (MVP)

Read-only relationships across Services, consumers, and remembered deps —
**not** a continuous full-cluster topology product.

Contracts: service graph (T-059 · T-060) · impact (S-005 · T-083) · namespace memory deps (AG-015).

## What ships today

| Surface | Prompt / CLI | Artifact |
|---------|--------------|----------|
| Service dependency graph | `kprompt "show service dependency graph" -n payments` | `type: service-graph` nodes/edges |
| Reverse impact | `kprompt "who consumes redis" -n payments` | `Investigation` + `degraded` |
| Namespace dep facts | `kprompt agent memory discover/list -n payments` | local / ConfigMap facts |
| Agent dump | `kprompt agent graph -n payments` | same `service-graph` JSON |

```bash
# NL path (PlanResult envelope; read-only — no approve needed)
kprompt "show service dependency graph" -n payments
kprompt "show service dependency graph" -n payments --output json | jq '.result'

kprompt "who consumes redis" -n payments
kprompt "impact of deployment orders" -n payments

# Explicit agent helper (AG-055)
kprompt agent graph -n payments
kprompt agent graph -n payments -o json
```

Helm / laptop Observe agents do **not** upload topology to `api.kprompt.ai`.

## Node & edge honesty

**Included (MVP):**

- Services, EndpointSlice-backed pods, optional NetworkPolicy allows/denies
- Optional OTel **calls** edges when a querier is configured (else noted as degraded)
- Static reverse consumers via [impact.md](./impact.md)
- Heuristic redis/postgres/… facts via Incident Memory (evidence, not proof)

**Not claimed yet (still building / exploring):**

- Always-on cluster-wide graph of Secrets, PVCs, Ingress, Kafka, external APIs as first-class product nodes
- Interactive topology UI / Team `/graph` viewer
- Complete mesh call graph without OTel
- Sandbox / chaos Simulation beyond change preview (see [simulation.md](./simulation.md) for MVP)

## Relation to other surfaces

- **Plan `BlastRadius` / Simulation MVP** — change preview before apply ([simulation.md](./simulation.md))
- **Impact** — “what currently depends on this live object?”
- **Incident Memory** — recurring signatures + dep facts; never sole RCA proof
- **Coordinator** — cross-ns handoff/probe merge; not a shared topology store yet

## Non-goals

- Auto-remediation from graph edges
- Inventing runtime callers when OTel/mesh signals are missing
- Replacing Prometheus, service mesh, or CMDB products
- Continuous full-cluster Secrets/PVC/external-API topology product (still building)
