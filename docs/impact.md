# Impact / reverse dependencies

`impact` is a read-only reverse dependency walk (S-005 · T-083):

```bash
kprompt "who consumes redis" -n payments
kprompt "impact of service api" -n production
kprompt "what depends on deployment checkout" -n shop
kprompt "blast radius for payment-api" -n payments --output json
```

It complements the mutation-time `BlastRadius` on `PlanResult`:

- **Plan blast radius** asks: “What will this proposed change touch?”
- **Impact** asks: “What currently points at or depends on this live Service/Deployment?”

## MVP signals

For a **Service**, kprompt reports:

- Deployments whose container env / command / args statically reference the Service
- backend Deployments selected by the Service
- Ingress objects that route to the Service

For a **Deployment**, kprompt reports:

- Services that select its pod template
- Deployments that statically reference those Services
- HPAs that scale it
- PodDisruptionBudgets that protect its pods

The result uses the versioned `Investigation` JSON contract:

```bash
kprompt "who consumes redis" -n payments --output json | jq '.result'
```

## Honest limits

Kubernetes does not record “who called this Service” by itself. The MVP only
claims relationships it can infer from Kubernetes objects. It does **not** read
Secret values and it does not claim complete runtime coverage.

`degraded` includes:

- `otel` — runtime caller edges are unavailable in the static walk
- `mesh` — service-mesh traffic edges are not walked yet

No findings means “no static references found,” not “nobody calls this.”
`impact` never mutates and never asks for approval.

See also: [Knowledge Graph MVP](./graph.md) · [Simulation MVP](./simulation.md) (plan preview + blastRadius).
