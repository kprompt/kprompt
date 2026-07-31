# Simulation (MVP)

**Change preview** before apply — not a sandbox cluster or failure-injection lab.

| Surface | How | Role |
|---------|-----|------|
| Plan dry-run | Default: print PlanResult, mutate only after `y` / `--approve` | Review the change |
| Blast radius | `blastRadius` on mutating plans (T-069) | “What would this touch?” |
| Impact / who consumes | `kprompt "who consumes …"` (S-005) | Live reverse deps |
| Helm dry-run | Chart template preview before install/upgrade (T-027) | Rendered YAML intent |
| Setup dry-run | `kprompt setup` (default) | Host/cluster install plan |

```bash
# Mutating plans always preview first (Simulation MVP core)
kprompt "scale api to 10" -n payments
# → PlanResult + blastRadius; nothing applied until y/--approve

kprompt "scale api to 10" -n payments -o json | jq '.blastRadius'

# Reverse deps (what depends on this live object)
kprompt "who consumes redis" -n payments
kprompt "impact of deployment checkout" -n payments

# Helm render path (install/upgrade)
kprompt "install redis" -n demo
```

## What this is

A named **review path** that composes shipped trust aids:

1. Typed plan (actions, risk, diffs when available)
2. Blast-radius namespaces / related objects
3. Optional impact walk for “who points at X”
4. Helm template dry-run when the plan is chart-shaped

Still **plan → safety → approve → apply**. Simulation never mutates by itself.

## What this is not

- Ephemeral what-if sandbox / shadow cluster
- Full mesh/OTel blast radius when signals are missing
- Failure injection, chaos, or capacity forecasting product
- Durable Coordinator blast-radius knowledge graph beyond the handoff ring (Shared Knowledge persists edges via file/ConfigMap — [coordinator-knowledge.md](./coordinator-knowledge.md); full continuous product graph still building)
- Auto-remediation from a simulated outcome

## Related

- Impact: [docs/impact.md](./impact.md)
- Knowledge Graph MVP: [docs/graph.md](./graph.md)
- Safety / approval: [docs/ci.md](./ci.md) · ADR-0003
- Helm preview / install: [docs/integration-matrix.md](./integration-matrix.md)
