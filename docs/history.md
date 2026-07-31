# kprompt history

`kprompt history` shows recent prompts and plan summaries recorded by the CLI.

History is stored locally and does **not** include manifests or API key values.

## File location

History is written to:

```text
~/.kprompt/history.jsonl
```

The file is append-only JSON Lines. Newest entries are shown first in the CLI.

## Examples

```bash
kprompt history
kprompt history --limit 10
kprompt history rerun 3 --approve
```

## Listing entries

The default list shows up to `20` entries.

Use `--limit` to change the number shown:

```bash
kprompt history --limit 50
```

## Re-running a prompt

Use the `rerun` subcommand to replay a stored prompt through the normal
pipeline:

```bash
kprompt history rerun
kprompt history rerun 3
kprompt history rerun 3 --approve
```

- `1` means the newest entry
- reruns still use the normal approval flow unless you pass `--approve`

## Disabling history

Set this environment variable to disable history writes and truncation:

```bash
export KPROMPT_DISABLE_HISTORY=1
```

This does not remove existing history; it only stops new writes for that
environment.

## Notes

- Prompts and summaries are stored locally
- Manifests and secrets are not stored in history
- `kprompt history` reads the local file and does not require cluster access
