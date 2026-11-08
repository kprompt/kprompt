# CI PlanResult cookbook (IC-007)

**Example only.** Copy-paste a GitHub Actions workflow that installs kprompt, emits a **plan-only** `PlanResult` JSON document, and fails the job if your `jq` gates reject it. Nothing mutates the cluster unless you deliberately add a separate `--approve` job (not recommended as the default).

Canonical workflow file (raw copy target):

[`examples/github-actions/planresult-gate.yml`](../examples/github-actions/planresult-gate.yml)

Schema and jq helpers: [ci.md](./ci.md) · Field map: [planresult-parity.md](./planresult-parity.md) · Approve vocab: [approval.md](./approval.md)

---

## What you get

1. Install pinned `kprompt` via `https://kprompt.ai/install`
2. Load a **staging/sandbox** kubeconfig from `secrets.KPROMPT_KUBECONFIG_B64`
3. Run `kprompt "<prompt>" -n <ns> -o json` (**no** `--approve`)
4. Gate with `jq` on `risk.denied`, intent, delete ops, blast radius, `applied == false`
5. Upload `planresult.json` as an Actions artifact

## Setup

1. Create a **non-production** kubeconfig (or cloud auth that yields one).
2. Base64-encode it (no newlines in the secret value):

   ```bash
   base64 < ~/.kube/staging-config | tr -d '\n'
   ```

3. Repo → Settings → Secrets → `KPROMPT_KUBECONFIG_B64`
4. Copy [`examples/github-actions/planresult-gate.yml`](../examples/github-actions/planresult-gate.yml) to `.github/workflows/kprompt-planresult-gate.yml`
5. Edit `KPROMPT_PROMPT` / `KPROMPT_NAMESPACE` for your workload names
6. Open a PR and confirm the job runs

## Honesty / non-goals

| Do | Do not |
|----|--------|
| Gate **plans** before humans or GitOps apply | Treat this as production-ready policy |
| Prefer GitOps PR / environment reviewers for apply | Auto `--approve` on every PR by default |
| Pin `KPROMPT_VERSION` | Assume latest forever without reading CHANGELOG |
| Use sandbox kubeconfigs | Paste prod admin kubeconfigs into CI |

kprompt remains **experimental**. A passing gate means your jq predicates passed — not that the plan is safe for production.

## Apply later (optional)

The example workflow comments out an `apply` job behind a GitHub Environment. Prefer:

- Laptop: `kprompt "…" --approve` after review, or
- GitOps: `--gitops` PR mode ([gitops-pr.md](./gitops-pr.md)), or
- Team: app `/run` + `kprompt run listen` (never auto-apply)

If you enable CI apply anyway: require environment reviewers, keep `--wait`, and assert `.verify.status == "ok"` when verify is present.

## Related

- Product blog: [Kubernetes CI/CD plan gates](https://kprompt.ai/blog/kubernetes-ci-cd-plan-gates)
- Deep dive: [PlanResult JSON](https://kprompt.ai/blog/planresult-json-deep-dive)
