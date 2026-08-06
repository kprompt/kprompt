# AGENTS.md — kprompt CLI

This repo is **[kprompt/kprompt](https://github.com/kprompt/kprompt)**: the public Go CLI and in-cluster Observe / Namespace Agent. Module: `github.com/kprompt/kprompt` (Go 1.23).

Architecture ADRs and task boards live in the private `kprompt-architecture` repo — do not invent conflicting product contracts here.

## Product DNA

- **The AI Runtime for Kubernetes** — not a kubectl chatbot.
- Pipeline: prompt → intent → **PlanResult** → safety → **approve** → apply → verify.
- Observe / Namespace Agent: **propose-first**; never silent mutate without Autopilot + policy.
- Hard-deny wipe-class prompts; `--approve` is explicit and dangerous — treat carefully in docs/demos.
- Honesty: label experimental / building; never claim “AI auto-fixes production.”

## Layout

| Path | Role |
|------|------|
| `cmd/kprompt` | CLI entry (Cobra) |
| `internal/pipeline` | Prompt → plan → safety → apply |
| `internal/planner`, `intent`, `safety`, `executor` | Plan / deny / mutate |
| `internal/llm` | Multi-provider LLM interface |
| `internal/cluster` | client-go cluster access |
| `internal/agent/*` | Observe / Namespace Agent / Coordinator |
| `internal/investigate`, `why`, `timeline`, … | AI SRE read surfaces |
| `charts/` | Helm (agent, operator, coordinator) |
| `deploy/crd` | CRDs |
| `docs/` | User + contributor docs |
| `test/e2e` | Kind E2E (`-tags=e2e`) |
| `ide/vscode` | VS Code extension (separate npm tree) |

## Build / test

```bash
go test ./...
go build -o bin/kprompt ./cmd/kprompt
./bin/kprompt version
./bin/kprompt doctor
```

Kind E2E (Docker + kind):

```bash
go test -tags=e2e ./test/e2e/ -count=1 -v -timeout 10m
```

CI mirrors: `go test ./...` then `go build -o bin/kprompt ./cmd/kprompt` (see `.github/workflows/ci.yml`).

Prefer the smallest package test while iterating; full `./...` before PR.

## Cursor rules (scoped)

| When editing | Rule |
|--------------|------|
| `internal/safety/**` | `.cursor/rules/safety.mdc` — hard-deny + corpus |
| `internal/agent/**` | `.cursor/rules/agent-observe.mdc` — propose-first Observe |
| `internal/{pipeline,planner,intent,executor,verify,pretrust}/**` | `.cursor/rules/pipeline.mdc` — plan → approve → apply |
| `charts/**`, `deploy/crd/**` | `.cursor/rules/helm-charts.mdc` — Observe defaults / RBAC |
| `internal/llm/**` | `.cursor/rules/llm.mdc` — BYOK / multi-provider / stub |

Always-on defaults: `.cursor/rules/project.mdc`.

Skills: `cli-feature` · `docs-sync` · `kind-e2e` (live apiserver tests).

Hook: `.cursor/hooks.json` → `afterFileEdit` runs `.cursor/hooks/gofmt.sh` on edited `*.go` files (fail-open).

## Working rules

1. One concern per change; match neighboring Go style (`gofmt`, `internal/` layout).
2. Behavior or flag changes → update matching `docs/*.md` (and help text); use `docs-sync`.
3. Mutating paths stay behind PlanResult + approval (ADR-0003 DNA).
4. Agent watch/analyze paths stay notify/propose unless Autopilot gates are explicit.
5. No secrets, API keys, or kubeconfigs in the tree.
6. Commit / PR only when the user asks.
7. For new CLI surfaces or pipeline/agent work, follow the `cli-feature` skill.
8. For kind coverage, follow the `kind-e2e` skill.

## Out of scope here

Marketing site, Team app/admin UI, control-plane API, and architecture markdown boards — other repos. Link or mention them; do not recreate them in this tree.
