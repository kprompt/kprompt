# kprompt Observe agent

In-cluster, **namespace-scoped** agent that continuously watches Pods/Events (and optional workloads), correlates incidents, optionally analyzes with an LLM, and notifies Slack/webhooks.

This is **Observe Mode** by default — it never applies, patches, or deletes cluster resources ([ADR-0013](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0013-in-cluster-agent.md)). Optional `--autopilot-propose` emits PlanResult-shaped proposals only ([ADR-0015](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0015-autopilot-mode.md)); **apply** stays gated. The Namespace Agent continuous-intelligence contract is [ADR-0016](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0016-namespace-agent.md).

## Positioning (honest)

| Tool | Job | How kprompt Observe differs |
|------|-----|-----------------------------|
| **K8sGPT** | On-demand / scheduled **analyzer** (scan → explain) | We are **always-on watch → correlated Incident → gated alert**, not a fleet scanner. Use K8sGPT when you want analyzer findings; use this agent when you want threaded Slack alerts from live Events/Pods. |
| **Kagent** | **In-cluster agent framework** (multi-agent CRDs / tools) | We ship one **kprompt-native Observe pipeline** (Incident / AgentAlert + PlanResult DNA), not a general multi-agent platform. Do not expect Kagent feature parity. |
| **kprompt CLI** | Reactive intent compiler (plan → approve → apply) | The agent is **optional**. The laptop CLI still needs no daemon ([ADR-0001](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0001-go-cli.md)). |

Explicit non-claims: no silent remediations, no ClusterRole-by-default on namespace agents, no “we host your fleet agent” SaaS. Autopilot **apply** is opt-in (`policyAuto` + allowlist + explicit approve/`--autopilot-apply`) — never LLM-said-so.

**Modes table (Observe vs Namespace Agent vs Coordinator):** [namespace-agent.md](./namespace-agent.md) · **Ops runbook (cost/RBAC):** [agent-ops.md](./agent-ops.md).

## Where files live

CLI config and Observe local stores use **different** roots on purpose (no automatic migration):

| Path | Purpose |
|------|---------|
| `~/.kprompt/config.yaml` | CLI providers, aliases, tool URLs |
| `~/.kprompt/history.jsonl` | Prompt / plan history (no secrets) |
| `~/.kprompt/` credentials / policy | Team login + pulled org policy |
| `~/.config/kprompt/memory/` | Observe namespace memory (AG-015) |
| `~/.config/kprompt/patterns/` | Observe pattern learning (AG-016) |
| `~/.config/kprompt/incidents/` | Durable incidents (AG-032) |
| `~/.config/kprompt/autopilot/` | Autopilot audit JSONL (AG-017) |

In-cluster agents may use ConfigMaps (`kprompt-namespace-memory`, `kprompt-incident-state`, `kprompt-remediation-policy`) instead of the `~/.config/kprompt` paths.

## RBAC

Default install is a **Role + RoleBinding in one namespace** (get/list/watch on pods, events, logs, deployments, …). Not a ClusterRole god-mode SA.

- Secrets **watch is off by default**; when enabled (`--watch …,secrets`), only **metadata** is emitted (type + key count) — never Secret values.
- Status sync onto a `KpromptAgent` CR (`--agent-cr`) adds **status patch** verbs for that CR only — still no workload mutate.
- You remain responsible for the ServiceAccount scope you deploy.

## LLM cost

- The agent does **not** call the LLM on every raw API event. It batches by open **Incident**, then applies a **severity + confidence gate** before Slack/webhook.
- Prefer `--heuristic` for demos / offline; turn LLM on when you accept API spend.
- Gate tighter with `--min-severity` / `--min-confidence` (defaults: medium / 0.7) to limit alert fatigue and token burn.
- Credentials stay in a **Secret** (`envFrom`) — never in CRD/ConfigMap plaintext.

## Laptop smoke test

```bash
kprompt agent run -n payments --analyze --fetch-logs --health --heuristic
kprompt agent run -n payments --slack --fetch-logs   # needs Slack env
```

