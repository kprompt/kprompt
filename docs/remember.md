# Remember / local memory

Durable **local** operator facts (S-015 · [ADR-0022](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0022-laptop-ai-native.md)):

```bash
kprompt remember "payment ns = Team A"
kprompt remember "oncall = alice" -n payments
kprompt remember list
kprompt forget "payment ns"
kprompt "remember that tier = gold"
```

Store: `~/.kprompt/memory.json` (mode `0600`). **Not** uploaded to
`api.kprompt.ai` by default.

Facts are injected as planning bias on later prompts (alongside `learn`
profiles). Live cluster reads still win — memory is a hint, not proof.

## Honest limits

- Local-only MVP (no cloud sync).
- Stale facts are your responsibility (`forget`).
- Distinct from namespace agent dependency memory (`kprompt agent memory`).

> Laptop `remember` vs the in-cluster Incident Memory (namespace facts, incident
> patterns, Coordinator outcome ring) is spelled out in
> [cluster-memory.md](./cluster-memory.md).

See also: [history.md](./history.md) · [learn.md](./learn.md) · [session.md](./session.md) · [cluster-memory.md](./cluster-memory.md).
