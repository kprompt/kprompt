# kprompt

Open-source AI CLI to control Kubernetes with natural language.

> Talk to Your Cluster.

```bash
kprompt "scale api to 10"
kprompt "deploy redis" --approve
```

## Status

**v0 skeleton** — plan → safety → approve → apply pipeline is in place. First mutation path: **scale a Deployment**. Multi-LLM providers: OpenAI-compatible + Anthropic.

## Install (dev)

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

Install script (release binaries) lives in [`install/install.sh`](./install/install.sh).

## Quick start

1. Point kubeconfig at a cluster (`~/.kube/config` or `KUBECONFIG`).
2. Set an LLM API key:

```bash
export KPROMPT_OPENAI_API_KEY=sk-...
# or
export KPROMPT_ANTHROPIC_API_KEY=sk-ant-...
```

3. Optional config at `~/.kprompt/config.yaml` (no secrets):

```yaml
provider: openai
model: gpt-4o-mini
```

4. Run a prompt (default is **plan only**):

```bash
kprompt "scale api to 10"
kprompt "scale api to 10" --approve
```

Destructive prompts (wipe cluster, delete everything, …) are **hard-denied**.

## Flags

| Flag | Description |
|------|-------------|
| `--approve` | Apply the plan after safety checks |
| `--provider` | `openai` or `anthropic` |
| `--model` | Model id |
| `--context` | kubeconfig context |
| `--namespace` / `-n` | Default namespace |

## Architecture

Pipeline: **Prompt → Intent → Plan → Safety → Approval → Executor → Kubernetes**.

Package layout matches the private architecture ADRs (`cmd/kprompt`, `internal/{config,llm,intent,planner,safety,executor,cluster,pipeline,ui}`).

## License

[MIT](./LICENSE) © 2026 Muhtalip Dede
