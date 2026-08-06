# Reality anchors

**Contract ([S-020](https://github.com/kprompt/kprompt-architecture/issues/212) · [AG-070](https://github.com/kprompt/kprompt-architecture/issues/216)):** the Investigation Graph is only as honest as the nodes that **refuse to move**.

An *anchor* is evidence or policy the LLM / Autopilot optimizer **cannot invent, weaken, or waive**. Soft “looks good” in the same chat is not an anchor.

Related: [investigation-graph.md](./investigation-graph.md) · [ci.md](./ci.md) · ADR-0003 · ADR-0014 · ADR-0015.

---

## Registry

| Anchor | What it freezes | Who may change it | LLM / agent may… |
|--------|-----------------|-------------------|------------------|
| **Hard deny / safety policy** | Wipe-class, delete-namespace, cluster destroy intents | Humans via `internal/safety` + ADR-0003; optional org deny lists | Never waive; denied plans do not apply |
| **Generated command boundary** | Helm/GitOps host exec argv + Argo/Tekton generated command interpolation | Humans via `internal/tools/{helm,argo,tekton}` + setup allowlist rules | Keep argv slice-only host exec; reject `/bin/sh -c` launcher in workflow params; quote injected script literals. Residual risk: intentional container args still run in-cluster after explicit approval |
| **CLI pre-trust verify (T-089)** | EvidenceRef + optional re-read before high confidence / approve UX | Humans via `internal/pretrust` | Never raise confidence; soft-agree caps at 0.4 |
| **PlanResult schema** | `apiVersion` / `kind` / `schemaVersion` / risk / actions | Humans via product schema bumps | Emit into schema; not redefine fields mid-run |
| **Investigation / InvestigationReport schema** | Findings, EvidenceRef, Unknowns, confidence | Humans via ADR-0014 / schemaVersion | Fill fields; not drop Unknowns to fake certainty |
| **EvidenceRef kinds** | event / object / metric / trace / gitops — cluster-backed pointers | Humans via `internal/incident` | Cite refs; not fabricate Prom/OTel/GitOps when degraded |
| **Post-apply verify (T-070)** | `verify.ok` / failed / pending after apply | Humans via executor verify hooks | Not mark applied goals “done” without the check |
| **Coordinator probe Evidence** | `Source: coordinator-kube-probe` (+ honest probe Unknowns) | Probe code + AG-068 Merge caps | Not raise cross-ns confidence via narrative soft-agree |
| **RemediationPolicy allowlist** | Which Autopilot actions exist; `proposeOnly` vs `policyAuto` | Humans via policy JSON / ConfigMap (AG-040) | Propose only within allowlist |
| **Deny pack (AG-044)** | Wipe / ns delete / Secret values / fabricated evidence | Humans via Autopilot deny code | Never allowlist these |
| **Memory as evidence-not-proof (AG-034)** | Patterns/memory boost explainability only | Humans via confidence caps | Not treat memory alone as root-cause proof |
| **Probe / ns RBAC defaults** | Role-scoped ns agents; Coordinator SA-only; optional probe Roles | Humans via Helm / AG-039 · AG-069 | Not invent ClusterRole god-mode |

---

## Who owns truth

```text
  Model proposes ──► IR (Investigation / PlanResult)
                         │
                         ▼
              Anchors refuse or stamp
              (safety · schema · Evidence · verify)
                         │
                         ▼
              Human / CI approve mutate
                         │
                         ▼
              Apply ──► post-apply verify anchor
```

If an improvement loop can rewrite both the solution **and** the evaluator, Goodhart wins. Keep anchors in **code and schemas**, not in prompts.

---

## CLI vs agent

| Surface | Primary anchors |
|---------|-----------------|
| **CLI** | Hard deny, PlanResult, Investigation JSON, blastRadius, T-070 verify, CI `risk.denied` |
| **Namespace Agent** | EvidenceRef + degraded honesty, AG-034 memory cap, Role RBAC, handoff Unknowns |
| **Coordinator** | AG-068 probe Evidence / Unknowns, `mutateAttempted=false`, optional probe RBAC |
| **Autopilot** | RemediationPolicy + AG-044 deny pack + explicit approve / `--autopilot-apply` |

Pre-trust CLI hooks that re-read before high confidence ([T-089](https://github.com/kprompt/kprompt-architecture/issues/217) · `internal/pretrust`) are **shipped** — high confidence without EvidenceRef or a contradicting re-read is capped; suggested approve UX is withheld until anchors pass.

---

## Explicit non-goals

- Letting Autopilot or an LLM edit hard-deny regexes / deny packs at runtime
- “Second LLM in the same session” as a substitute for EvidenceRef or T-070
- Waiving `degraded[]` / Unknowns because the model sounds sure
- Uploading raw cluster dumps to `api.kprompt.ai` as a fake authority

---

## See also

- Investigation Graph: [investigation-graph.md](./investigation-graph.md)
- CI PlanResult gate: [ci.md](./ci.md)
- Autopilot / deny pack: [agent.md](./agent.md#autopilot-ag-017--ag-040ag-044--adr-0015)
- Worker isolation: [agent-ops.md](./agent-ops.md#worker-isolation-ag-069)
- Architecture: [SRE-TASKS S-020](https://github.com/kprompt/kprompt-architecture/blob/main/SRE-TASKS.md) · [AGENT-TASKS AG-070](https://github.com/kprompt/kprompt-architecture/blob/main/AGENT-TASKS.md)