Need a namespace that actually misbehaves? [kprompt-examples](https://github.com/kprompt/kprompt-examples) provisions a kind cluster plus seven failure scenarios (crashloop, image pull, OOM, stalled rollout, unbound PVC, failing CronJob, missing dependency), each documenting what the agent is expected to conclude:

```bash
git clone https://github.com/kprompt/kprompt-examples.git && cd kprompt-examples
make walkthrough   # up → break-all → verify → agent-full (~45s)
```

Or step by step:

```bash
make up
make break SCENARIO=01-crashloop
make verify
kprompt agent run -n payments --analyze --health --heuristic
```

## Helm install (AG-012)

Preferred in-cluster path: [`charts/kprompt-agent`](../charts/kprompt-agent).

```bash
docker build -t ghcr.io/kprompt/kprompt:dev .
# push to a registry your cluster can pull

kubectl -n payments create secret generic kprompt-agent \
  --from-literal=OPENAI_API_KEY="$OPENAI_API_KEY"

helm upgrade --install kprompt-agent ./charts/kprompt-agent \
  -n payments --create-namespace \
  --set image.tag=dev \
  --set agent.heuristic=false
```

Chart README: [`charts/kprompt-agent/README.md`](../charts/kprompt-agent/README.md). Website mirror: [kprompt.ai/docs/agent](https://kprompt.ai/docs/agent).

RBAC is a **Role** in the watch namespace (pods, events, logs, deployments, … — get/list/watch only).

## KpromptAgent CRD (AG-013)

CRD installs with the Helm chart (`charts/kprompt-agent/crds/`). Standalone:

```bash
kubectl apply -f deploy/crd/kprompt.ai_kpromptagents.yaml
kubectl apply -f config/samples/kpromptagent.yaml
```

`spec.mode` defaults to **Observe**. Status fields: `healthScore`, `healthTrend`, `lastAlert`, `openIncidents`, `conditions`.

Optional status sync from the running agent (no Operator yet):

```bash
# CLI
kprompt agent run -n payments --health --analyze --heuristic \
  --agent-cr demo --agent-cr-namespace payments

# Helm
helm upgrade --install kprompt-agent ./charts/kprompt-agent -n payments \
  --set agentCR.name=demo \
  --set agentCR.create=true
```

Then:

```bash
kubectl get kpromptagents -n payments
kubectl get kpa demo -n payments -o yaml   # status.healthScore / status.lastAlert
```

Full Deployment lifecycle for the CR is **AG-014 Operator**.

## Operator (AG-014)

Optional controller that watches `KpromptAgent` CRs and creates the Observe agent ServiceAccount, Role, RoleBinding, and Deployment.

```bash
# Laptop
kprompt agent operator --once -n payments
kprompt agent operator --in-cluster   # in-cluster via Helm chart

# Helm
kubectl apply -f deploy/crd/kprompt.ai_kpromptagents.yaml
helm upgrade --install kprompt-operator ./charts/kprompt-operator \
  -n kprompt-system --create-namespace \
  --set image.tag=dev \
  --set defaultAgentImage=ghcr.io/kprompt/kprompt:dev
kubectl apply -f config/samples/kpromptagent.yaml
```

Constraints (V1):

- Mode must be **Observe** (Autopilot rejected)
- `spec.namespace` empty or equal to the CR namespace (no cross-namespace)
- Operator uses a **ClusterRole** to manage agent objects; prefer the manual `kprompt-agent` chart if you want Role-only installs

Chart: [`charts/kprompt-operator`](../charts/kprompt-operator).

### Secret keys

| Env key | Purpose |
|---------|---------|
| `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` / … | LLM (same as CLI) |
| `KPROMPT_SLACK_BOT_TOKEN` + `KPROMPT_SLACK_CHANNEL` | Threaded Slack (preferred) |
| `KPROMPT_SLACK_WEBHOOK_URL` | Slack webhook fallback |
| `KPROMPT_WEBHOOK_URL` | Generic AgentAlert JSON POST |
| `KPROMPT_AGENT_CR` (+ `_NAMESPACE`) | Patch KpromptAgent.status |

## Watched resources (AG-004 · AG-023)

Default is `pods,events`. Expand with `--watch`:

```bash
kprompt agent run -n payments \
  --watch pods,events,deployments,services,ingresses,hpa,resourcequotas,limitranges,pvc,configmaps
```

| Value(s) | Kind |
|----------|------|
| `pods` | Pod |
| `events` | Event |
| `deployments` / `deploy` | Deployment (ready/updated/available) |
| `replicasets` / `rs` | ReplicaSet |
| `statefulsets` / `sts` | StatefulSet |
| `jobs` | Job (Complete/Failed) |
| `cronjobs` / `cj` | CronJob (schedule/suspend) |
| `pvc` | PersistentVolumeClaim (phase) |
| `configmaps` / `cm` | ConfigMap (key count) |
| `services` / `svc` | Service |
| `ingresses` / `ing` | Ingress (hosts / LB pending) |
| `hpa` | HorizontalPodAutoscaler (at max) |
| `resourcequotas` / `quota` | ResourceQuota (exceeded) |
| `limitranges` | LimitRange |
| `secrets` | Secret — **opt-in, metadata only** (never values, ADR-0013) |

Secrets are never watched implicitly and only metadata (type + key count) is emitted.

**Node pressure:** derived from Events (`NodeNotReady`, `SystemOOM`, …). Cluster-scoped Node objects are not watched (Role default, ADR-0016).

**Metrics (AG-024):** when `KPROMPT_PROMETHEUS_URL` (or config `tools.prometheus.url`) is set, context builder attaches CPU/memory/restart metric EvidenceRefs. Missing Prom → `degraded: prometheus` (never invents values).

**Traces (AG-025):** when `KPROMPT_OTEL_ENDPOINT` (+ optional `KPROMPT_OTEL_BACKEND`) is set, context builder attaches compact trace EvidenceRefs. Missing OTel → `degraded: otel`.

**GitOps evidence (AG-035):** `--gitops-evidence` lists Argo CD Applications / Flux Kustomizations in the watch namespace and attaches sync/health + deploy history EvidenceRefs (`type=gitops`). Opt-in; missing CRDs → `degraded: gitops`. Helm: `agent.gitopsEvidence`.

**Priority policy (AG-030):** analysis stamps an ADR-0016 objective (`outage` → … → `best_practices`) and raises severity to the objective floor (never lowers). Recommended actions are ranked accordingly.

**Durable incidents (AG-032):** `--incidents-backend file|configmap` persists open incidents / Slack thread ts across restarts (`~/.config/kprompt/incidents` or ConfigMap `kprompt-incident-state`).

**Slack ask (AG-019):** `--slack-ask` listens on `--slack-ask-addr` (default `:8080`) for Slack Events (`status` / `why` / `what broke` / `false positive`). Read-only for the cluster — never mutates. With `--patterns`, `false positive` dampens future “seen before” boosts (AG-033). Requires bot token mode + Events API URL (or port-forward).

**Coordinator handoff (AG-036 · AG-037 · AG-048…AG-053 · AG-059 · AG-060):** Ns agents POST with `--coordinator-url`. Run the thin fan-in with `kprompt agent coordinator --addr :9090` (or Helm [`charts/kprompt-coordinator`](../charts/kprompt-coordinator)). Optional `--probe-kube` enables a read-only Pods/Events probe of `suspectNamespace`. Returns `CoordinatorReply` with merged InvestigationReport; with `--slack` (bot token) the reply is posted into the incident thread, and `--webhook` gets a JSON follow-up. **Shared Knowledge:** `GET /v1/knowledge` (and `kprompt agent coordinator knowledge`) summarizes handoff edges; Helm defaults to ConfigMap persistence (`--knowledge-backend configmap`, AG-060) — still not a full continuous blast-radius product graph ([coordinator-knowledge.md](./coordinator-knowledge.md)). **Mutate stays off** ([ADR-0017](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0017-coordinator.md)).

## Pipeline flags

| Flag | Task |
|------|------|
| `--watch` | AG-004 resource selection |
| `--incidents` | AG-006 correlate |
| `--fetch-logs` | AG-005 on-demand logs |
| `--build-context` | AG-007 context |
| `--analyze` | AG-008 gated AgentAlert |
| `--slack` | AG-009 |
| `--webhook` | AG-010 |
| `--health` | AG-011 score |
| `--agent-cr` | AG-013 status sync |
| `--memory` | AG-015 namespace facts |
| `--patterns` | AG-016 seen-before + AG-033 outcome weights |
| `--autopilot-propose` | AG-017 / ADR-0015 propose (default) |
| `--autopilot-policy` | AG-040 RemediationPolicy file |
| `--autopilot-apply` | AG-042 policyAuto in-loop apply (off by default) |
| `--slack-ask` | AG-019 ask (+ FP learning with `--patterns`) |
| `--coordinator-url` | AG-036 Coordinator handoff (opt-in) |
| `--gitops-evidence` | AG-035 Argo/Flux EvidenceRefs (opt-in) |

## Incident Memory (AG-015 · AG-016 · AG-032…AG-034 · AG-054)

**Shipped** as the Learn stack — local / in-cluster only, never uploaded to `api.kprompt.ai`.

| Layer | Flag / CLI | What it remembers |
|-------|------------|-------------------|
| Namespace facts | `--memory` · `agent memory list` | Redis/Postgres/… deps (evidence, not proof) |
| Incident patterns | `--patterns` · `agent patterns list` | Signatures → “Seen before (N×)” + outcome weights |
| Durable incidents | `--incidents-backend` | Open incidents + Slack thread ts across restarts |

Helm chart defaults (`charts/kprompt-agent`) persist all three via ConfigMaps so pod restarts do not wipe memory.

```bash
# Laptop (file backends)
kprompt agent run -n payments --analyze --heuristic --memory --patterns \
  --incidents-backend file

kprompt agent memory list -n payments
kprompt agent patterns list -n payments

# In-cluster ConfigMaps (Helm defaults)
kprompt agent patterns list -n payments --patterns-backend configmap
```

**Honesty:** memory/patterns boost confidence and explainability — they never auto-mutate. Memory alone never proves root cause (AG-034). Thin **Knowledge Graph** MVP (service graph + impact + memory deps) is documented in [graph.md](./graph.md); full continuous topology stays building.

## Namespace memory (AG-015)

Persists dependency facts (“uses Redis/Kafka/Postgres”) **locally or in-cluster only** — never uploaded to `api.kprompt.ai` by default.

```bash
# Discover + inject into analyzer context while watching
kprompt agent run -n payments --analyze --heuristic --memory

# Manual facts (file backend → ~/.config/kprompt/memory)
kprompt agent memory set -n payments --kind dependency --key redis --value "cache for sessions"
kprompt agent memory discover -n payments
kprompt agent memory list -n payments

# In-cluster ConfigMap backend (Helm agent.memoryBackend=configmap)
kprompt agent memory list -n payments --memory-backend configmap
```

Relevant facts are filtered into `AgentContext.memory` / `namespace_memory (evidence, not proof):` prompt blocks when the incident text mentions the dependency or infra failure patterns (timeout, connection refused, …). Memory alone never proves root cause (AG-034) — confidence is capped without Events/logs/metrics/traces.

## Pattern learning (AG-016 · AG-033)

Remembers incident signatures (reason + workload kind + bucket like crashloop/oom) under `~/.config/kprompt/patterns`. When a similar incident appears (≥2 priors), confidence is boosted and root cause is annotated with **Seen before (N×)** — still **Observe-only**; patterns never trigger apply/patch/delete.

**Outcome learning (AG-033):** alert recovered → `Confirmed` weight up; Slack `false positive` → `FalsePositives` weight down (dampens future boost).

```bash
kprompt agent run -n payments --analyze --heuristic --patterns
kprompt agent patterns list -n payments
```

## Autopilot (AG-017 · AG-040…AG-044 · ADR-0015)

Opt-in. Default remains Observe / **proposeOnly**. **Silent apply is not shipped** — mutate requires RemediationPolicy `mode=policyAuto` **and** `apply=true`, plus an explicit gate (`apply-proposal --approve` or `--autopilot-apply`). Helm chart defaults keep both propose and apply flags off.

```bash
# Propose allowlisted actions (rollback / restart / scale / evict)
kprompt agent run -n payments --analyze --heuristic --autopilot-propose

# Optional RemediationPolicy JSON (AG-040); else ConfigMap kprompt-remediation-policy or defaults
kprompt agent run -n payments --analyze --heuristic --autopilot-propose \
  --autopilot-policy ./policy.json

# Human approve bridge (AG-043) — requires --approve + policyAuto
kprompt agent autopilot apply-proposal --file proposal.json --approve --policy ./policy-auto.json
```

| Action ID | Trigger (heuristic) |
|-----------|---------------------|
| `rollbackFailedRollout` | ProgressDeadline / failed rollout |
| `restartDeployment` | CrashLoop / OOM / ImagePull |
| `evictPod` | Node pressure / eviction signals |
| `scaleDeployment` | Explicit scale language in RCA |

**Deny pack (AG-044):** wipe/delete namespace/cluster, Secret values, fabricated evidence — never allowlistable.

**Apply (AG-042):** only when RemediationPolicy `mode=policyAuto` **and** `apply=true`, plus `--autopilot-apply` (in-loop) or `apply-proposal --approve`. Helm defaults keep both false.

**Not shipped:** silent LLM-said-so apply.
