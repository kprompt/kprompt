# Cluster memory vs laptop `remember`

kprompt has **two different memory systems**. They look similar (both "remember
things", both local-first, neither uploads to `api.kprompt.ai`) but they live in
different places, hold different data, and serve different loops. This page
contrasts them so you pick the right one (RT-024).

## TL;DR

| | Laptop `remember` | In-cluster Incident Memory |
|---|---|---|
| Command | `kprompt remember` / `forget` | `kprompt agent memory` · `agent patterns` · Coordinator outcome ring |
| ADR | [ADR-0022](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0022-laptop-ai-native.md) | AG-015 · AG-032…AG-034 · RT-021 |
| Store | `~/.kprompt/memory.json` (0600) | file (`~/.config/kprompt/…`) or ConfigMaps |
| Scope | your laptop, your prompts | the agent(s) running in the cluster |
| Holds | free-form operator facts ("tier = gold") | dependency facts, incident patterns, cross-ns outcomes |
| Feeds | planning bias on your later prompts | analyzer context + Autopilot ranking bias |
| Lifetime | until you `forget` | pod restarts survive (ConfigMap); TTL/cap on the outcome ring |
| Upload | never by default | never by default |

Both are **evidence, not proof**: they bias confidence and explainability; live
cluster reads always win, and neither ever auto-mutates (AG-034).

## Laptop `remember` (ADR-0022)

Personal, prompt-time facts on the machine you drive kprompt from:

```bash
kprompt remember "payment ns = Team A"
kprompt remember "oncall = alice" -n payments
kprompt remember list
```

Injected as planning bias on later prompts alongside `learn` profiles. Details in
[remember.md](./remember.md). This never leaves your laptop and is unrelated to
what an in-cluster agent knows.

## In-cluster Incident Memory

What a running Namespace Agent / Coordinator remembers so pod restarts don't wipe
learning. Three layers (see [agent.md](./agent.md#incident-memory)):

1. **Namespace facts** (`agent memory`, AG-015) — "uses Redis/Kafka/Postgres".
2. **Incident patterns** (`agent patterns`, AG-032…034) — signatures → "Seen
   before (N×)" + outcome weights that bias Autopilot ranking.
3. **Coordinator outcome ring** (RT-021) — durable cross-namespace outcomes
   (`action`, `namespace`, `result`) beside Shared Knowledge, TTL + size cap. Read
   by namespace agents as bounded **fleet bias** (RT-022) — never sole proof.
   See [coordinator-knowledge.md](./coordinator-knowledge.md#outcome-ring-rt-021).

### Export / backup (RT-023)

`agent memory export` writes namespace memory to a local file (or stdout) for
backup — **offline only, never uploaded to `api.kprompt.ai`**:

```bash
# One namespace (restorable Snapshot)
kprompt agent memory export -n payments --out payments-memory.json

# Whole fleet as a bundle (file backend: scans memory dir)
kprompt agent memory export --fleet --out fleet-memory.json

# Fleet from in-cluster ConfigMaps (lists labelled kprompt-namespace-memory)
kprompt agent memory export --fleet --memory-backend configmap --in-cluster \
  --out fleet-memory.json
```

Single-namespace export emits a `memory.Snapshot`; `--fleet` emits a
`NamespaceMemoryExport` bundle with a per-kind summary across namespaces.

## Which do I use?

- Jotting a personal note while triaging from your laptop → `remember`.
- Teaching the in-cluster agent a lasting dependency / capturing what fixed an
  incident so future proposals rank better → `agent memory` / `agent patterns` /
  the Coordinator outcome ring.

## Related

- [remember.md](./remember.md) — laptop memory (ADR-0022)
- [agent.md](./agent.md#incident-memory) — in-cluster Incident Memory layers
- [coordinator-knowledge.md](./coordinator-knowledge.md) — Shared Knowledge + outcome ring
- [learn.md](./learn.md) — Learn writeback / ranking bias
