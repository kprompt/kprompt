# kprompt-agent Helm chart

Namespace-scoped **Observe Mode** agent for [kprompt](https://github.com/kprompt/kprompt).

Deploys a single Deployment that runs:

```text
kprompt agent run --namespace <ns> --in-cluster [--analyze] [--fetch-logs] [--health] …
```

Never mutates the cluster in Observe Mode (ADR-0013). Default is **Observe**. Optional `agent.autopilotPropose` emits proposals only (ADR-0015) and persists them to `kprompt-autopilot-proposals` (RT-007). `agent.autopilotApply` stays **false** by default and requires `agent.autopilotMode=policyAuto` plus RemediationPolicy `apply=true` — never silent LLM apply.

**RT-005 product path (Helm):**

```bash
helm upgrade --install kprompt-agent ./charts/kprompt-agent -n payments \
  --set agent.autopilotPropose=true \
  --set agent.autopilotMode=proposeOnly \
  --set agent.autopilotAllow='{rollbackFailedRollout}'

# Gated in-loop apply (explicit opt-in — review ADR-0015 first):
helm upgrade --install kprompt-agent ./charts/kprompt-agent -n payments \
  --set agent.autopilotPropose=true \
  --set agent.autopilotMode=policyAuto \
  --set agent.autopilotApply=true \
  --set agent.autopilotAllow='{rollbackFailedRollout}'
```

Honest positioning (vs K8sGPT / Kagent, RBAC, LLM cost): [docs/agent.md](../../docs/agent.md) · [docs/namespace-agent.md](../../docs/namespace-agent.md) · [docs/agent-ops.md](../../docs/agent-ops.md) · [kprompt.ai/docs/agent](https://kprompt.ai/docs/agent).

## Install

```bash
# Build/push image (until published tags exist)
docker build -t ghcr.io/kprompt/kprompt:dev .
docker push ghcr.io/kprompt/kprompt:dev

# Secret with provider + notifier credentials (do not commit values)
kubectl -n payments create secret generic kprompt-agent \
  --from-literal=OPENAI_API_KEY=sk-... \
  --from-literal=KPROMPT_SLACK_BOT_TOKEN=xoxb-... \
  --from-literal=KPROMPT_SLACK_CHANNEL=C... \
  --from-literal=KPROMPT_DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...

helm upgrade --install kprompt-agent ./charts/kprompt-agent \
  -n payments --create-namespace \
  --set image.tag=dev \
  --set agent.discord=true \
  --set agent.slack=true
```

## Values (high level)

| Key | Default | Notes |
|-----|---------|-------|
| `watchNamespace` | release ns | Namespace the agent watches |
| `image.repository` | `ghcr.io/kprompt/kprompt` | Container image |
| `agent.analyze` | `true` | AG-008 analyzer |
| `agent.fetchLogs` | `true` | AG-005 on-demand logs |
| `agent.health` | `true` | AG-011 health score |
| `agent.memory` | `true` | AG-015 namespace deps/facts |
| `agent.memoryBackend` | `configmap` | Incident Memory persist (file\|configmap) |
| `agent.patterns` | `true` | AG-016 seen-before confidence boost |
| `agent.patternsBackend` | `configmap` | Pattern store (file\|configmap) |
| `agent.incidentsBackend` | `configmap` | AG-032 durable open incidents |
| `agent.gitopsEvidence` | `false` | AG-035 Argo/Flux EvidenceRefs (+ Role rules) |
| `agent.autopilotPropose` | `false` | ADR-0015 propose-only (never apply by itself); enables `--proposals` store |
| `agent.autopilotApply` | `false` | AG-042 in-loop apply (needs `autopilotMode=policyAuto`) |
| `agent.autopilotMode` | `proposeOnly` | RT-005 RemediationPolicy mode (`proposeOnly` \| `policyAuto`) |
| `agent.autopilotAllow` | `[rollbackFailedRollout]` | RT-005 narrow allowlist in RemediationPolicy ConfigMap |
| `agent.autopilotMinConfidence` | `0.85` | Policy floor for propose/apply |
| `agent.proposalsBackend` | `configmap` | RT-007 durable proposal store (`kprompt-autopilot-proposals`) |
| `agent.remediationPolicy` | `false` | AG-040 create policy ConfigMap (auto when autopilotPropose/Apply) |
| `agent.discord` / `agent.slack` / `agent.webhook` | `false` | Enable notifiers |
| `secret.name` | `kprompt-agent` | Env-from Secret |
| `rbac.create` | `true` | Namespace Role (get/list/watch) |
| `agentCR.name` | `""` | Patch `KpromptAgent.status` (AG-013) |
| `agentCR.create` | `false` | Also create the CR from values |
| `networkPolicy.enabled` | `false` | Opt-in egress default-deny policy; strongly recommended for production |

## Security hardening (SEC-007 follow-up)

Decision A accepts operator-controlled endpoints (LLM, observability, webhook) as trusted inputs.
This chart does not auto-block private ranges; operators should enforce egress allowlists.

NetworkPolicy controls in this chart:

- `networkPolicy.enabled`: enable egress default-deny for agent pods
- `networkPolicy.allowDNS`: allow DNS to cluster DNS
- `networkPolicy.allowKubeApi` + `networkPolicy.kubeAPIServerCIDRs`: allow kube-apiserver egress via explicit CIDRs
- `networkPolicy.llmCIDRs`, `networkPolicy.observabilityCIDRs`, `networkPolicy.webhookCIDRs`: explicit endpoint allowlists

Use these settings with your cluster CNI policy model (Calico/Cilium/etc.) and keep allowlists narrow.

The chart installs the `KpromptAgent` CRD from [`crds/`](./crds). Sample: [`config/samples/kpromptagent.yaml`](../../config/samples/kpromptagent.yaml).

See [values.yaml](./values.yaml) and [docs/agent.md](../../docs/agent.md).
