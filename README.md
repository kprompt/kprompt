# kprompt

Open-source AI CLI to control Kubernetes with natural language.

> Talk to Your Cluster.

```bash
kprompt "scale api to 10" --approve --wait
kprompt "deploy redis" --approve
kprompt "deploy nginx" --approve
kprompt "rollback payment-api" --approve
kprompt "logs payment-api"
kprompt "describe payment-api"
kprompt "delete deployment redis" --approve
kprompt "list deployments"
kprompt "show pods" -n default
kprompt "explain why payment-api is crashing"
```

## Status

**v0.2.0+** — plan → safety → apply for **deploy** + **scale** + **rollback** + **named delete**; read-only **get/list** + **explain-lite** + **logs** + **describe**; `kprompt config`; multi-LLM presets; friendlier kube/RBAC errors. Kind E2E under `go test -tags=e2e ./test/e2e/`.

## Install

### From releases (recommended)

```bash
curl -fsSL https://kprompt-website.vercel.app/install | bash
```

(After `kprompt.ai` DNS is live, the same path will be `https://kprompt.ai/install` — see architecture `DOMAIN.md`.)

Fallback (pinned release script on jsDelivr):

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/kprompt/kprompt@v0.2.0/install/install.sh | bash
```

### From source (dev)

```bash
git clone https://github.com/kprompt/kprompt.git
cd kprompt
go install ./cmd/kprompt
```

Or build locally:

```bash
go build -o bin/kprompt ./cmd/kprompt
./bin/kprompt version
```

## Quick start

1. Point kubeconfig at a cluster (`~/.kube/config` or `KUBECONFIG`).
2. Set an LLM API key (pick a provider):

```bash
export KPROMPT_OPENAI_API_KEY=sk-...          # --provider openai (default)
export KPROMPT_ANTHROPIC_API_KEY=sk-ant-...   # --provider anthropic
export KPROMPT_GEMINI_API_KEY=...             # --provider gemini
export KPROMPT_GROQ_API_KEY=...               # --provider groq
# local: kprompt --provider ollama --model llama3.2 "..."
```

See [docs/providers.md](./docs/providers.md) for the full list.

3. Optional config at `~/.kprompt/config.yaml` (no secrets):

```bash
kprompt config
kprompt config set provider gemini
kprompt config set model gemini-2.0-flash
kprompt config set namespace default
```

Or edit YAML:

```yaml
provider: openai
model: gpt-4o-mini
```

4. Run a prompt (default is **plan only**; mutations ask `y/N` on a TTY, or use `--approve`):

```bash
kprompt "scale api to 10"
kprompt "scale api to 10" --approve
```

Destructive prompts (wipe cluster, delete everything, delete a namespace, …) are **hard-denied**.
Phrases like `in staging` or `on kind-kprompt-e2e context` resolve to namespace / kube context
(LLM + heuristics). Explicit `--namespace` / `--context` always win.

Cluster / kubeconfig failures print short actionable hints (missing config, bad context, RBAC deny, unreachable API) and point at the [Usage guide](https://kprompt-website.vercel.app/#usage) when helpful.

## Flags

| Flag | Description |
|------|-------------|
| `--approve` | Apply without interactive confirmation |
| `--wait` | After apply, wait for Deployment rollout |
| `--timeout` | Timeout for `--wait` (default `5m`) |
| `--provider` | `openai`, `anthropic`, `gemini`, `groq`, `mistral`, `deepseek`, `openrouter`, `together`, `ollama`, `openai-compatible` |
| `--model` | Model id |
| `--context` | kubeconfig context |
| `--namespace` / `-n` | Default namespace |

## Architecture

Pipeline: **Prompt → Intent → Plan → Safety → Approval → Executor → Kubernetes**.

Package layout matches the private architecture ADRs (`cmd/kprompt`, `internal/{config,llm,intent,planner,safety,executor,cluster,pipeline,ui}`).

## License

[MIT](./LICENSE) © 2026 Muhtalip Dede
