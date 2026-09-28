# `kprompt demo`

$0 canonical AI Runtime walkthrough — **no LLM key**, no Team login.

Prints prerequisite checks (Docker, kind, kubectl, make, git, kprompt) and the exact [kprompt-examples](https://github.com/kprompt/kprompt-examples) commands. Does **not** clone or mutate; it is a guided checklist (OB-004 MVP).

The canonical path uses one failed rollout and follows it through **Observe →
PlanResult-shaped proposal → explicit human approval → apply → verify → Learn**.
Analysis is deterministic/heuristic rather than LLM-backed. Apply remains behind a
narrow `policyAuto` allowlist and `[y/N]`.

## Usage

```bash
kprompt demo           # prereq status + walkthrough commands
kprompt demo --check   # exit 1 if any PATH tool is missing
```

Walkthrough (after tools are ready):

```bash
git clone https://github.com/kprompt/kprompt-examples.git
cd kprompt-examples && make magic-moment
```

The script prints the complete proposal before approval. When stdin is not a TTY,
it stops before mutation and prints the exact review/apply command.

For the advanced multi-scenario Observe/health demo:

```bash
make walkthrough
```

## After the demo

```bash
kprompt init --ollama
kprompt "how's my cluster"
kprompt "scale api to 3"   # plan first, then y/N
```

## Related

- [init.md](./init.md) — LLM Day-0 setup
- [agent.md](./agent.md) — Observe agent reference
- Bare `kprompt` coach points here when you want the $0 demo path
