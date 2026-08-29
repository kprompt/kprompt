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

## GitHub Actions (CI)

`.github/workflows/ci.yml` runs a separate **`e2e`** job (after unit `test`) with kind + SHA-pinned `azure/setup-kubectl`:

| Trigger | Suite |
|---------|--------|
| `push` / `pull_request` | **Smoke** — `TestGenericReadMatrixOnKind`, `TestDeployRedisOnKind`, `TestAgentWatchPodsEvents` (10m) |
| `workflow_dispatch` → `full` | Full `./test/e2e/` (15m) |
| Dependabot PRs | Job skipped |

No cluster secrets required (stub LLM). Fork PRs can run smoke like other PRs.

Optional cleanup:

```bash
kind delete cluster --name kprompt-e2e
```

Notes:

- Uses stub LLM providers so no real API key is required.
- Exercises: Intent → Planner → Safety → Executor against a live APIserver.
- Generic read matrix covers Node (EN/TR prompts), ConfigMap, Secret, a sample CRD (`widgets.example.com`), JSON output, unknown resources, list limits, and RBAC denial (limited ServiceAccount — no elevated product RBAC).
- Integration family matrix: `TestIntegrationMatrix` ([integration-matrix.md](./integration-matrix.md)).
- Product codepaths use client-go only; helpers may call `kind`/`kubectl` for cluster lifecycle.
