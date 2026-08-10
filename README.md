<div align="center">

# kprompt

**The AI Runtime for Kubernetes** — observe, reason, plan safe actions, and execute only after approval.

[![CI](https://github.com/kprompt/kprompt/actions/workflows/ci.yml/badge.svg)](https://github.com/kprompt/kprompt/actions/workflows/ci.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=kprompt_kprompt&metric=alert_status)](https://sonarcloud.io/project/overview?id=kprompt_kprompt)
[![Release](https://img.shields.io/github/v/release/kprompt/kprompt?logo=github)](https://github.com/kprompt/kprompt/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/kprompt/kprompt/total?logo=github)](https://github.com/kprompt/kprompt/releases)
[![Go](https://img.shields.io/badge/go-1.23-00ADD8?logo=go&logoColor=white)](./go.mod)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](./LICENSE)
[![Stars](https://img.shields.io/github/stars/kprompt/kprompt?style=flat&logo=github)](https://github.com/kprompt/kprompt/stargazers)

<img src="./.github/assets/kprompt-mascot-run.gif" alt="kprompt mascot — Border Collie running" width="240" />

[Website](https://kprompt.ai) · [Docs](https://kprompt.ai/docs) · [Roadmap](https://kprompt.ai/docs/roadmap) · [Discussions](https://github.com/kprompt/kprompt/discussions) · [Blog](https://kprompt.ai/blog)

</div>

---

kprompt turns natural language into **reviewable Kubernetes actions**. Every mutation becomes a typed **plan** — actions, diff, risk, and blast radius — that you approve before anything touches the cluster. Destructive, wipe-class prompts are hard-denied outright.

```console
$ kprompt "delete everything in the cluster"
🚨 Intent: destructive cluster operation
🛡️ Safe execution: denied
😅 Named targets only — chaos needs a ticket

Next: kprompt "delete deployment <name>" -n <namespace>
```

![kprompt hard-denies wipe prompts, then turns scale into a reviewable plan waiting at Apply this plan? y/N](./.github/assets/kprompt-plan-deny.gif)

> [!WARNING]
> **Experimental software.** Always review the plan before apply, prefer non-production clusters first, and treat `--approve` with care. Safety hard-denies help — they do not make unattended production use safe.

kprompt is **open source (Apache-2.0)** and **free forever**. Natural-language plans run on **your** LLM — local [Ollama](https://ollama.com) (`$0`) or a cloud provider key you already own (BYOK). No cluster credentials are ever uploaded.

## Highlights

- 🧠 **Natural language → typed plan** — ask in plain English; get actions, diff, risk, and blast radius, not a raw `kubectl` guess.
- 🛡️ **Plan before apply** — mutations stop for review; wipe-class intents are hard-denied regardless of flags.
- 🔌 **Your LLM, your keys** — 13 supported providers including local Ollama (`$0`), major clouds (BYOK), and any OpenAI-compatible endpoint. No markup, no data sold.
- 🔎 **AI SRE built in** — `investigate`, `why`, `timeline`, and `impact` for real root-cause analysis.
- 👀 **Observe agent** — optional in-cluster watch → Incident → gated notify; propose-only, never a silent mutate.
- ⚙️ **CI-native** — `--output json` emits a versioned `PlanResult` for `jq` gates and GitOps PRs.
- 🧩 **Day-2 integrations** — Helm, Prometheus, OpenTelemetry, Grafana, Argo, and GitHub under one approval loop.

---

## Table of contents

- [Try it in 60 seconds](#try-it-in-60-seconds)
- [Why kprompt](#why-kprompt)
- [Providers & integrations](#providers--integrations)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Usage](#usage)
- [Safety model](#safety-model)
- [Observe agent](#observe-agent)
- [History & sessions](#history--sessions)
- [CI & PlanResult JSON](#ci--planresult-json)
- [Team (optional)](#team-optional)
- [CLI reference](#cli-reference)
- [Architecture](#architecture)
- [Documentation](#documentation)
- [Privacy & telemetry](#privacy--telemetry)
- [Contributing](#contributing)
- [License](#license)

---

## Try it in 60 seconds

Run the Observe agent demo on a local kind cluster — **`$0`, no provider key, no cloud**. The demo is heuristic and offline (zero LLM spend).

```bash
kprompt demo              # prints prerequisites + exact walkthrough commands
kprompt demo --check      # verify required tools are on PATH
```

Or set it up manually:

```bash
brew install kind kubectl
curl -fsSL https://kprompt.ai/install | bash

git clone https://github.com/kprompt/kprompt-examples.git
cd kprompt-examples && make walkthrough
```

Prefer one failure at a time? `make up && make break SCENARIO=01-crashloop && make agent`.

![Observe agent: watch → Incident → gated alert — propose only, never silent mutate](./.github/assets/kprompt-observe-demo.gif)

---

## Why kprompt

| Principle | What it means |
|-----------|---------------|
| **AI Runtime** | A full loop: observe → reason → plan → validate → approve → execute → learn |
| **Plan before apply** | Mutations become a typed `PlanResult`; approve via TTY `y/N` or `--approve`. Wipe-class intents are hard-denied |
| **Local & BYOK** | Your kubeconfig + your LLM. Ollama is `$0`; no cluster credentials leave your machine |
| **CI-ready** | `--output json` emits a versioned `PlanResult` for `jq` gates |
| **Day-2 native** | Helm, Prometheus, OTel, Grafana, and GitOps under one approval loop |
| **Observe agent** | Optional in-cluster watch → Incident → gated notify (propose-only, never a silent mutate) |

kprompt is an **AI Runtime for Kubernetes** — not a ChatGPT wrapper or free-form `kubectl` chat. It shares the natural-language day-2 lane with tools like [kubectl-ai](https://github.com/GoogleCloudPlatform/kubectl-ai), but enforces a different contract: **PlanResult → safety → approve → apply**.

Further reading: [AI Runtime for Kubernetes](https://kprompt.ai/blog/ai-runtime-for-kubernetes) · [Intent compiler, not chat](https://kprompt.ai/blog/intent-compiler-not-chat) · [AI SRE direction](https://kprompt.ai/blog/ai-sre-not-ai-kubectl) · [kprompt vs kubectl-ai](https://kprompt.ai/blog/kprompt-vs-kubectl-ai)

---

## Providers & integrations

kprompt is **bring-your-own-key**. Point it at local Ollama for `$0` inference, or any cloud provider you already pay for — kprompt never marks up tokens or sits between you and your model. See [docs/providers.md](./docs/providers.md).

### LLM providers

| | | | |
|--|--|--|--|
| **Ollama** (`$0`, local) | **OpenAI** | **Anthropic** | **Gemini** |
| **Groq** | **xAI** | **Cerebras** | **Mistral** |
| **DeepSeek** | **Moonshot** | **OpenRouter** | **Together** |
| **OpenAI-compatible** (any base URL) | | | |

### Kubernetes & day-2

| Integration | What kprompt does |
|-------------|-------------------|
| **Kubernetes** | Read/plan/apply against your kubeconfig (built-ins + CRDs) |
| **Helm** | Install / upgrade charts as reviewable plans (needs `helm` on PATH) |
| **Argo** | Argo Workflows plans and ArgoCD GitOps sync status |
| **Prometheus** | Performance reports and metric-backed reasoning |
| **OpenTelemetry** | Trace walks and bottleneck narration |
| **Grafana** | Dashboard summaries |
| **GitHub (GitOps)** | Open/update a PR instead of applying — `--gitops` |

### Notifications

| | |
|--|--|
| **Slack** (with approve bridge) | **Discord** |

---

## Installation

### From releases (recommended)

```bash
curl -fsSL https://kprompt.ai/install | bash
```

### Homebrew

```bash
brew install kprompt/tap/kprompt
```

### Pinned fallback (jsDelivr)

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/kprompt/kprompt@v0.10.0/install/install.sh | bash
```

### From source

```bash
git clone https://github.com/kprompt/kprompt.git
cd kprompt
go install ./cmd/kprompt        # or: go build -o bin/kprompt ./cmd/kprompt
kprompt version
```

### Shell completions

```bash
source <(kprompt completion bash)                                  # Bash
source <(kprompt completion zsh)                                   # Zsh
kprompt completion fish > ~/.config/fish/completions/kprompt.fish  # Fish
```

See `kprompt completion --help` to persist completions.

---

## Quick start

kprompt never sells you an API key. Natural-language plans need **your** LLM — local Ollama (`$0`) or a cloud provider key you already own. After install, a bare `kprompt` prints a readiness coach (kube / LLM / cluster). With no configured provider, the CLI is **unconfigured** and will not silently default to OpenAI.

**1. Point kubeconfig at a cluster** (`~/.kube/config` or `KUBECONFIG`).

**2. Configure a provider once** — Ollama first (`$0`), cloud BYOK only if you want it:

```bash
# A) Local Ollama — no cloud key, $0 inference
#    ollama serve && ollama pull llama3.2
kprompt init --ollama
kprompt "list pods"

# B) Optional: your own cloud provider key (BYOK)
kprompt init --provider openai
export KPROMPT_OPENAI_API_KEY=sk-...   # or anthropic / gemini / groq / xai / …
```

**3. Tune config** at `~/.kprompt/config.yaml` (no secrets stored here):

```bash
kprompt config                                  # show effective config
kprompt config set provider ollama
kprompt config set model llama3.2
kprompt config set namespace default
kprompt config set theme nord
kprompt theme preview                           # sample every palette

# Cluster aliases (short name → kubeconfig context)
kprompt config alias set prod gke_myproj_us-central1_prod
kprompt --context prod "list deployments"
kprompt config set require_alias_match true     # refuse mutate unless current-context matches
```

Or edit YAML directly:

```yaml
provider: ollama
model: llama3.2
```

**4. Run a prompt** (default is **plan only**; mutations ask `y/N` on a TTY, or use `--approve`):

```bash
kprompt "scale api to 10"
kprompt "scale api to 10" --approve
kprompt "add HPA for redis"
```

Run `kprompt doctor` anytime to verify kubeconfig, provider setup, integrations, and optional Team enrollment. See [docs/init.md](./docs/init.md) and [docs/providers.md](./docs/providers.md).

---

## Usage

kprompt groups prompts into three intents: **read**, **explain / root-cause**, and **mutate** (plan-gated).

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
kprompt "install redis" --approve              # Helm chart (needs helm on PATH)

# day-2
kprompt "how's my cluster"                     # health summary (no LLM key needed)
kprompt "optimize my cluster"
kprompt "show gitops sync status"
```

<details>
<summary><strong>Full prompt catalogue</strong></summary>

```bash
kprompt "deploy redis" --approve
kprompt "deploy nginx" --approve
kprompt "upgrade nginx to 1.3" --approve       # Helm upgrade
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
kprompt "cleanup unused resources" -n payments
kprompt "why is my api slow?" -n production
kprompt "show service dependency graph"
kprompt "why is api slow then scale api to 4"  # multi-tool chain, one approval
kprompt watch -n payments --once
kprompt remember "payment ns = Team A"
kprompt session
kprompt login
```

</details>

Generic get/list works for discoverable built-ins and CRDs (Node, ConfigMap, Secret, …) — see [docs/kubernetes-reads.md](./docs/kubernetes-reads.md).

---

## Safety model

The core contract is **Prompt → Intent → Plan → Safety → Approval → Executor → Kubernetes**.

- **Plan only by default.** Reads run immediately; mutations produce a typed `PlanResult` (actions, diff, risk, blast radius) and stop.
- **Approve explicitly.** Confirm interactively with `y/N` on a TTY, or pass `--approve` for non-interactive apply.
- **Hard-denied intents.** Wipe-class prompts (wipe cluster, delete everything, delete a namespace, …) are refused regardless of flags.
- **Optional post-apply verify.** `--wait` waits for the Deployment rollout, then verifies health.
- **Org policy can only tighten.** Team policy narrows local hard-denies; it can never waive wipe-class or namespace-delete denials.

See [docs/approval.md](./docs/approval.md) and [docs/safety](https://kprompt.ai/docs/safety).

---

## Observe agent

An optional in-cluster agent that watches workloads, correlates failures into **Incidents**, and sends **gated** notifications. It is **propose-only by default** — it never silently mutates the cluster.

```bash
kprompt agent run -n payments --health --heuristic     # local run (no LLM key)
kprompt agent proposals list                           # durable remediation proposals
kprompt agent proposals apply <id> --approve           # apply after review
```

Deploy in-cluster via Helm: [`charts/kprompt-agent`](./charts/kprompt-agent) · [`charts/kprompt-coordinator`](./charts/kprompt-coordinator). Guides: [docs/agent.md](./docs/agent.md) · [modes](./docs/namespace-agent.md) · [ops](./docs/agent-ops.md) · [coordinator](./docs/coordinator-knowledge.md).

---

## History & sessions

```bash
kprompt history                          # last 20 prompts/plans (~/.kprompt/history.jsonl)
kprompt history --namespace payments     # filter by namespace
kprompt history --kind deploy            # filter by intent/kind
kprompt history rerun                     # replay newest prompt
kprompt history rerun 3 --approve
kprompt history show 1                    # inspect one entry
kprompt history clear                     # clear with confirmation (--approve to skip)
```

History stores prompt, kind, summary, and action refs — **never manifests or API keys**. Filters are case-insensitive exact matches and can be combined.

Explore setup, the learn profile, and built-in recipes:

```bash
kprompt setup                                 # dry-run plan (platform profile)
kprompt setup --profile minimal --approve     # host Helm only
kprompt learn
kprompt recipe list
kprompt contexts --check                       # kubeconfig contexts + API reachability
kprompt dash                                   # local read-only cluster UI (needs kprompt-dash)
```

---

## CI & PlanResult JSON

Use `--output json` for a versioned `PlanResult` suitable for `jq` gates — no cluster apply required.

```bash
kprompt "scale api to 3" -n payments --output json | jq '.risk'
```

Multi-context reads and optimizes emit a `MultiContextResult`. Cluster/kubeconfig failures print short, actionable hints (missing config, bad context, RBAC deny, unreachable API).

See [docs/ci.md](./docs/ci.md) · [docs/multi-cluster.md](./docs/multi-cluster.md) · [PlanResult JSON deep dive](https://kprompt.ai/blog/planresult-json-deep-dive).

---

## Team (optional)

An opt-in control-plane login for org policy and audit. It does **not** change Free CLI behavior until you enroll.

- **Policy that only tightens** — org rules narrow local hard-denies (namespaces, max risk, change windows, approve-by-role); they can never loosen wipe-class safety.
- **Centralized secrets** — pull org LLM keys to `0600` files; local env vars always win.
- **Audit trail** — each plan best-effort pushes `planned` / `denied` / `applied` events; disable with `KPROMPT_DISABLE_AUDIT=1`.
- **Remote runs** — app `/run` jobs stay queued until `kprompt run listen` claims and plans them locally (never auto-apply).

```bash
kprompt login            # device code → approve at app.kprompt.ai/connect
kprompt whoami           # org + member
kprompt policy pull      # fetch org policy → ~/.kprompt/policy.yaml
kprompt secrets pull     # fetch org LLM keys → ~/.kprompt/provider-secrets.yaml (0600)
kprompt run listen       # claim app /run jobs; plan locally (never auto-apply)
kprompt logout           # revoke token + clear local credentials
```

The `kp_…` token is stored only in `credentials.yaml` (0600), never in `config.yaml`. Cached org policy **only tightens** local hard-denies (namespaces, max risk, deny intents, change windows, approve-by-role, context aliases). Provider env vars always win over pulled secrets. Override the API with `KPROMPT_API_URL` / `KPROMPT_API_TOKEN`; disable audit push with `KPROMPT_DISABLE_AUDIT=1`. Full walkthrough: [docs/runs.md](./docs/runs.md).

---

## CLI reference

| Flag | Description |
|------|-------------|
| `--approve` | Apply without interactive confirmation |
| `--approve-each-context` | Apply a mutating plan to every `--contexts` entry (explicit; not implied by `--approve`) |
| `--wait` | After apply, wait for Deployment / StatefulSet / DaemonSet rollout, then verify |
| `--timeout` | Timeout for `--wait` (default `5m`) |
| `--output` / `-o` | `text` (default) or `json` (CI PlanResult) |
| `--provider` | `openai`, `anthropic`, `gemini`, `groq`, `xai`, `cerebras`, `mistral`, `deepseek`, `moonshot`, `openrouter`, `together`, `ollama`, `openai-compatible` |
| `--model` | Model id |
| `--context` | kubeconfig context |
| `--contexts` | Comma-separated contexts for read fan-out / per-context mutate |
| `--namespace` / `-n` | Default namespace |
| `--theme` | Output theme (`auto`, `dracula`, `gruvbox`, `mono`, `nord`, `none`) |
| `--gitops` | Open/update a GitHub PR instead of applying to the cluster (requires `gitops.repo`) |
| `--gitops-repo` | GitHub `owner/name` for `--gitops` (or `gitops.repo` / `KPROMPT_GITOPS_REPO`) |
| `--gitops-path` | Path prefix inside the repo for PR files (default `kprompt`) |
| `--gitops-base-branch` | PR base branch (default `main`) |

GitOps PR mode example — open a PR instead of applying:

```bash
kprompt "deploy redis" -n demo --gitops --gitops-repo acme/infra --approve
```

See [docs/gitops-pr.md](./docs/gitops-pr.md).

---

## Architecture

```
Prompt → Intent → Plan → Safety → Approval → Executor → Kubernetes
```

The package layout mirrors the architecture ADRs: `cmd/kprompt` and `internal/{config,llm,intent,planner,safety,executor,cluster,pipeline,ui}`.

---

## Documentation

Full docs live at [kprompt.ai/docs](https://kprompt.ai/docs). Source guides in this repo:

| Area | Guides |
|------|--------|
| **Getting started** | [setup](./docs/setup.md) · [init](./docs/init.md) · [demo](./docs/demo.md) · [doctor](./docs/doctor.md) · [providers](./docs/providers.md) · [tools](./docs/tools.md) |
| **Investigate & RCA** | [investigate](./docs/investigate.md) · [investigation-graph](./docs/investigation-graph.md) · [why](./docs/why.md) · [timeline](./docs/timeline.md) · [impact](./docs/impact.md) · [reality-anchors](./docs/reality-anchors.md) |
| **Knowledge & analysis** | [graph](./docs/graph.md) · [simulation](./docs/simulation.md) · [search](./docs/search.md) · [score](./docs/score.md) · [architecture](./docs/architecture.md) |
| **Hygiene & day-2** | [audit](./docs/audit.md) · [cleanup](./docs/cleanup.md) · [optimize](./docs/optimize.md) · [drift](./docs/drift.md) · [gitops-pr](./docs/gitops-pr.md) · [recipes](./docs/recipes.md) |
| **Workflow** | [approval](./docs/approval.md) · [history](./docs/history.md) · [watch](./docs/watch.md) · [remember](./docs/remember.md) · [session](./docs/session.md) · [learn](./docs/learn.md) · [ide](./docs/ide.md) · [mcp](./docs/mcp.md) |
| **Observe agent** | [agent](./docs/agent.md) · [namespace-agent](./docs/namespace-agent.md) · [agent-ops](./docs/agent-ops.md) · [coordinator-knowledge](./docs/coordinator-knowledge.md) |
| **Fleet & CI** | [ci](./docs/ci.md) · [multi-cluster](./docs/multi-cluster.md) · [runs](./docs/runs.md) · [kubernetes-reads](./docs/kubernetes-reads.md) |

---

## Privacy & telemetry

The Free CLI does **not** phone home. Adoption signals stay public and passive:

- **Install volume** — public [download badges](https://github.com/kprompt/kprompt/releases) (curl and Homebrew both count as release asset downloads).
- **Stars** — public star count.
- **Prompt runs** — local only (`~/.kprompt/history.jsonl`); Team audit only after `kprompt login`.

Downloads are not unique users (re-installs, CI, checksums all count). Cluster data and prompts go only to the LLM provider you choose.

---

## Contributing

Contributions are welcome. Start with [CONTRIBUTING.md](./CONTRIBUTING.md) and the [`good first issue`](https://github.com/kprompt/kprompt/labels/good%20first%20issue) label. Questions, demos, and roadmap ideas belong in [Discussions](https://github.com/kprompt/kprompt/discussions).

If the plan-before-apply contract is useful in your own workflow, a ⭐ helps other SREs find it.

[![Contributors](https://contrib.rocks/image?repo=kprompt/kprompt)](https://github.com/kprompt/kprompt/graphs/contributors)

**Status:** `v0.10.0` (experimental) — the full AI Runtime loop (Observe → Reason → Plan → Validate → Approve → Execute → **Learn**) now runs in-cluster, not only on a laptop. Autopilot stays propose-only by default. See [CHANGELOG.md](./CHANGELOG.md).

---

## License

[Apache-2.0](./LICENSE) © 2026 Muhtalip Dede

<div align="center">

[Website](https://kprompt.ai) · [X @kpromptai](https://x.com/kpromptai) · [LinkedIn](https://www.linkedin.com/company/kprompt) · [YouTube](https://www.youtube.com/@kprompt-ai) · [Instagram](https://www.instagram.com/kprompt.ai) · [Bluesky](https://bsky.app/profile/kprompt.bsky.social) · [hello@kprompt.ai](mailto:hello@kprompt.ai)

</div>
