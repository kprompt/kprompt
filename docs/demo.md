# `kprompt demo`

$0 Observe walkthrough entry — **no LLM key**, no Team login.

Prints prerequisite checks (Docker, kind, kubectl, make, git, kprompt) and the exact [kprompt-examples](https://github.com/kprompt/kprompt-examples) commands. Does **not** clone or mutate; it is a guided checklist (OB-004 MVP).

This is **Observe / heuristic** (kind + broken workloads + propose-only agent). It is **not** the NL plan→approve loop.

## Usage

```bash
kprompt demo           # prereq status + walkthrough commands
kprompt demo --check   # exit 1 if any PATH tool is missing
```

Walkthrough (after tools are ready):

```bash
git clone https://github.com/kprompt/kprompt-examples.git
cd kprompt-examples && make walkthrough
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
