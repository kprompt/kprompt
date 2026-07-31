# Audit / security hygiene

`audit` is a read-only security and hygiene scan (S-006 · T-084):

```bash
kprompt "audit payments namespace" -n payments
kprompt "security scan" -n production
kprompt "audit my cluster"
kprompt "hygiene check" -n shop --output json
```

It walks Deployment, StatefulSet, and DaemonSet pod templates and emits an
ADR-0014 `Investigation` with coded findings. The scan itself never mutates.

When findings include privilege grants, `audit` may offer a single **reviewable
harden plan** (same gate as `explain` / `why`): TTY `y/N` or `--approve`. Nothing
applies silently, and `-o json` stays report-only.

## MVP checks

| Code | What it flags |
|------|----------------|
| `Audit.RunAsRoot` | `runAsNonRoot` is not `true` on container or pod |
| `Audit.Privileged` | `securityContext.privileged=true` |
| `Audit.PrivilegeEscalation` | `allowPrivilegeEscalation=true` (explicit) |
| `Audit.LatestTag` | image is untagged or tagged `latest` (digests OK) |
| `Audit.MissingImagePullPolicy` | empty `imagePullPolicy` with a mutable tag |
| `Audit.MissingRequests` | missing CPU or memory requests |
| `Audit.MissingLimits` | missing CPU or memory limits |
| `Audit.HostNamespace` | `hostNetwork` / `hostPID` / `hostIPC` |
| `Audit.WritableRootFS` | `readOnlyRootFilesystem` is not `true` (unset or `false`) |

```bash
kprompt "audit payments" -n payments --output json | jq '.result'
```

## Harden plan (approve-required)

`audit` offers **one aggregate harden plan** that only ever *removes* a privilege
grant on Deployment, StatefulSet, and DaemonSet containers — never a change that
could stop a container from starting or that requires an invented value:

| Finding | Auto-patch (approve-gated) |
|---------|-----------------------------|
| `Audit.Privileged` | set `securityContext.privileged: false` |
| `Audit.PrivilegeEscalation` | set `allowPrivilegeEscalation: false` |

Everything else stays **guidance-only** — `Audit.RunAsRoot` (enforcing non-root
can break a root image), `Audit.WritableRootFS` (enforcing a read-only root
filesystem can break containers that write to disk), `Audit.HostNamespace`,
`Audit.LatestTag` (never invent a tag), `Audit.MissingRequests` /
`Audit.MissingLimits` (never invent CPU/memory), and any other workload kind
(Pod, Job, CronJob templates are reported, not patched).

## Honest limits

This MVP does **not** cover NetworkPolicy gaps, RBAC over-permission, or
PodSecurity admission levels. Findings are static template rules — not a
CIS benchmark or live runtime attestation.
