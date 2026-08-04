# Development

Architecture overview lives in the private `kprompt/kprompt-architecture` repo (ADRs).

## Layout

CLI and core flow:

- `cmd/kprompt` — CLI entry
- `internal/pipeline` — Prompt → Intent → Plan → Safety → Apply
- `internal/intent` / `internal/planner` — parse request and build plans
- `internal/safety` — hard deny + risk checks (including Argo-aware gates)
- `internal/executor` — apply mutations (Kubernetes / Argo and related runtimes)

LLM providers (see also [providers.md](./providers.md)):

- `internal/llm` — provider interface + OpenAI, Anthropic, Gemini, and OpenAI-compatible adapters (Ollama, Groq, xAI, …)

Observe / ops packages (Investigate fleet and related commands):

- `internal/audit` — read-only security / hygiene scan
- `internal/cleanup` — unused / stale Kubernetes resource detection
- `internal/impact` — blast-radius / consumer discovery (read-only)
- `internal/investigate` / `internal/agent` — investigation orchestration and agent fleet
- `internal/drift`, `internal/doctor`, `internal/optimize`, `internal/score`, `internal/why` — supporting diagnose / score / explain surfaces
- `internal/search`, `internal/graph`, `internal/timeline`, `internal/history` — context and history helpers
- `internal/session`, `internal/setup`, `internal/config`, `internal/cluster` — runtime config and cluster access
- `internal/ui` / `internal/output` — presentation helpers

This list is a map for first-time contributors, not every package under `internal/`. Prefer package comments and existing `docs/*.md` for depth.

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
