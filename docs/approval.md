# Approval vocabulary

One language family for “human said yes” (IC-006). Risk words are always **`low` · `medium` · `high` · `denied`**.

## CLI

| Context | Interactive (TTY) | Non-interactive / CI |
|---------|-------------------|----------------------|
| Cluster mutate / setup apply | `Apply this plan? [y/N]` | `--approve` |
| GitOps PR instead of apply | `Apply this plan as a GitHub PR? [y/N]` | `--approve` |
| Multi-context mutate | Per-context `Apply … to context …?` | `--approve-each-context` (plain `--approve` refused) |
| Local history clear | `Clear all local history? [y/N]` | `--approve` (`--yes` / `-y` alias) |
| Extra-destructive gates | Type exact phrase (e.g. `DELETE-ORPHANS`) | Only after the prompt already carries confirm language; still prefer TTY |
| Durable AutopilotProposal | Review `agent proposals show`, then apply | `agent proposals apply -n <ns> --id <id> --approve` |

**Primary CI / non-TTY flag:** `--approve`. Do not teach a second primary flag.

Phrase gates stay rare and intentional — they are not a substitute for `--approve`.

## Slack ask (`--slack-ask`)

Same gate as CLI approve — RemediationPolicy `policyAuto` + allowlist + audit. Never silent apply.

| Phrase | Effect |
|--------|--------|
| `approve <proposal-id>` | Apply that stored proposal (preferred) |
| `approve` / `approve proposal <id>` | Latest / named id (see agent docs) |
| `apply <proposal-id>` | Alias of approve (same path) |
| `status` · `why` · `what broke` | Read-only |
| `false positive` / `fp` | Learn dampen (needs `--patterns`) |
| `help` | Lists phrases + CLI equivalence |

**Equivalence:** Slack `approve <id>` ≡ CLI `--approve` after you have reviewed the proposal (TTY would have asked `Apply this plan? [y/N]`).

Alerts print:

- `Approve (CLI):` `kprompt agent proposals apply … --approve`
- `Approve (Slack ask):` `approve <id>`
- `risk` using the same low/medium/high/denied words
- optional `Impact:` from the proposal’s expected impact (not a full blast-radius dump)

## Discord

Notify-only today (no ask listener). Use the **Approve (CLI)** hint on the alert, or Slack ask if that channel is enabled. Do not invent a Discord-only approve verb.

## Org Policy Tighten-Only Priority (SEC-005)

When a remote org policy is applied, local CLI safety rules (such as blocking unscoped deletes, cluster deletes, and namespace deletions) are always evaluated first. 

An org policy may only **tighten** safety parameters (e.g., lower the risk ceiling or restrict allowed namespaces/intents). An org policy can **never** waive, override, or loosen local safety rules. Local hard-denies always take precedence.

See also: [planresult-parity.md](./planresult-parity.md) · [agent.md](./agent.md) · [ci.md](./ci.md) · [multi-cluster.md](./multi-cluster.md) · [gitops-pr.md](./gitops-pr.md) · [history.md](./history.md)
