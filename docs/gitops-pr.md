# GitHub Integration (MVP)

CLI path to change desired state through **GitHub**, not silent cluster apply.

| Surface | How | Status |
|---------|-----|--------|
| **GitOps PR mode** | `--gitops` opens/updates a PR via `gh` / `GH_TOKEN` | Shipped (T-072) |
| **GitOps controllers** | `kprompt "show gitops sync status"` (Flux / Argo CD) | Shipped (T-043) |
| **Agent evidence** | `--gitops-evidence` Argo/Flux EvidenceRefs | Shipped (AG-035) |
| **Team org connect-repo** | `app.kprompt.ai` SCM binding | Shipped MVP (ADR-0019 · A-061…A-069) including Checks annotate upsert |

This doc is the umbrella for the **CLI GitHub Integration MVP**. Deep dive on PR mode: sections below (formerly `gitops-pr.md` body). Team org repos are **not** wired into the CLI yet.

## GitOps PR mode

`--gitops` opens a **GitHub pull request** instead of applying a mutating plan to
the cluster (T-072):

```bash
# one-shot
kprompt "deploy redis" -n demo --gitops --gitops-repo acme/infra --approve

# or persist
kprompt config set gitops.mode pr
kprompt config set gitops.repo acme/infra
kprompt config set gitops.path clusters/dev
kprompt "deploy redis" -n demo --approve
```

The plan banner shows **Apply target: Git PR (not cluster)** with repo / path /
base branch. Confirm becomes “Apply this plan as a GitHub PR?” (or `--approve`).

## Requirements

| Need | How |
|------|-----|
| Repo | `gitops.repo` / `--gitops-repo` / `KPROMPT_GITOPS_REPO=owner/name` |
| Auth | `gh` on PATH + `gh auth login`, or `GH_TOKEN` / `GITHUB_TOKEN` |
| Path prefix | `gitops.path` (default `kprompt`) |
| Base branch | `gitops.base_branch` (default `main`) |

If the repo is unset, kprompt **refuses** with an honest error (no silent
cluster fallback). Team org repos ([ADR-0019](https://github.com/kprompt/kprompt-architecture/blob/main/decisions/ADR-0019-org-repos-pipelines.md) · **A-061+**) are not wired yet — this is the
local CLI path.

## Supported plans (MVP)

| Kind | Behavior |
|------|----------|
| `deploy` | Commit rendered Deployment/Service YAML into the PR |
| `install` / `upgrade` (Helm) | Re-run `helm template` / dry-run and commit full YAML |

Scale, delete, rollback, live Flux/Argo **sync**, Argo Workflows, Tekton, KEDA,
and Crossplane stay on **cluster apply** — omit `--gitops` for those.

## What happens

1. Plan + safety as usual (still PlanResult-shaped).
2. After approval: create branch `kprompt/<kind>-<timestamp>`, write files under
   `gitops.path/`, open a PR with `gh api`.
3. **Cluster is not mutated.** Merge the PR and let Flux/Argo CD reconcile.

```bash
kprompt "deploy api" -n demo --gitops --gitops-repo acme/infra -o json | jq '.result'
```

## Related

- Drift report (read-only): [docs/drift.md](./drift.md)
- Live GitOps sync status: `kprompt "gitops status"`
- Observe GitOps evidence: [docs/agent.md](./agent.md)
- Org SCM binding (future): architecture **A-060…A-066**
