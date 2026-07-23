# kprompt

Open-source AI CLI to control Kubernetes with natural language.

> Talk to Your Cluster.

**Experimental.** Early software — always review the plan before apply, prefer non-production clusters first, and treat `--approve` with care. Safety hard-denies help; they do not make unattended production use safe.

```bash
kprompt "scale api to 10" --approve --wait
kprompt "deploy redis" --approve
kprompt "install redis" --approve   # Helm chart (requires helm on PATH)
kprompt "upgrade nginx to 1.3" --approve   # Helm upgrade (params.version from LLM)
kprompt "deploy nginx" --approve
kprompt "rollback payment-api" --approve
kprompt "logs payment-api"
kprompt "describe payment-api"
kprompt "delete deployment redis" --approve
kprompt "list deployments"
kprompt "show pods" -n default
kprompt "how many nodes are in the cluster"
kprompt "list configmaps" -n default
kprompt "get secret db-creds" -n prod
kprompt "explain why payment-api is crashing"
kprompt "why is my api slow?" -n production
kprompt "optimize my cluster"
kprompt "show service dependency graph"
kprompt "show gitops sync status"
kprompt "why is api slow then scale api to 4"
kprompt login
```

Generic get/list works for discoverable built-ins and CRDs (Node, ConfigMap, Secret, …). See [docs/kubernetes-reads.md](./docs/kubernetes-reads.md).

## Status

**v0.4.0 (experimental)** — Kubernetes plan → safety → apply for deploy, scale, rollback, and named delete; deep explain, logs, describe, history, JSON CI output, and terminal themes. Integrations: Helm, Argo Workflows, Prometheus, OpenTelemetry, Grafana, Tekton, KEDA, Istio (read-first), Crossplane, Flux/Argo CD GitOps, optimize reports, service graphs, multi-tool routes with one approval, Homebrew install, and optional Team login/policy/audit. See [docs/integration-matrix.md](./docs/integration-matrix.md) and [docs/kubernetes-reads.md](./docs/kubernetes-reads.md).

## Install

### From releases (recommended)

```bash
curl -fsSL https://kprompt.ai/install | bash
```

### Homebrew

```bash
brew install kprompt/tap/kprompt
```

Fallback (pinned release script on jsDelivr):

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/kprompt/kprompt@v0.4.0/install/install.sh | bash
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

# Cluster aliases (short name → kubeconfig context)
kprompt config alias set prod gke_myproj_us-central1_prod
kprompt config alias set staging kind-staging
kprompt --context prod "list deployments"
kprompt config set require_alias_match true   # refuse mutate unless kubectl current-context matches
kprompt tools   # detect Helm, Argo CRD, Prometheus/Grafana URLs (integration layer)
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
## History

```bash
kprompt history              # last 20 prompts/plans (~/.kprompt/history.jsonl)
kprompt history rerun        # replay newest prompt
kprompt history rerun 3 --approve
```

History stores prompt, kind, summary, and action refs — never manifests or API keys.

Use `kprompt doctor` after install to verify kubeconfig, LLM keys, integrations, and optional Team enrollment.

```bash
kprompt doctor           # kube + LLM key + tools + Team health (exit 1 if required fail)
kprompt doctor --json
kprompt dash             # local read-only cluster UI (requires kprompt-dash on PATH)
```

## Team enrollment (optional)

Opt-in control-plane login for org policy / audit (does not change Free CLI behavior until you enroll):

```bash
kprompt login            # device code → approve at app.kprompt.ai/connect
kprompt login --open     # also open the browser
kprompt whoami           # org + member
kprompt policy pull      # fetch org policy → ~/.kprompt/policy.yaml
kprompt policy           # show cached policy
kprompt secrets pull     # fetch org LLM keys → ~/.kprompt/provider-secrets.yaml (0600)
kprompt logout           # revoke token + clear credentials/policy/secrets
```

Override API with `KPROMPT_API_URL` / `KPROMPT_API_TOKEN` if needed. The `kp_…` token is stored only in `credentials.yaml` (0600), never in `config.yaml`. Cached org policy only **tightens** local hard-denies. Provider keys: env vars always win over pulled secrets. When enrolled, each plan also best-effort pushes an audit event (`planned` / `denied` / `applied`) to the control plane — disable with `KPROMPT_DISABLE_AUDIT=1`.

## CI

Use `--output json` for a versioned PlanResult (see [docs/ci.md](./docs/ci.md)).

Cluster / kubeconfig failures print short actionable hints (missing config, bad context, RBAC deny, unreachable API) and point at the [Usage guide](https://kprompt.ai/#usage) when helpful.

## Flags

| Flag | Description |
|------|-------------|
| `--approve` | Apply without interactive confirmation |
| `--wait` | After apply, wait for Deployment rollout |
| `--timeout` | Timeout for `--wait` (default `5m`) |
| `--output` / `-o` | `text` (default) or `json` (CI PlanResult) |
| `--provider` | `openai`, `anthropic`, `gemini`, `groq`, `mistral`, `deepseek`, `openrouter`, `together`, `ollama`, `openai-compatible` |
| `--model` | Model id |
| `--context` | kubeconfig context |
| `--namespace` / `-n` | Default namespace |

## Architecture

Pipeline: **Prompt → Intent → Plan → Safety → Approval → Executor → Kubernetes**.

Package layout matches the private architecture ADRs (`cmd/kprompt`, `internal/{config,llm,intent,planner,safety,executor,cluster,pipeline,ui}`).

## License

[Apache-2.0](./LICENSE) © 2026 Muhtalip Dede
