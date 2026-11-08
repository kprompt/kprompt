# PlanResult parity (CLI · JSON · proposals · Slack)

kprompt’s review contract is the same idea everywhere: **see what will change, see risk, then approve** — never silent mutate. Surfaces differ in density; this page is the field map (IC-005).

Canonical planning matrix: [PLANRESULT-MATRIX.md](https://github.com/kprompt/kprompt-architecture/blob/main/PLANRESULT-MATRIX.md) (private architecture repo mirror of this table).

## Surfaces

| Surface | Command / path | Artifact |
|---------|----------------|----------|
| CLI TTY | `kprompt "…"` | Printed plan + `Apply this plan? [y/N]` |
| JSON | `kprompt "…" -o json` | `kind: PlanResult` |
| Durable proposal | `agent run --autopilot-propose` → `agent proposals show` | `kind: AutopilotProposal` |
| Slack / Discord | Observe notify | `AgentAlert` + optional `proposalId` |
| Slack ask | `--slack-ask` | `approve <proposal-id>` |

## Field matrix

| Field | CLI TTY | JSON PlanResult | AutopilotProposal | Slack / Discord |
|-------|---------|-----------------|-------------------|-----------------|
| Actions / steps | Yes (ops + diff) | `plan.actions[]` | `plan.summary` + `plan.steps[]` | Alert summary / recommendation only |
| Risk | Yes | `risk.level` / `denied` | `risk` | `proposalRisk` when proposal attached |
| Blast / impact | Blast radius block | `blastRadius` optional | `expectedImpact` / `why` (text) | Not in alert body |
| Proposal id | N/A (laptop NL) | N/A | `id` | `proposalId` |
| Approve hint | `[y/N]` / `--approve` | `requiresApproval` | `proposals apply --approve` | `proposalHint` + ask `approve <id>` |
| Applied | Session | `applied` | `applied` + `decision` | Pre-apply alert |
| Verify / Learn | `--wait` / verify UX | `verify` optional | `verifyStatus` / `outcome` / `learnNote` | Not in alert |

## Approve vocabulary (next to CLI `y/N`)

| Context | How | Same as |
|---------|-----|---------|
| Interactive CLI | `Apply this plan? [y/N]` → `y` | — |
| CI / non-TTY | `--approve` ([approval.md](./approval.md)) | TTY `y` |
| Stored proposal | `kprompt agent proposals apply -n <ns> --id <id> --approve` | TTY `y` / `--approve` |
| Slack ask | `approve <proposal-id>` (alias: `apply <id>`) | `--approve` on that proposal |
| Slack/Discord alert | Shows CLI hint + (Slack) ask phrase | Points at the rows above |
| Discord | Notify only — use CLI hint | — |

Risk labels (all surfaces): `low` · `medium` · `high` · `denied`.

Slack/Discord alerts may also show a short **Impact** line from the proposal (`expectedImpact`). That is not a substitute for TTY blast-radius review.

## Honesty

- Slack is **not** a full TTY diff viewer. It carries **id + risk + apply hint** so an engineer who knows the CLI can approve without a glossary.
- AutopilotProposal is PlanResult-**shaped**, not a byte-identical `PlanResult` document.
- Default remains propose-only; `policyAuto` + allowlist + explicit approve are required to mutate.

## Related

- [ci.md](./ci.md) — JSON schema + jq gates  
- [ci-cookbook.md](./ci-cookbook.md) — copy-paste GitHub Action (IC-007, example only)  
- [approval.md](./approval.md) — full approve vocabulary  
- [agent.md](./agent.md) — Observe + proposals + Slack ask  
- [demo.md](./demo.md) — magic-moment path  
