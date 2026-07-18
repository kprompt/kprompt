# E2E tests (kind)

Requires `kind` and `kubectl` on PATH. Docker (or compatible runtime) must be running.

```bash
# Creates/reuses kind cluster "kprompt-e2e", deploys fixtures, runs stub LLM + pipeline.
go test -tags=e2e ./test/e2e/ -count=1 -v -timeout 15m
```

Optional focused generic-read matrix (T-051):

```bash
go test -tags=e2e ./test/e2e/ -run 'TestGenericReadMatrixOnKind|TestListDeploymentsOnKind' -count=1 -v -timeout 15m
```

Optional cleanup:

```bash
kind delete cluster --name kprompt-e2e
```

Notes:

- Uses stub LLM providers so no real API key is required.
- Exercises: Intent → Planner → Safety → Executor against a live APIserver.
- Generic read matrix covers Node (EN/TR prompts), ConfigMap, Secret, a sample CRD (`widgets.example.com`), JSON output, unknown resources, list limits, and RBAC denial (limited ServiceAccount — no elevated product RBAC).
- Product codepaths use client-go only; helpers may call `kind`/`kubectl` for cluster lifecycle.
