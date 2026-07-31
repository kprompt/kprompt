# kprompt-coordinator Helm chart

Thin **Coordinator** HTTP fan-in for [kprompt](https://github.com/kprompt/kprompt) Namespace Agents ([ADR-0017](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0017-coordinator.md) · AG-037…AG-039 · AG-050…AG-051).

```text
kprompt agent coordinator --addr :9090
kprompt agent coordinator --addr :9090 --probe-kube   # read-only Pods/Events in suspect ns
```

**Never mutates workloads.** Receives `CoordinatorHandoff`, returns `CoordinatorReply` with merged InvestigationReport v2. Shared Knowledge: `GET /v1/knowledge`; with `knowledge.enabled=true` (default) persists the handoff ring in ConfigMap `kprompt-coordinator-knowledge` (AG-060).

## Install

```bash
helm upgrade --install kprompt-coordinator ./charts/kprompt-coordinator \
  -n kprompt-system --create-namespace \
  --set image.tag=<tag>
```

With optional kube probe into named namespaces:

```bash
helm upgrade --install kprompt-coordinator ./charts/kprompt-coordinator \
  -n kprompt-system --create-namespace \
  --set image.tag=<tag> \
  --set probe.enabled=true \
  --set rbac.probeNamespaces={platform,shared}
```

Ns agents point at the Service:

```bash
# on each Namespace Agent
--coordinator-url http://kprompt-coordinator.kprompt-system.svc:9090/v1/handoff
```

## RBAC (AG-039 · AG-051)

| Subject | Scope | Notes |
|---------|-------|-------|
| Namespace Agent | **Role** in watch ns | Unchanged — never ClusterRole-by-default |
| Coordinator | **ServiceAccount only** by default | No ClusterRole; no get/list/watch pods/Secrets cluster-wide |
| Optional ClusterRole | **off** (`rbac.clusterRole.create=false`) | namespaces get/list only — still no mutate |
| Optional probe Roles | **off** until `probe.enabled` + `rbac.probeNamespaces` | Pods + Events `get/list` in listed ns only |

Do not grant the Coordinator `create/update/patch/delete` on workloads.

## Values

| Key | Default | Notes |
|-----|---------|-------|
| `image.repository` | `ghcr.io/kprompt/kprompt` | Same binary as the ns agent |
| `service.port` | `9090` | HTTP API |
| `rbac.clusterRole.create` | `false` | Keep minimal |
| `probe.enabled` | `false` | Passes `--probe-kube --in-cluster` |
| `rbac.probeNamespaces` | `[]` | RoleBindings for AG-050 probe |

See [docs/namespace-agent.md](../../docs/namespace-agent.md) · [docs/agent-ops.md](../../docs/agent-ops.md).
