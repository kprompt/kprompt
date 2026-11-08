# Watch (laptop proactive assistant)

Opt-in **laptop** scanner (S-014 · [ADR-0022](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0022-laptop-ai-native.md)):

```bash
kprompt watch -n payments --once
kprompt watch -n payments --interval 30s
kprompt "watch payments namespace" -n payments
```

Reads Pods + recent Warning Events and prints **suggestions** such as
`kprompt "investigate api-0" -n payments`. Never mutates and never auto-applies.

## vs in-cluster Observe

| | `kprompt watch` | `kprompt agent` / Helm |
|--|-----------------|-------------------------|
| Where | Laptop foreground | In-cluster workload |
| ADR | ADR-0022 | ADR-0013 |
| Always-on | No (opt-in loop / `--once`) | Yes (Observe Mode) |
| Notifiers | Print only | Slack / webhook |

## Honest limits

- Namespace-scoped MVP (`-n` required).
- No fake Prom latency alerts without a configured querier in this MVP.
- Suggestions are hints — you still run investigate/why under the normal approval loop.

See also: [agent.md](./agent.md) · [investigate.md](./investigate.md) · [remember.md](./remember.md) · [session.md](./session.md) · README [Quick start §5](../README.md#quick-start).
