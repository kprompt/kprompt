---
name: kind-e2e
description: >-
  Runs or extends kprompt kind E2E tests under test/e2e. Use when verifying pipeline
  against a live apiserver, adding Kind coverage for deploy/scale/wait/delete/agent,
  or when the user asks for e2e, kind, or integration matrix tests.
---

# Kind E2E workflow

## Prerequisites

- Docker (or compatible) running
- `kind` and `kubectl` on PATH
- No real LLM API key — tests use stub providers

See `docs/e2e.md`.

## Run

Full suite (creates/reuses cluster `kprompt-e2e`):

```bash
go test -tags=e2e ./test/e2e/ -count=1 -v -timeout 15m
```

Focused (faster while iterating):

```bash
go test -tags=e2e ./test/e2e/ -run 'TestNameHere' -count=1 -v -timeout 15m
```

Cleanup:

```bash
kind delete cluster --name kprompt-e2e
```

## Extending tests

1. Prefer helpers in `test/e2e/helpers_test.go` over new shell scripts.
2. Exercise real product paths (Intent → Planner → Safety → Executor), not kubectl-only assertions.
3. Keep tests keyless (stub LLM).
4. For read matrix / integration family, follow patterns in `matrix_test.go` and `docs/integration-matrix.md`.
5. Do not require elevated product RBAC beyond what the scenario documents.

## When not to run full E2E

Unit/package tests are enough for pure logic. Use kind when behavior depends on a live apiserver, rollout wait, RBAC denial, or agent watch fixtures.
