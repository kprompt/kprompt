# Learn loop (RT-001…)

kprompt’s AI Runtime Learn path closes Observe → Plan → Approve → Execute with **durable outcomes** that reshape the next “seen before” boost.

## Signals

| Signal | Outcome | Effect on pattern store |
|--------|---------|-------------------------|
| Alert recovered | `resolved` | `Confirmed++`, weight +0.05 |
| Slack `false positive` | `false_positive` | `FalsePositives++`, weight −0.15 (floor 0.2) |
| Autopilot apply + verify `ok` | `apply_success` | same as resolved (+ `LastActionID`) |
| Autopilot apply error or verify `failed` | `apply_failed` | weight −0.10 (**not** FP) |
| Verify `pending` | `apply_partial` | weight −0.02 |

**RT-002 ranking:** when multiple Autopilot actions match, prefer `LastActionID` with healthy weight; dampen when FP-heavy / low weight. `ActionConfidence` and `learnNote` reflect bias; MinConfidence gate still uses raw alert confidence (evidence-not-proof).

## Surfaces

- `kprompt agent run … --patterns --autopilot-propose [--autopilot-apply]`
- `kprompt agent autopilot apply-proposal --approve --patterns …`
- `kprompt agent proposals list|show|apply` — durable store (RT-007)
- Incident fields: `lastApplyOutcome`, `lastVerifyStatus`, `lastActionId`

## Second-hit demo (IC-009)

Same fingerprint twice in kind — first apply writes Learn; second propose should show a **visible** `learnNote` / `actionConfidence` nudge (and higher pattern `weight` / `confirmed`).

```bash
# in kprompt-examples
make learn-second-hit
# artifacts under $STATE_ROOT: hit1-proposal.json · hit2-proposal.json · patterns/
```

What to say in a demo Q&A:

| Question | Answer |
|----------|--------|
| Does it auto-fix prod? | **No.** Propose → human `--approve` → verify → Learn. |
| What did Learn change? | Ranking / `ActionConfidence` bias for the next similar alert — **evidence, not proof**. |
| Can weight skip Safety? | **No.** MinConfidence still uses raw alert confidence; wipe-class / policy gates unchanged. |

## Non-goals

- Silent fleet heal
- Treating Learn weight as approval to skip Safety / RemediationPolicy
- Claiming second-hit confidence as “proven correct”
