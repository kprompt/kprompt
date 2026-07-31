# timeline (S-004)

Ordered **incident chronology** for a workload — Events + rollout/controller revisions + HPA where applicable — not chat scroll.

Emits the same ADR-0014 **`Investigation`** artifact; the primary payload is `timeline[]` of `EvidenceRef` (time-sorted).

## Usage

```bash
kprompt "timeline for api" -n payments
kprompt "what happened to ledger" -n payments -o json
kprompt "timeline for StatefulSet db" -n payments
kprompt "what happened to DaemonSet node-agent" -n kube-system
```

Optional window (default `1h`):

```bash
kprompt "timeline for api" -n payments
# LLM / normalize sets params.window=1h; override via structured intent when using stubs/tests
```

## Sources (MVP)

1. **Events** on the target workload (Deployment, StatefulSet, DaemonSet, or Pod) and related pods
2. **ReplicaSet** revisions for Deployments (`deployment.kubernetes.io/revision`)
3. **ControllerRevision** history for StatefulSets and DaemonSets where available
4. **HPA** targeting the Deployment (status + condition transitions)

## Honest gaps (`degraded`)

MVP lists `prometheus`, `otel`, and `mesh` in `Investigation.degraded` — metrics/traces/mesh hops are not walked yet, and timeline does not invent those signals for StatefulSets or DaemonSets.

## vs `investigate` / `why`

| | `investigate` | `why` | `timeline` |
|--|---------------|-------|------------|
| Focus | Multi-hop RCA | Cause tree | Chronology |
| Primary field | `findings` | ordered Symptom→Cause | `timeline[]` |
| Trigger | “investigate X” | “why is X pending” | “timeline for X” / “what happened to X” |

Try against [kprompt-examples](https://github.com/kprompt/kprompt-examples):

```bash
make break SCENARIO=01-crashloop
kprompt "timeline for api" -n payments
```
