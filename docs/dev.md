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

## Build

```bash
go build -o bin/kprompt ./cmd/kprompt
```
