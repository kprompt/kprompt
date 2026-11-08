# Daily driver (week of day-2 prompts)

**IC-015.** One week of Free CLI muscle memory — **shipped capabilities only**. Swap `payments` / `api` for your namespace and workload. Prefer a sandbox context first.

Default contract: **plan before approve**. Reads run immediately; mutates stop on a PlanResult unless you say `y` / `--approve`.

```bash
kubectl config current-context   # know where you are
kprompt doctor                   # once when something feels off
```

---

## The week

| Day | Job | Prompt / command | Mutates? |
|-----|-----|------------------|----------|
| **1** | Health pulse | `kprompt "how's my cluster"` | No (no LLM key needed) |
| **2** | Laptop scan | `kprompt watch -n payments --once` | No — prints investigate/why suggestions only |
| **3** | Root cause | `kprompt "investigate api" -n payments` · or `kprompt "why is api crashing" -n payments` | No |
| **4** | Rightsizing read | `kprompt "optimize my cluster"` | No (report; any follow-up mutate is a separate plan) |
| **5** | Scale plan | `kprompt "scale api to 3" -n payments` → review PlanResult → `--approve` only if you trust it | Yes **after** approve |
| **6** | Rollback plan | `kprompt "rollback api" -n payments` → same gate | Yes **after** approve |
| **7** | Habit close | `kprompt session` · `kprompt history` · optional `kprompt remember "payments = Team A"` | No |

Escape hatch anytime (`$0` magic moment, no LLM key): `kprompt demo` → [demo.md](./demo.md).

---

## Copy-paste block

```bash
# Day 1–2 — observe
kprompt "how's my cluster"
kprompt watch -n payments --once

# Day 3–4 — reason (read-only)
kprompt "investigate api" -n payments
kprompt "optimize my cluster"

# Day 5–6 — plan, then maybe approve
kprompt "scale api to 3" -n payments
# kprompt "scale api to 3" -n payments --approve   # only after review
kprompt "rollback api" -n payments
# kprompt "rollback api" -n payments --approve

# Day 7 — local memory
kprompt session
kprompt history
kprompt remember "payments = Team A"
```

---

## Honesty

| Do | Do not |
|----|--------|
| Run Days 1–4 on production if RBAC allows reads | `--approve` on Day 5–6 against prod on first try |
| Use `kprompt demo` / kind when you have no cluster | Treat laptop `watch` as a replacement for in-cluster `agent` |
| Keep MCP / IDE for plan review ([mcp.md](./mcp.md)) | Expect MCP to apply |
| Graduate to [recipes.md](./recipes.md) packs when the week sticks | Invent prompts for capabilities not shipped |

---

## Related

- Quick start habit cues: [README §5](../README.md#quick-start) · [watch](./watch.md) · [remember](./remember.md) · [session](./session.md)
- Brownfield: [adopt.md](./adopt.md)
- Curated multi-step packs: [recipes.md](./recipes.md)
- Observe always-on: [agent.md](./agent.md) · Magic moment: [demo.md](./demo.md)
