---
name: cli-feature
description: >-
  Adds or changes kprompt CLI commands, pipeline/plan/safety/apply paths, Observe
  agent packages, Helm/CRD agent charts, or docs under kprompt/. Use when implementing
  a new subcommand, mutating PlanResult flow, agent watch/analyze/notify, or updating
  CLI docs and tests.
---

# CLI feature workflow

## 1. Place the change

| Kind of work | Primary packages |
|--------------|------------------|
| NL plan / mutate | `internal/pipeline`, `planner`, `intent`, `safety`, `executor`, `verify` |
| LLM providers | `internal/llm` |
| Cluster I/O | `internal/cluster` |
| Observe / Namespace Agent | `internal/agent/*` |
| SRE reads | `investigate`, `why`, `timeline`, `impact`, `audit`, … |
| Config / UX | `internal/config`, `ui`, Cobra in `cmd/kprompt` |
| Ship agent | `charts/`, `deploy/crd`, `docs/agent*.md` |

Wire new user-facing commands through Cobra in `cmd/kprompt`. Reuse existing PlanResult / Investigation types instead of inventing parallel JSON shapes.

## 2. Safety checklist

- Mutates require plan + interactive approve or explicit `--approve`.
- Agent defaults: watch → correlate → analyze → notify; **no** auto-apply.
- Wipe / unbounded delete stays hard-denied (`internal/safety`).
- Multi-context mutates keep per-context approval semantics.

## 3. Tests & docs

1. Unit tests next to the package; stub LLM where CI must stay keyless.
2. Kind E2E only if the path needs a live apiserver (`test/e2e`, `-tags=e2e`).
3. Update matching `docs/*.md` and command `--help` text.
4. Run `go test` for touched packages (full `./...` before handoff).

## 4. Do not

- Add a required laptop daemon (one-binary CLI).
- Upload local history/remember data to the cloud without explicit product opt-in.
- Expand into website/app/api repos from this skill — stay in `kprompt/`.
