# `kprompt init`

Day-0 setup for natural-language plans. Writes non-secret prefs to `~/.kprompt/config.yaml` (`provider`, `model`, optional `context`).

Does **not** create clusters, install Helm/Prometheus, or enroll Team (`kprompt login`). Use `kprompt setup` for host/cluster tool bootstrap and `kprompt doctor` to re-check health.

## Quick path

```bash
# $0 local LLM
ollama serve && ollama pull llama3.2
kprompt init --ollama

# BYOK
kprompt init --provider openai
export KPROMPT_OPENAI_API_KEY=sk-...
```

Then:

```bash
kprompt doctor
kprompt "list pods"
kprompt "how's my cluster"   # no LLM key
```

## Flags

| Flag | Meaning |
|------|---------|
| `--ollama` | Set provider to `ollama` (no API key) |
| `--provider` | Named preset (`openai`, `anthropic`, `gemini`, …) |
| `--model` | Override preset default model |
| `--context` | Persist a kubeconfig context |
| `--dry-run` | Print what would be written; do not save |

Non-interactive shells **require** `--ollama` or `--provider` (no hanging prompts).

Interactive TTY with bare `kprompt init` walks provider → model → optional context.

## Related

- Bare `kprompt` — readiness coach (points here when unconfigured)
- [providers.md](./providers.md) — full preset list
- [doctor.md](./doctor.md) — health report
- [setup.md](./setup.md) — Helm / Argo / Prom bootstrap (not LLM onboarding)
