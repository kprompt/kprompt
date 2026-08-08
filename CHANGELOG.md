# Changelog

All notable changes to kprompt are documented here. Versions follow [GitHub Releases](https://github.com/kprompt/kprompt/releases).

## Unreleased

### Features

- **RT-001 Learn writeback** — Autopilot apply + post-apply verify outcomes (`apply_success` / `apply_failed` / `apply_partial`) update the incident patterns store; Incident stamps `lastApplyOutcome` / `lastVerifyStatus` / `lastActionId` ([docs/learn.md](./docs/learn.md))
- `agent autopilot apply-proposal --patterns` for CLI Learn writeback
- **RT-002 proposal ranking bias** — multi-candidate Autopilot detect + Learn weight / `LastActionID` ranking; `learnNote` + `ActionConfidence` bias
- **RT-007 durable proposals** — `agent proposals list|show|apply`; ConfigMap/file store; auto-enabled with `--autopilot-propose`
- **RT-017/018 Incident→PlanResult bridge** — propose+store before notify; alerts carry `proposalId` + apply hint (`agent proposals apply --approve`)
- **RT-005 Helm Autopilot path** — `charts/kprompt-agent` values `autopilotMode`, `autopilotAllow`, proposals ConfigMap + RemediationPolicy templates; RBAC for proposals store
- **RT-006 post-apply verify gate** — `ApplyProposal` sets `applied=true` only after T-070 verify ok (or skipped)
- **RT-008 Slack approve bridge** — `--slack-ask` `approve [proposal-id]` applies durable proposals under policyAuto (ADR-0015)
- **RT-013…016 Topology KG** — ExternalName/env `depends_on`, ready EndpointSlice routes, NetworkPolicy peer `allows`, Autopilot `expectedImpact` graph notes ([docs/graph.md](./docs/graph.md))
- **RT-009…012 Continuous Coordinator** — opt-in `--tick-interval` proactive correlation; blast-radius `status=degraded` without `--mesh-otel`; `--max-hops` + audit ([docs/coordinator-knowledge.md](./docs/coordinator-knowledge.md))
- **RT-021 Coordinator outcome ring** — durable cross-ns outcomes (action/ns/result) beside Shared Knowledge with TTL + size cap; `POST /v1/outcome`, `GET /v1/outcomes`, `agent coordinator outcomes`; coexists in the knowledge ConfigMap/file; evidence-not-proof summary for fleet bias ([docs/coordinator-knowledge.md](./docs/coordinator-knowledge.md))
- **RT-022 Fleet pattern read (evidence-not-proof)** — `agent run --coordinator-url` reads the Coordinator outcome summary and nudges Autopilot `ActionConfidence` only (bounded ±0.05, ≥3 samples, cached), applied ONLY when local Learn already matched — never gates apply, never invents candidates (AG-034); proposals carry a `Fleet evidence (not proof)` note
- **RT-023 Export / backup Incident Memory** — `agent memory export -n <ns>` (restorable Snapshot) or `--fleet` (NamespaceMemoryExport bundle); file backend scans the memory dir, ConfigMap backend lists labelled `kprompt-namespace-memory`; `--out` writes a local file (0600), never uploaded to api.kprompt.ai
- **RT-024 Docs cluster memory vs laptop remember** — new [docs/cluster-memory.md](./docs/cluster-memory.md) contrasts ADR-0022 laptop `remember` vs in-cluster Incident Memory (namespace facts + patterns + RT-021 outcome ring), cross-linked from `remember.md` / `agent.md`

## [v0.9.0](https://github.com/kprompt/kprompt/releases/tag/v0.9.0) — 2026-08-04

Day-0 CLI onboarding pack + provider/agent follow-ups since v0.8.0.

### Features

- **Coach** — bare `kprompt` prints kube/LLM/cluster readiness and next steps (OB-001)
- **`kprompt init`** — configure Ollama ($0) or BYOK without silent OpenAI default (OB-002 · OB-003)
- **`kprompt demo`** — $0 Observe walkthrough checklist + `--check` (OB-004)
- **Help groups** — Day-0 vs Advanced; `kprompt advanced` (OB-005)
- **Approval vocabulary** — unified `Apply …? [y/N]` / `--approve`; history clear `--approve` (`--yes` alias) (OB-006)
- **Wipe deny remediation** — flavor punchlines + stable `Next:` named-target hint (OB-007)
- **Cerebras** provider preset (#95)
- **Discord** notify wiring for Observe agent Helm/CRD (#94)

### Docs

- `docs/init.md` · `docs/demo.md` · `docs/approval.md`
- Architecture [CLI-ONBOARDING-TASKS.md](https://github.com/kprompt/kprompt-architecture/blob/main/CLI-ONBOARDING-TASKS.md) (OB-001…007 Done)
- Gemini free-tier / Team run bridge honesty notes

### Notes

Experimental — prefer non-production clusters. Empty config no longer implies OpenAI; run `kprompt init` first. Autopilot remains propose-only by default.

## [v0.8.0](https://github.com/kprompt/kprompt/releases/tag/v0.8.0) — 2026-08-02

AI SRE intelligence pack + laptop AI Native surfaces.

### Features

- **`search`** — NL inventory query → `SearchReport` hits (S-010)
- **`score`** — reliability / security / cost scorecard; cost skipped without Prometheus (S-011)
- **`explain architecture`** — narrative from learn + graph + heuristic deps (S-012)
- **`watch`** — opt-in laptop proactive scan; suggests investigate; never mutates (S-014 · ADR-0022)
- **`remember` / `forget`** — local operator memory (`~/.kprompt/memory.json`); planning bias (S-015)
- **`session`** — today’s history day digest (S-016)
- Setup profiles / honesty (T-065 · T-066) and prior SRE MVPs already on `main`

### Docs

- `docs/search.md` · `docs/score.md` · `docs/architecture.md` · `docs/watch.md` · `docs/remember.md` · `docs/session.md`
- Architecture [ADR-0022](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0022-laptop-ai-native.md)

### Notes

Experimental — prefer non-production clusters. Autopilot remains propose-only by default. Laptop `watch` is not a required daemon; always-on Observe stays on `kprompt agent`.

## [v0.7.0](https://github.com/kprompt/kprompt/releases/tag/v0.7.0) — 2026-07-28

Community-powered patch-plus release: first-wave contributor PRs, plus day-2 CLI features shipped on `main` since v0.6.0.

### Thanks

Huge thanks to first-time and returning contributors:

- [@syloe1](https://github.com/syloe1) — docs, tests, charts, UX copy (#19, #27–#32)
- [@atharvafulay](https://github.com/atharvafulay) — doctor Helm setup hint (#43)
- [@terminalchai](https://github.com/terminalchai) — detect helper unit tests (#26)
- [@pollychen-lab](https://github.com/pollychen-lab) — contexts help clarification (#9)

### Community contributions

- **Docs / help:** theme flag docs; Cobra `Example` fields for `doctor` / `contexts` / `history`; GitOps PR flags in README; `learn --show` header clarity; contexts help text
- **Tests:** UI helpers, route helpers, tools detect helpers
- **Charts:** `kprompt-coordinator` Helm `NOTES.txt`
- **UX:** doctor Helm-missing hint points at `kprompt setup` (no longer “coming soon”)

### Features (since v0.6.0)

- **`kprompt setup`** — detect/plan + approve-gated host Helm and cluster operator installs (T-062…T-064)
- **GitOps PR mode** — `--gitops` opens/updates a GitHub PR instead of applying live (T-072)
- **`kprompt learn`** — local cluster tool profiles (S-009)
- **Drift scan** — GitOps out-of-sync apps (S-008)
- **Recipes** — curated packs that expand to approve-gated routes (S-013)
- **Optimize** — labeled cost/carbon estimate notes on idle/rightsizing (T-073)
- **Moonshot / Kimi K3** — named BYOK provider preset

### Notes

Experimental — prefer non-production clusters. Autopilot remains propose-only by default.

## [v0.6.0](https://github.com/kprompt/kprompt/releases/tag/v0.6.0) — 2026-07-26

Namespace Agent pack: multi-signal Observe (Prom/OTel/GitOps), InvestigationReport v2, Slack ask, thin Coordinator, gated Autopilot (`policyAuto`), plus CLI investigate/why/timeline/impact/audit/cleanup.
