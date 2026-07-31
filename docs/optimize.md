# Cost Intelligence (MVP)

Read-only capacity, rightsizing, and **labeled** $/carbon estimates —
**not** a Kubecost/OpenCost bill product.

| Surface | How | Status |
|---------|-----|--------|
| Optimize report | `kprompt "optimize my cluster"` | Shipped (T-052…T-057) |
| Idle / rightsizing | Prometheus usage vs requests | Shipped |
| HPA hints | Static / maxed HPA guidance | Shipped |
| Cost / carbon notes | Optional estimates on idle + lower-rightsizing (T-073) | Shipped |
| Fleet rollup | `--contexts a,b "optimize my cluster"` | Shipped (T-078) |

Deep dive on the CLI surface continues below (optimize pack).

## Optimize my cluster

```bash
kprompt "optimize my cluster"
kprompt "optimize payments namespace"
kprompt --contexts staging,prod "optimize my cluster"
kprompt "optimize my cluster" -o json | jq '.result'
```

Never mutates. Optional fix plans still need a **separate** approval —
optimize `--approve` does **not** auto-apply.

## Sections

| Section | Signal |
|---------|--------|
| Inventory | Deployments / StatefulSets, replicas, requests/limits |
| Idle | Prometheus usage ≪ request (underutilized) |
| Rightsizing | Concrete request/limit deltas from usage |
| HPA | Static-replica / maxed-HPA hints; static Deployments get an optional approve-gated HPA create plan |
| Cost / carbon notes | Optional $/gCO2e estimates on idle + rightsizing **lower** (T-073) |

## Cost / carbon notes (T-073)

When Prometheus-backed idle or rightsizing-lower findings exist **and** inventory
has request quantities, kprompt appends labeled estimates:

- Generic public-cloud list-price averages (not your bill)
- Rough carbon intensity (not region-accurate)
- Missing Prom → section skipped → **no fake costs**
- Missing requests → no estimate for that workload

Look for `costNote` in JSON and the `optimize.cost.notes` rollup finding.

## Honesty

- Estimates are **order-of-magnitude**, not invoices
- No Prom → no invented dollars
- Recommendations that mutate still go through PlanResult → safety → approve
- Not a continuous FinOps control plane or cloud-provider cost API sync

## Related

- Recipes that chain optimize: [docs/recipes.md](./recipes.md)
- Fleet fan-out: [docs/multi-cluster.md](./multi-cluster.md)
- Blog: [optimize my cluster](https://kprompt.ai/blog/optimize-my-cluster)
