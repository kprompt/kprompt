# Workflow recipe packs

Curated multi-step workflows expand into the same **route + approval** loop as a
manual `A then B` chain (S-013 · T-088). They never mutate silently.

```bash
kprompt recipe list
kprompt recipe show harden-production
kprompt recipe run harden-production -n payments
kprompt recipe run crashloop-rca --workload api -n payments

# NL also matches triggers:
kprompt "harden production" -n payments
kprompt "prepare for black friday"
kprompt "migrate ingress to gateway api" -n edge
```

## Built-in packs

| ID | Mode | Steps (summary) |
|----|------|-----------------|
| `harden-production` | approve-gated | audit → optimize → cleanup |
| `prepare-black-friday` | approve-gated | optimize → audit → drift → cleanup |
| `migrate-ingress-gateway` | read-only | list Ingress → learn → service graph |
| `crashloop-rca` | read-only | why crash → investigate → logs |
| `oom-rca` | read-only | why OOM → investigate → logs |
| `imagepull-rca` | read-only | why ImagePull → investigate → describe |

RCA packs need `--workload` (or `for <name>` in the prompt). Placeholders:
`{{namespace}}` (default `default`) and `{{workload}}`.

## Safety

- Mutating suggestions (harden patches, cleanup deletes, sync) stay on the
  normal PlanResult approval path (`--approve` / TTY `y/N`).
- Ingress→Gateway is **discover only** — no auto-rewrite of Ingress objects.
- Distinct from Team org recipe library (**A-030**).

## Related

- Week of single prompts (habit): [daily-driver.md](./daily-driver.md)
- Incident RCA: [docs/investigate.md](./investigate.md) · [docs/why.md](./why.md)
- Audit / cleanup / drift / learn: [docs/audit.md](./audit.md) · [docs/cleanup.md](./cleanup.md) · [docs/drift.md](./drift.md) · [docs/learn.md](./learn.md)
