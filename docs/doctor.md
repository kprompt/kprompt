# kprompt doctor

`kprompt doctor` runs local **read-only** setup checks for the CLI.

It verifies your config, current LLM provider setup, Kubernetes access,
detected integrations, optional Team enrollment state, and related local cache
files. It never prints key or token values.

**Key vocabulary:** an **LLM provider key** is yours (OpenAI/Gemini/… env var, or none for Ollama). A Team **`kp_…` token** from `kprompt login` is separate org enrollment — not required for Free CLI NL plans. kprompt does not sell either.

If no provider is configured, the LLM check fails with a hint to run `kprompt init --ollama` (the CLI no longer silently defaults to OpenAI).

## What it checks

- Config file path and selected provider
- LLM provider/model and whether a usable provider key is set (Ollama needs no key)
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

- LLM provider keys and Team tokens are never printed
- Team enrollment is optional; missing `kp_…` is not a Free CLI failure
- Prefer Ollama ($0) or the zero-LLM walkthrough before buying a cloud provider key
- Integration summaries point you to `kprompt tools` for deeper inspection
