# Observe / Namespace Agent ops runbook

Operator-facing notes for cost, RBAC, and day-2 ops ([AG-047](https://github.com/kprompt/kprompt-architecture/issues/205)).

Companion: [agent.md](./agent.md) · [namespace-agent.md](./namespace-agent.md) · chart [`charts/kprompt-agent`](../charts/kprompt-agent).

## Install checklist

1. Pick **one watch namespace** (or one agent Deployment per ns).
2. Create Secret for LLM + Slack/webhook (never put keys in values/CRD plaintext).
3. `helm upgrade --install` with image your cluster can pull.
4. Confirm Role is namespaced: `kubectl -n <ns> get role,rolebinding,sa -l app.kubernetes.io/name=kprompt-agent`.
5. Tail agent logs; expect `watching namespace … (read-only)`.

```bash
helm upgrade --install kprompt-agent ./charts/kprompt-agent \
  -n payments --create-namespace \
  --set image.tag=<tag> \
  --set agent.heuristic=true   # demos / offline
```

## RBAC matrix

| Capability | Default Role | Extra |
|------------|--------------|--------|
| Pods, Events, logs, Deployments, Services, Ingress, HPA, Quota, LimitRange | get/list/watch | — |
| Secrets | **off** | `--watch …,secrets` → metadata only |
| Memory / incidents ConfigMap backend | off unless `*Backend=configmap` | create/get/update named CMs |
| `KpromptAgent` status | off | `agentCR.name` → status patch |
| Argo Applications / Flux Kustomizations | **off** | `agent.gitopsEvidence=true` |
| Workload create/update/delete/patch | **never** | Autopilot apply not in chart |

**Do not** widen to ClusterRole “just in case.” Cross-ns needs Coordinator ([ADR-0017](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0017-coordinator.md)), not a god-mode ns agent.

Argo CD Applications often live in `argocd`, not the app ns — `--gitops-evidence` only sees objects **in the watch namespace**. Document that limitation for operators.

## Cost controls (LLM + alert fatigue)

| Lever | Guidance |
|-------|----------|
| `--heuristic` | Zero token spend; fine for demos and many detectors |
| `--min-severity` / `--min-confidence` | Defaults medium / 0.7 — raise to cut noise |
| Incident batching | One LLM call per evidence fingerprint, not per raw Event |
| `--patterns` | Local disk/CM; no cloud upload; dampens repeat analysis quality issues |
| Slack threads | Prefer bot token + channel over webhook for update-in-thread |
| Prom / OTel | Optional; missing → `degraded:` — no invented metrics |

Rough expectation: busy ns with LLM on, gate at medium/0.7 → handful of completions per real incident, not hundreds per hour. If spend spikes, check gate settings and whether CrashLoop storms reopen fingerprints.

## Durability & state

| Store | Flag / value | Where |
|-------|--------------|--------|
| Patterns | `--patterns` | `~/.config/kprompt/patterns` or CM |
| Memory | `--memory` | file or `kprompt-namespace-memory` CM |
| Incidents | `--incidents-backend file\|configmap` | restart-safe open incidents + Slack thread ts |
| Autopilot audit | `--autopilot-propose` | local JSONL only |

All of the above stay **local / in-cluster** — not uploaded to `api.kprompt.ai` by default.

## Troubleshooting

| Symptom | Check |
|---------|--------|
| No alerts | Gate (`min-severity` / `min-confidence`); is `--analyze` on? Open incidents? |
| `degraded: prometheus\|otel\|gitops` | Backend URL / CRDs / RBAC; expected when opt-in signal missing |
| Slack ask silent | `--slack-ask` + bot token; Events API URL / port-forward to `--slack-ask-addr` |
| False-positive learning no-op | Need `--patterns` with ask callback |
| Handoff errors | `--coordinator-url` reachable; envelope validation (report needs summary + ns) |
| OOM / CPU on agent pod | Prefer heuristic; lower watch set; raise resources in values |
| Forbidden on GitOps list | Enable `gitopsEvidence` Role rules **or** turn the flag off |

## Security hygiene

- Rotate provider + Slack tokens via Secret; restart Deployment.
- Never enable Secret **value** reads (agent does not support it).
- Treat InvestigationReport / Slack text as sensitive (may include log snippets).
- Autopilot propose audit files may name workloads — protect laptop paths in shared demos.

## Coordinator (AG-037…AG-039 · AG-050)

```bash
# Laptop / kind
kprompt agent coordinator --addr :9090
kprompt agent coordinator --addr :9090 --probe-kube   # read-only suspect-ns probe
kprompt agent coordinator --addr :9090 --knowledge-backend configmap --in-cluster --knowledge-namespace kprompt-system
kprompt agent coordinator knowledge --url http://127.0.0.1:9090

# In-cluster (optional probe into named namespaces)
helm upgrade --install kprompt-coordinator ./charts/kprompt-coordinator \
  -n kprompt-system --create-namespace \
  --set probe.enabled=true \
  --set rbac.probeNamespaces={platform}
```

| Check | Expect |
|-------|--------|
| `GET /healthz` | `ok` |
| `POST /v1/handoff` | `CoordinatorReply` JSON, `mutateAttempted: false` |
| `GET /v1/recent` | In-memory recent handoff records (restart-lossy) |
| `GET /v1/knowledge` | Shared Knowledge summary (namespace edges; AG-059 · AG-060 durable when Store set) |
| RBAC | SA only by default — **no** ClusterRole unless `rbac.clusterRole.create=true` (namespaces get/list only) |
| Probe RBAC | With `probe.enabled` + `rbac.probeNamespaces`: Pods/Events `get/list` in listed ns only |
| Ns agents | Stay Role-scoped; point `--coordinator-url` at the Service `/v1/handoff` |

Handoff errors on the ns agent: URL wrong, report validation failed, or Coordinator down — Observe loop continues.

## Escalation path

1. Namespace Agent report + Slack thread  
2. Human / runbook  
3. Cross-ns suspicion → Coordinator handoff (when service exists)  
4. Remediation → CLI plan/approve or future `policyAuto` — never “agent fixed it silently”

## Related

- Modes & non-claims: [namespace-agent.md](./namespace-agent.md)
- Pipeline flags: [agent.md](./agent.md)
- Examples: [kprompt-examples](https://github.com/kprompt/kprompt-examples)
