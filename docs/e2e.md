# E2E tests (kind)

Requires `kind` and `kubectl` on PATH. Docker (or compatible runtime) must be running.

```bash
# Creates/reuses kind cluster "kprompt-e2e", deploys nginx, scales to 3 via stub LLM + pipeline.
go test -tags=e2e ./test/e2e/ -count=1 -v -timeout 10m
```

Optional cleanup:

```bash
kind delete cluster --name kprompt-e2e
```

Notes:

- Uses `llm.ScaleStub` so no real API key is required.
- Exercises: Intent parse → Planner → Safety → Executor scale against a live APIserver.
