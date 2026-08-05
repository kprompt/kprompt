# Development

Architecture overview lives in the private `kprompt/kprompt-architecture` repo (ADRs).

## Layout

- `cmd/kprompt` — CLI entry

### `internal/agent/`
- `internal/agent/analyze` — Turns AgentContext into a gated AgentAlert.
- `internal/agent/autopilot` — Auto fixes cluster issues, policy-gated when turned on.
- `internal/agent/brief` — Builds a read-only Namespace Agent intelligence summary.
- `internal/agent/confidence` — Calibrates Namespace Agent confidence from amount of evidence.
- `internal/agent/coordinator` — Merges cross-namespace investigations, never mutates.
- `internal/agent/ctxbuild` — Gathers cluster data to give the LLM.
- `internal/agent/memory` — Persists namespace dependency facts for Observe context.


### `internal/`

- `internal/audit` — Runs a read-only security / hygiene scan.
- `internal/cleanup` — Detects unused / stale Kubernetes resources.
- `internal/cluster` — Connects and talks to a Kubernetes cluster.
- `internal/config` — Saves non-secret kprompt settings into a file.
- `internal/drift` — Reports live cluster vs GitOps desired state.
- `internal/executor` — Applies mutations (Kubernetes / Argo and related runtimes).
- `internal/llm` — Provider interface + model adapters.
- `internal/optimize` — Read-only report on efficiency of a Kubernetes cluster.
- `internal/pipeline` — Prompt → Intent → Plan → Safety → Apply.
- `internal/planner` — Creates execution plan.
- `internal/safety` — Hard deny + risk of prompts.
- `internal/suggest` — Turns cluster findings into safer suggestions.
- `internal/tools/` — Checks what kprompt features you can use.
- `internal/ui` — Formats CLI.


## Test

```bash
go test ./...
```

Kind E2E scale (requires Docker + `kind`):

```bash
go test -tags=e2e ./test/e2e/ -count=1 -v -timeout 10m
```

See [docs/e2e.md](./e2e.md).

## Build

```bash
go build -o bin/kprompt ./cmd/kprompt
```

## Related

- Contributing: [CONTRIBUTING.md](../CONTRIBUTING.md)
- Compatible LLM provides: [docs/providers.md](./providers.md)
- Observe agent: [agent.md](./agent.md)
