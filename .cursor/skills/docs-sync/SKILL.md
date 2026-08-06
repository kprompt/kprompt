---
name: docs-sync
description: >-
  Keeps kprompt CLI docs and --help text aligned with code changes. Use when adding
  or changing flags, commands, PlanResult fields, agent Helm values, or when the
  user asks to update docs/, README examples, or approval/agent documentation.
---

# Docs sync workflow

## 1. Map code → docs

| Change area | Primary docs |
|-------------|--------------|
| Approval / `--approve` / PlanResult | `docs/approval.md`, `docs/architecture.md`, `docs/ci.md` |
| Observe / agent / Helm | `docs/agent.md`, `docs/namespace-agent.md`, `docs/agent-ops.md`, `docs/agent-fleet.md` |
| investigate / why / timeline / … | Matching `docs/<cmd>.md` |
| Providers / setup / doctor | `docs/providers.md`, `docs/setup.md`, `docs/doctor.md`, `docs/init.md` |
| Multi-cluster | `docs/multi-cluster.md` |
| Kind E2E | `docs/e2e.md` |
| Layout for contributors | `docs/dev.md`, `CONTRIBUTING.md` |

Also update Cobra `--help` / flag descriptions in `cmd/kprompt` when user-facing.

## 2. Honesty checklist

- Do not document Autopilot apply as default Observe behavior.
- Mark experimental / MVP surfaces clearly.
- Examples should show plan-before-apply (or read-only) unless demonstrating `--approve` with a warning.
- Keep README demos consistent with real flags after renames.

## 3. Finish

1. Diff docs against the code change; remove stale flag names.
2. Prefer short, command-shaped examples over essays.
3. No commit unless the user asks.
