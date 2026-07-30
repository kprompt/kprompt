# kprompt doctor

`kprompt doctor` runs local **read-only** setup checks for the CLI.

It verifies your config, current LLM provider setup, Kubernetes access,
detected integrations, optional Team enrollment state, and related local cache
files. It never prints API key values.

## What it checks

- Config file path and selected provider
- LLM provider/model and whether a usable API key is set
- Kubernetes access for the selected context
- Optional integrations such as Helm and configured backends
- Optional Team enrollment, cached policy, pulled provider keys, and learned profile

Required failures make the command exit with status `1`.

## Examples

```bash
kprompt doctor
kprompt doctor --json
kprompt doctor --context staging
```

## Output modes

Default output is a human-readable table with:

- `CHECK`
- `STATUS`
- `DETAIL`

Checks may be marked:

- `PASS`
- `FAIL`
- `WARN`
- `SKIP`

Use JSON when you want machine-readable output:

```bash
kprompt doctor --json
```

## Context override

To probe a specific kubeconfig context, pass `--context`:

```bash
kprompt doctor --context prod
```

This only changes the cluster-related checks for that run.

## Exit behavior

- Exit `0`: all required checks passed
- Exit `1`: at least one required check failed

Optional checks may still show `WARN` or `SKIP` even when the overall result is
OK.

## Notes

- API keys are never printed
- Team enrollment is optional
- Integration summaries point you to `kprompt tools` for deeper inspection
