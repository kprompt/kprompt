# Coordinator Shared Knowledge

Cross-namespace **handoff memory** on the thin Coordinator — edges from recent handoffs, optionally **restart-safe**, with an opt-in **proactive tick** (RT-009).

| Surface | How | Role |
|---------|-----|------|
| Handoff merge | `POST /v1/handoff` → `CoordinatorReply` | Origin + optional suspect probe |
| Proactive tick | `--tick-interval` (RT-009) | Re-scan Shared Knowledge + re-probe without new handoff |
| Recent ring | `GET /v1/recent` | Last N handoffs |
| Knowledge summary | `GET /v1/knowledge` | Namespace edges + latest summaries |
| Blast-radius MVP | `GET /v1/blast-radius` | Risk-ranked hops; `status=degraded` without mesh/OTel (RT-010) |
| Cascade cap | `--max-hops` (RT-011) | BFS hop limit from focus namespace |
| Outcome ring | `POST /v1/outcome` · `GET /v1/outcomes` (RT-021) | Durable cross-ns action/result memory (TTL + cap) |
| Durable store (AG-060) | `--knowledge-backend file\|configmap` | Survive Coordinator restarts |
| CLI | `knowledge` · `blast-radius` · `outcomes` | Human-readable views |

```bash
# Run Coordinator with durable ConfigMap store (Helm default)
kprompt agent coordinator --addr :9090 --probe-kube \
  --knowledge-backend configmap --in-cluster --knowledge-namespace kprompt-system

# Continuous correlation (opt-in — not silent heal)
kprompt agent coordinator --addr :9090 --probe-kube --tick-interval 5m --tick-budget 5 --max-hops 3

# Laptop file backend
kprompt agent coordinator --addr :9090 --knowledge-backend file

# Inspect
kprompt agent coordinator knowledge --url http://127.0.0.1:9090
kprompt agent coordinator blast-radius --url http://127.0.0.1:9090
kprompt agent coordinator blast-radius --url http://127.0.0.1:9090 -n payments --json
kprompt agent coordinator outcomes --url http://127.0.0.1:9090
```

## Outcome ring (RT-021)

Cross-namespace remediation **outcomes** (action, namespace, result) persist beside
Shared Knowledge — same file/ConfigMap, separate `outcomes.json` key — with a TTL
(`--outcome-ttl`, default 30d) and size cap (`--outcome-max`, default 200). Records
survive Coordinator restarts alongside handoffs.

```bash
# Record an outcome (namespace agent / apply pipeline pushes these)
curl -sS -XPOST http://127.0.0.1:9090/v1/outcome \
  -d '{"namespace":"payments","action":"rollout-restart","result":"apply_success"}'

# Fleet summary (evidence-not-proof, for bias only)
kprompt agent coordinator outcomes --url http://127.0.0.1:9090
```

The summary aggregates per-result and per-action success/fail counts. It is
**evidence, not proof** (AG-034 / RT-022): namespace agents may read it to bias
proposal ranking, never as sole root-cause proof. The ring stores only
`action/namespace/result` (+ optional action/incident IDs) — never Secret values
or full manifests.

### Fleet read from the namespace agent (RT-022)

`kprompt agent run --coordinator-url http://<coord>:9090/v1/handoff` reuses the
same URL to `GET /v1/outcomes` and feed Autopilot a **bounded** bias:

- Nudges `ActionConfidence` only — never the raw `confidence` that drives the
  `MinConfidence` apply gate, and never creates a candidate `detectCandidates`
  didn't already produce.
- Delta capped at ±0.05, requires ≥3 fleet samples for the action, and is
  applied **only when local Learn already matched** (AG-034: fleet can nudge an
  already-locally-supported proposal, never stand alone).
- Cached for a short TTL to avoid a network call per proposal.
- The proposal records a `Fleet evidence (not proof): … — bias only (AG-034/RT-022)`
  note in `expectedImpact` / `learnNote` so audit shows fleet data was advisory.

Helm (`charts/kprompt-coordinator`): `knowledge.enabled=true` (default); set `continuous.tickInterval` (e.g. `5m`) to enable RT-009.

Kind demo: `make coordinator-e2e` in [kprompt-examples](https://github.com/kprompt/kprompt-examples) asserts `/v1/knowledge` durable + restore after restart (AG-061).

## What this is

1. Namespaces observed on handoffs
2. `from → suspect` edges with counts
3. Latest merged InvestigationReport summaries
4. `durable: true` when a Store is configured (file/ConfigMap)
5. Blast-radius MVP hops with low/medium/high risk; `status=ok|degraded` (RT-010)
6. Opt-in proactive tick refreshing edges without a new handoff POST (RT-009)
7. Audit of every merge (handoff + tick) with `mutateAttempted=false` (RT-011)
8. Durable cross-ns outcome ring (action/ns/result) with TTL + cap (RT-021)

Still **no Coordinator mutate** ([ADR-0017](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0017-coordinator.md)). **Continuous ≠ silent heal.**

## What this is not

- Silent fleet remediation / Autopilot apply from the Coordinator
- Full mesh / OTel call graph product (opt-in `--mesh-otel` only flips status honesty today)
- Replacement for per-namespace Incident Memory ([agent.md](./agent.md))
- Replacement for read-only Knowledge Graph MVP ([graph.md](./graph.md))
- Cluster-wide Secret value topology

## Related

- Coordinator ops: [agent-ops.md](./agent-ops.md)
- Namespace Agent modes: [namespace-agent.md](./namespace-agent.md)
- Simulation / plan blast radius: [simulation.md](./simulation.md)
- Impact reverse deps: [impact.md](./impact.md)
- Topology KG: [graph.md](./graph.md)
