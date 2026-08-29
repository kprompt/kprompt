# kprompt-operator Helm chart

Reconciles [`KpromptAgent`](../../deploy/crd/kprompt.ai_kpromptagents.yaml) CRs into namespace-scoped **Observe** agent Deployments (AG-014).

- Creates ServiceAccount + Role + RoleBinding + Deployment per CR
- **Never** enables Autopilot (rejects non-`Observe` modes)
- V1: CR namespace must equal the watch namespace

Prefer the manual [`kprompt-agent`](../kprompt-agent) chart when you do not want a cluster-scoped operator SA.

## NetworkPolicy (SEC-007)

Opt-in egress default-deny via `networkPolicy.enabled` (default `false`). Set `networkPolicy.kubeAPIServerCIDRs` before enabling. Guide: [`docs/security/operator-endpoint-hardening.md`](../../docs/security/operator-endpoint-hardening.md).

## Install

```bash
# CRD (also shipped with kprompt-agent chart)
kubectl apply -f deploy/crd/kprompt.ai_kpromptagents.yaml

helm upgrade --install kprompt-operator ./charts/kprompt-operator \
  -n kprompt-system --create-namespace \
  --set image.tag=dev \
  --set defaultAgentImage=ghcr.io/kprompt/kprompt:dev

# Then create a CR in the namespace you want observed:
kubectl apply -f config/samples/kpromptagent.yaml
# Ensure Secret kprompt-agent exists in that namespace for LLM/Slack keys.
```

## Laptop

```bash
kprompt agent operator --once -n payments
kprompt agent operator              # all namespaces until Ctrl-C
```
