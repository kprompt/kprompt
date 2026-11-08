# Session / day digest

Summarize **today’s** local history (S-016 · [ADR-0022](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0022-laptop-ai-native.md) · T-019):

```bash
kprompt session
kprompt session --json
kprompt "what did I do today"
```

Source: `~/.kprompt/history.jsonl` only. Counts investigates / scales /
rollbacks / other kinds and lists highlights. No LLM essay in MVP.

## Honest limits

- Local history only (not Team cloud `/history`).
- “Today” uses the local timezone.
- Empty digest if you have not run prompts today (or history is disabled).

See also: [history.md](./history.md) · [remember.md](./remember.md) · [watch.md](./watch.md) · README [Quick start §5](../README.md#quick-start) (habit cues).
