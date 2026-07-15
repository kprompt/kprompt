# Development

Architecture overview lives in the private `kprompt/kprompt-architecture` repo (ADRs).

## Layout

- `cmd/kprompt` — CLI entry
- `internal/pipeline` — Prompt → Intent → Plan → Safety → Apply
- `internal/llm` — Provider interface + OpenAI / Anthropic adapters
- `internal/safety` — Hard deny + risk
- `internal/executor` — Deployment scale (v0 mutation)

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
