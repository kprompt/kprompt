# Approval vocabulary

One language family for “human said yes”:

| Context | Interactive (TTY) | Non-interactive / CI |
|---------|-------------------|----------------------|
| Cluster mutate / setup apply | `Apply …? [y/N]` | `--approve` |
| GitOps PR instead of apply | `Apply this plan as a GitHub PR? [y/N]` | `--approve` |
| Multi-context mutate | Per-context `Apply … to context …?` | `--approve-each-context` (plain `--approve` refused) |
| Local history clear | `Clear all local history? [y/N]` | `--approve` (`--yes` / `-y` alias) |
| Extra-destructive gates | Type exact phrase (e.g. `DELETE-ORPHANS`) | Only after the prompt already carries confirm language; still prefer TTY |

**Primary CI flag:** `--approve`. Do not teach a second primary flag.

Phrase gates stay rare and intentional — they are not a substitute for `--approve`.

## Org Policy Tighten-Only Priority (SEC-005)

When a remote org policy is applied, local CLI safety rules (such as blocking unscoped deletes, cluster deletes, and namespace deletions) are always evaluated first. 

An org policy may only **tighten** safety parameters (e.g., lower the risk ceiling or restrict allowed namespaces/intents). An org policy can **never** waive, override, or loosen local safety rules. Local hard-denies always take precedence.

See also: [ci.md](./ci.md) · [multi-cluster.md](./multi-cluster.md) · [gitops-pr.md](./gitops-pr.md) · [history.md](./history.md)
