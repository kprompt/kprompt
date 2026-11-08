# `kprompt init`

Day-0 setup for natural-language plans. Writes non-secret prefs to `~/.kprompt/config.yaml` (`provider`, `model`, optional `context`).

Does **not** create clusters, install Helm/Prometheus, or enroll Team (`kprompt login`). Use `kprompt setup` for host/cluster tool bootstrap and `kprompt doctor` to re-check health.

## Quick path

```bash
# 1) Point kubeconfig at a live cluster (or run: kprompt demo)
kubectl cluster-info

# 2) $0 local LLM — keep ollama running before the first NL plan
ollama serve && ollama pull llama3.2
kprompt init --ollama

# BYOK (instead of Ollama)
kprompt init --provider openai
export KPROMPT_OPENAI_API_KEY=sk-...
```

Then:

```bash
kprompt                 # readiness coach (TTY) / brief next step (pipes)
kprompt doctor
kprompt "list pods"
kprompt "how's my cluster"   # no LLM key
```

**Cold-machine traps (IC-012):** a bare non-TTY `kprompt` used to always say `init --ollama` even when the real blocker was kube — fixed to name the actual next step. `init --ollama` only writes prefs; it does not start Ollama or create a cluster. If plans dial `:11434` and fail, run `ollama serve`. If coach says `current-context unset`, run `kubectl config get-contexts` / `kubectl config use-context …`.

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
- [adopt.md](./adopt.md) — brownfield ~15 min on an existing cluster
- [setup.md](./setup.md) — Helm / Argo / Prom bootstrap (not LLM onboarding)
- **Security hardening (SEC-007):** when running the in-cluster Observe agent, prefer an egress NetworkPolicy — [operator-endpoint-hardening.md](./security/operator-endpoint-hardening.md)
