# kprompt

**The AI Runtime for Kubernetes** — observe, reason, plan safe actions, and execute only after approval.

[![CI](https://github.com/kprompt/kprompt/actions/workflows/ci.yml/badge.svg)](https://github.com/kprompt/kprompt/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/kprompt/kprompt?logo=github)](https://github.com/kprompt/kprompt/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/kprompt/kprompt/total?logo=github)](https://github.com/kprompt/kprompt/releases)
[![Downloads (latest)](https://img.shields.io/github/downloads/kprompt/kprompt/latest/total)](https://github.com/kprompt/kprompt/releases/latest)
[![Go](https://img.shields.io/badge/go-1.23-00ADD8?logo=go&logoColor=white)](./go.mod)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](./LICENSE)
[![Stars](https://img.shields.io/github/stars/kprompt/kprompt?style=flat&logo=github)](https://github.com/kprompt/kprompt/stargazers)

> The AI Runtime for Kubernetes.

![kprompt hard-denies wipe prompts, then turns scale into a reviewable plan waiting at Apply this plan? y/N](./.github/assets/kprompt-plan-deny.gif)

Nothing touched the cluster on the wipe — that is the point. Real mutates become a **typed plan you review first** — actions, diff, risk, blast radius — and the prompts that should never compile, don't:

```console
$ kprompt "delete everything in the cluster"
🚨 Intent: destructive cluster operation
🛡️ Safe execution: denied
😅 Nice try. Named targets only — chaos needs a ticket

Next: kprompt "delete deployment <name>" -n <namespace>
```

Wipe-class denials rotate a small **flavor pack** of punchlines (stable per prompt) — same safety, less boring stdout — and always end with a **Next:** remediation line.

Static still of a scale plan: [plan-demo.svg](./.github/assets/plan-demo.svg).
Open source (Apache-2.0). **Experimental.** Always review the plan before apply, prefer non-production first, and treat `--approve` with care. Safety hard-denies help; they do not make unattended production use safe.

## Try it in 60 seconds — $0, no provider key, no cloud

Observe agent demo (heuristic, offline, zero LLM spend). From any machine with Docker/kind:

```bash
kprompt demo              # prerequisites + exact walkthrough commands
kprompt demo --check      # PATH tools only
```

Or clone manually:

```bash
brew install kind kubectl
curl -fsSL https://kprompt.ai/install | bash

git clone https://github.com/kprompt/kprompt-examples.git
cd kprompt-examples && make walkthrough
```

Prefer one failure at a time? `make up && make break SCENARIO=01-crashloop && make agent`.

![Observe agent: watch → Incident → gated alert — propose only, never silent mutate](./.github/assets/kprompt-observe-demo.gif)

If the plan-before-apply contract is what you want in your own cluster workflow, a ⭐ helps other SREs find it.

Questions, demos, and roadmap ideas: **[Discussions](https://github.com/kprompt/kprompt/discussions)**. First PR? See [CONTRIBUTING.md](./CONTRIBUTING.md) and [`good first issue`](https://github.com/kprompt/kprompt/labels/good%20first%20issue).

## Why kprompt

| | |
|--|--|
| **AI Runtime** | Observe → reason → plan → validate → approve → execute → learn |
| **Plan before apply** | TTY `y/N` or explicit `--approve`; wipe-class denied |
| **CI-ready** | `--output json` PlanResult for jq gates |
| **Day-2 stack** | Helm, Prom, OTel, Grafana, GitOps… under one approval loop |
| **Local BYOK / Ollama** | Your kubeconfig + your LLM (Ollama = $0) — no cluster creds uploaded |
| **Observe agent** | Optional in-cluster watch → Incident → gated notify (no silent mutate) |

Category: **AI Runtime for Kubernetes** — not a ChatGPT wrapper or free-form kubectl chat. Same NL-CLI lane as [kubectl-ai](https://github.com/GoogleCloudPlatform/kubectl-ai) for day-2 mutate; different contract: **PlanResult → safety → approve → apply**. See [kprompt vs kubectl-ai](https://kprompt.ai/blog/kprompt-vs-kubectl-ai).

Longer positioning: [AI Runtime for Kubernetes](https://kprompt.ai/blog/ai-runtime-for-kubernetes) · [intent compiler, not chat](https://kprompt.ai/blog/intent-compiler-not-chat) · [AI SRE direction](https://kprompt.ai/blog/ai-sre-not-ai-kubectl) · [Roadmap](https://kprompt.ai/docs/roadmap)

## What you can ask

```bash
# read
kprompt "show pods" -n payments
kprompt "how many nodes are in the cluster"

# explain / root-cause
kprompt "explain why payment-api is crashing"
kprompt "why is ledger Pending" -n payments
kprompt "investigate api" -n payments
kprompt "who consumes redis" -n payments

# mutate — plan first, approve second
kprompt "scale api to 10" --approve --wait
kprompt "rollback payment-api" --approve
kprompt "install redis" --approve            # Helm chart (needs helm on PATH)

# day-2
kprompt "how's my cluster"           # witty health roast (no LLM key)
kprompt "optimize my cluster"
kprompt "show gitops sync status"
kprompt agent run -n payments --health --heuristic   # Observe agent (local)
```

<details>
<summary>Full prompt catalogue</summary>

```bash
kprompt "deploy redis" --approve
kprompt "deploy nginx" --approve
kprompt "upgrade nginx to 1.3" --approve   # Helm upgrade (params.version from LLM)
kprompt "delete deployment redis" --approve
kprompt "logs payment-api"
kprompt "describe payment-api"
kprompt "list deployments"
kprompt "list configmaps" -n default
kprompt "get secret db-creds" -n prod
kprompt "timeline for api" -n payments
kprompt "impact of deployment checkout" -n payments
kprompt "audit payments namespace" -n payments
kprompt "find every Deployment using redis" -n payments
kprompt "score payments namespace" -n payments
kprompt "explain architecture" -n payments
kprompt watch -n payments --once
kprompt remember "payment ns = Team A"
kprompt session
kprompt "how's my cluster"
kprompt "roast my namespace" -n payments
kprompt "cleanup unused resources" -n payments
kprompt "why is my api slow?" -n production
kprompt "show service dependency graph"
kprompt "why is api slow then scale api to 4"   # multi-tool chain, one approval
kprompt login
```

</details>

Multi-hop RCA: [docs/investigate.md](./docs/investigate.md) · Investigation Graph: [docs/investigation-graph.md](./docs/investigation-graph.md) · Reality anchors: [docs/reality-anchors.md](./docs/reality-anchors.md) · Causal why: [docs/why.md](./docs/why.md) · Timeline: [docs/timeline.md](./docs/timeline.md) · Reverse dependencies: [docs/impact.md](./docs/impact.md) · Knowledge Graph MVP: [docs/graph.md](./docs/graph.md) · Simulation MVP: [docs/simulation.md](./docs/simulation.md) · Security hygiene: [docs/audit.md](./docs/audit.md) · Cleanup: [docs/cleanup.md](./docs/cleanup.md) · Inventory search: [docs/search.md](./docs/search.md) · Scorecard: [docs/score.md](./docs/score.md) · Architecture narrative: [docs/architecture.md](./docs/architecture.md) · Watch: [docs/watch.md](./docs/watch.md) · Remember: [docs/remember.md](./docs/remember.md) · Session: [docs/session.md](./docs/session.md) · Learn profile: [docs/learn.md](./docs/learn.md) · Drift: [docs/drift.md](./docs/drift.md) · GitOps PR: [docs/gitops-pr.md](./docs/gitops-pr.md) · Recipes: [docs/recipes.md](./docs/recipes.md) · Cost Intelligence / Optimize: [docs/optimize.md](./docs/optimize.md) · Setup: [docs/setup.md](./docs/setup.md) · Init: [docs/init.md](./docs/init.md) · Demo: [docs/demo.md](./docs/demo.md) · Doctor: [docs/doctor.md](./docs/doctor.md) · Approval: [docs/approval.md](./docs/approval.md) · Tools: [docs/tools.md](./docs/tools.md) · History: [docs/history.md](./docs/history.md) · IDE PlanResult: [docs/ide.md](./docs/ide.md).
In-cluster Observe agent (Helm): [docs/agent.md](./docs/agent.md) · modes: [docs/namespace-agent.md](./docs/namespace-agent.md) · ops: [docs/agent-ops.md](./docs/agent-ops.md) · [`charts/kprompt-agent`](./charts/kprompt-agent) · Coordinator: [`charts/kprompt-coordinator`](./charts/kprompt-coordinator).

Generic get/list works for discoverable built-ins and CRDs (Node, ConfigMap, Secret, …). See [docs/kubernetes-reads.md](./docs/kubernetes-reads.md).

## Status

**v0.9.0 (experimental)** — Day-0 onboarding (`init` · `demo` · coach · help groups), plus Cerebras and Discord notify. Builds on search/score/architecture and laptop `watch` · `remember` · `session`. Autopilot stays **propose-only by default**. See [CHANGELOG.md](./CHANGELOG.md) · [docs/init.md](./docs/init.md).

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
curl -fsSL https://cdn.jsdelivr.net/gh/kprompt/kprompt@v0.9.0/install/install.sh | bash
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

### Shell autocompletion

To enable autocompletions for your shell:

```bash
# Bash
source <(kprompt completion bash)

# Zsh
source <(kprompt completion zsh)

# Fish
kprompt completion fish > ~/.config/fish/completions/kprompt.fish
```

See `kprompt completion --help` for details on how to persist completions.

## Transparent metrics

The Free CLI does **not** phone home. Adoption signals stay public and passive:

- **Install volume** — live [download badges](https://github.com/kprompt/kprompt/releases) above (curl install and Homebrew both count as release asset downloads)
- **Stars** — badge above
- **Prompt runs / setup** — local only (`~/.kprompt/history.jsonl`); Team audit only after `kprompt login`

Downloads are not unique users (re-installs, CI, checksums). Site `/install` fetches and GA4 stay maintainer-side.

## Quick start

No kprompt purchase — the Free CLI never sells you an API key. Natural-language plans need **your** LLM (local Ollama = **$0**, or a cloud provider key you already own). Prefer the [60-second walkthrough](#try-it-in-60-seconds--0-no-provider-key-no-cloud) first if you only want the Observe demo.

After install, bare `kprompt` prints a readiness coach (kube / LLM / cluster). With no `provider` in config and no `--provider` flag, the CLI is **unconfigured** (does not silently default to OpenAI).

1. Point kubeconfig at a cluster (`~/.kube/config` or `KUBECONFIG`).
2. Configure once — **Ollama first ($0)**, cloud BYOK only if you want it:

```bash
# A) Local Ollama — no cloud key, $0 inference
#    ollama serve && ollama pull llama3.2
kprompt init --ollama
kprompt "list pods"

# B) Optional: your LLM provider key (BYOK — not a kprompt product)
kprompt init --provider openai
export KPROMPT_OPENAI_API_KEY=sk-...          # or anthropic / gemini / groq / xai / …
```

See [docs/init.md](./docs/init.md) and [docs/providers.md](./docs/providers.md). Optional Team login (`kprompt login` → `kp_…` token) is org policy/audit only — unrelated to LLM keys.

3. Optional config at `~/.kprompt/config.yaml` (no secrets). CLI history/Team files also live under `~/.kprompt/`; Observe local stores use `~/.config/kprompt/` — see [docs/agent.md](./docs/agent.md#where-files-live).

```bash
kprompt config
kprompt config set provider ollama
kprompt config set model llama3.2
# or: gemini / openai / … after exporting that provider's key
kprompt config set namespace default
kprompt config set theme nord
kprompt theme preview   # sample every palette in the terminal

# Cluster aliases (short name → kubeconfig context)
kprompt config alias set prod gke_myproj_us-central1_prod
kprompt config alias set staging kind-staging
kprompt --context prod "list deployments"
kprompt config set require_alias_match true   # refuse mutate unless kubectl current-context matches
kprompt tools   # detect Helm, Argo CRD, Prometheus/Grafana URLs (integration layer)
```

Or edit YAML:

```yaml
provider: ollama
model: llama3.2
```

4. Run a prompt (default is **plan only**; mutations ask `y/N` on a TTY, or use `--approve`):

```bash
kprompt "scale api to 10"
kprompt "scale api to 10" --approve
kprompt "add HPA for redis"
```

Destructive prompts (wipe cluster, delete everything, delete a namespace, …) are **hard-denied**.

## History

```bash
kprompt history                          # last 20 prompts/plans (~/.kprompt/history.jsonl)
kprompt history --namespace payments     # filter by namespace
kprompt history --kind deploy             # filter by intent/kind string
kprompt history rerun                     # replay newest prompt
kprompt history rerun 3 --approve
kprompt history show 1                    # inspect one entry
kprompt history clear                     # clear history with confirmation
kprompt history clear --approve           # clear without prompting (--yes alias ok)
```

History stores prompt, kind, summary, and action refs — never manifests or API keys.
Filters are case-insensitive exact matches, and `--namespace` and `--kind` can be combined.

Explore setup, the learn profile, and built-in recipes:

```bash
kprompt setup                                 # dry-run plan (platform profile)
kprompt setup --profile minimal --approve     # host Helm only
kprompt setup --only prometheus --approve     # filter within profile
kprompt learn
kprompt recipe list
```

Use `kprompt doctor` after install to verify kubeconfig, LLM provider setup (Ollama or BYOK), integrations, and optional Team enrollment.

```bash
kprompt doctor           # kube + LLM key + tools + Team health (exit 1 if required fail)
kprompt doctor --json
kprompt contexts         # kubeconfig contexts + aliases
kprompt contexts --check # also probe API reachability
kprompt --contexts staging,prod "list deployments"   # read-only fan-out
kprompt --contexts staging,prod "optimize my cluster"  # fleet optimize rollup
kprompt "list pods across staging and prod"
# multi-context mutate: confirm each context (or --approve-each-context; never plain --approve)
kprompt --contexts staging,prod "scale api to 3"
kprompt dash             # local read-only cluster UI (requires kprompt-dash on PATH; see docs/dash.md)
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
kprompt run listen       # CLI bridge: claim app /run jobs; plan locally (never auto-apply)
# Full walkthrough: docs/runs.md · https://kprompt.ai/docs/runs
kprompt logout           # revoke token + clear credentials/policy/secrets
```

Override API with `KPROMPT_API_URL` / `KPROMPT_API_TOKEN` if needed. The `kp_…` token is stored only in `credentials.yaml` (0600), never in `config.yaml`. Cached org policy only **tightens** local hard-denies (namespaces, max risk, deny intents, optional **change windows**, **approve-by-role**, and **context aliases** / org `require_alias_match`). Provider keys: env vars always win over pulled secrets. When enrolled, each plan also best-effort pushes an audit event (`planned` / `denied` / `applied`) to the control plane — disable with `KPROMPT_DISABLE_AUDIT=1`. App `/run` jobs stay **queued** until `kprompt run listen` claims them — see [docs/runs.md](./docs/runs.md).

## CI

Use `--output json` for a versioned PlanResult (see [docs/ci.md](./docs/ci.md)). Multi-context reads/optimizes emit `MultiContextResult` (see [docs/multi-cluster.md](./docs/multi-cluster.md)).

This repo’s Actions can optionally upload a stub PlanResult to Team (`KPROMPT_ORG_TOKEN` secret) — see [docs/ci.md § Team org ingest](./docs/ci.md#team-org-ingest-this-repos-actions). No cluster apply.

Cluster / kubeconfig failures print short actionable hints (missing config, bad context, RBAC deny, unreachable API) and point at the [Usage guide](https://kprompt.ai/#usage) when helpful.

## Flags

See also: [GitHub Integration MVP / GitOps PR mode](./docs/gitops-pr.md). Example: `kprompt "deploy redis" -n demo --gitops --gitops-repo acme/infra --approve`

| Flag | Description |
|------|-------------|
| `--approve` | Apply without interactive confirmation |
| `--approve-each-context` | Apply a mutating plan to every `--contexts` entry (explicit; not implied by `--approve`) |
| `--wait` | After apply, wait for Deployment rollout, then verify |
| `--timeout` | Timeout for `--wait` (default `5m`) |
| `--output` / `-o` | `text` (default) or `json` (CI PlanResult) |
| `--provider` | `openai`, `anthropic`, `gemini`, `groq`, `xai`, `cerebras`, `mistral`, `deepseek`, `moonshot`, `openrouter`, `together`, `ollama`, `openai-compatible` |
| `--model` | Model id |
| `--context` | kubeconfig context |
| `--contexts` | Comma-separated contexts for read fan-out / per-context mutate |
| `--namespace` / `-n` | Default namespace |
| `--theme` | Output theme (`auto`, `dracula`, `gruvbox`, `mono`, `nord`, `none`) |
| `--gitops` | Open/update a GitHub PR instead of applying to the cluster (T-072; requires `gitops.repo`) |
| `--gitops-repo` | GitHub `owner/name` for `--gitops` (or config `gitops.repo` / `KPROMPT_GITOPS_REPO`) |
| `--gitops-path` | Path prefix inside the repo for PR files (default `kprompt`) |
| `--gitops-base-branch` | PR base branch (default `main`) |

## Architecture

Pipeline: **Prompt → Intent → Plan → Safety → Approval → Executor → Kubernetes**.

Package layout matches the private architecture ADRs (`cmd/kprompt`, `internal/{config,llm,intent,planner,safety,executor,cluster,pipeline,ui}`).

## Docs & guides

| | |
|--|--|
| Site | [kprompt.ai](https://kprompt.ai) · [Docs](https://kprompt.ai/docs) · [Roadmap](https://kprompt.ai/docs/roadmap) |
| Community | [Discussions](https://github.com/kprompt/kprompt/discussions) · [Contributing](./CONTRIBUTING.md) · [Good first issues](https://github.com/kprompt/kprompt/labels/good%20first%20issue) |
| Social | [X @kpromptai](https://x.com/kpromptai) · [LinkedIn](https://www.linkedin.com/company/kprompt) · [Bluesky](https://bsky.app/profile/kprompt.bsky.social) · [hello@kprompt.ai](mailto:hello@kprompt.ai) |
| Compare | [vs kubectl-ai](https://kprompt.ai/blog/kprompt-vs-kubectl-ai) · [AI tools map](https://kprompt.ai/blog/kubernetes-ai-tools-comparison) · [kubectl vs K9s](https://kprompt.ai/blog/kubectl-vs-k9s) |
| Product | [audit](./docs/audit.md) · [impact](./docs/impact.md) · [themes](./docs/theme.md) · [init](./docs/init.md) · [doctor](./docs/doctor.md) · [tools](./docs/tools.md) · [history](./docs/history.md) · [IDE PlanResult](./docs/ide.md) · [optimize my cluster](https://kprompt.ai/blog/optimize-my-cluster) · [PlanResult JSON](https://kprompt.ai/blog/planresult-json-deep-dive) · [AI SRE](https://kprompt.ai/blog/ai-sre-not-ai-kubectl) |

## Contributors

Thanks to everyone who has contributed to kprompt:

[![Contributors](https://contrib.rocks/image?repo=kprompt/kprompt)](https://github.com/kprompt/kprompt/graphs/contributors)

## License

[Apache-2.0](./LICENSE) © 2026 Muhtalip Dede
