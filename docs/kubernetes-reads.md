# Kubernetes reads (generic get/list)

kprompt can **get/list any discoverable Kubernetes resource** (built-ins and CRDs) using cluster discovery + the dynamic client. Reads are **plan → run without approval**. Mutations still require review/`--approve`.

## Examples

```bash
kprompt "list deployments"
kprompt "show pods" -n default
kprompt "how many nodes are in the cluster"
kprompt "clusterda kaç node var"
kprompt "list configmaps" -n default
kprompt "get secret db-creds" -n prod          # table shows type + key count, not values
kprompt "list widgets.example.com" -n demo      # group-qualified CRD
kprompt "list nodes" -o json
```

## Resource identity

`target.kind` (from the LLM) may be:

| Form | Example |
|------|---------|
| Kind | `Pod`, `Node`, `ConfigMap`, `Secret` |
| Plural | `pods`, `nodes`, `secrets` |
| Short name | `po`, `no`, `cm`, `deploy` |
| Group-qualified | `deployments.apps`, `widgets.example.com` |

If a short name matches multiple API resources, kprompt asks you to qualify with the group (e.g. `deployments.apps`).

## Scope, limits, RBAC

- **Namespaced** resources default to `default` (or `-n` / prompt namespace).
- **Cluster-scoped** resources (Node, Namespace, StorageClass, …) do not take a namespace.
- List **limit** defaults to 500 (max 5000); optional `params.timeout` (default `30s`).
- Authorization is your **kubeconfig identity**. Forbidden calls print a short RBAC hint (`kubectl auth can-i …`). kprompt does not elevate privileges.
- Secrets use the same get/list path as other resources (no special deny/redaction in the table). Treat terminal output like `kubectl get secret`.

## Tests

Deterministic kind coverage (stub LLM, no live model):

```bash
go test -tags=e2e ./test/e2e/ -run 'TestGenericReadMatrixOnKind|TestListDeploymentsOnKind' -count=1 -v -timeout 15m
```

See [e2e.md](./e2e.md).
