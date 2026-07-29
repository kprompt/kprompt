# kprompt dash

`kprompt dash` starts the local **read-only** cluster UI powered by
[`kprompt-dash`](https://github.com/kprompt/kprompt-dash).

It runs on your machine, uses your local kubeconfig, and does **not** mutate the
cluster.

## Install

Install the dashboard binary:

```bash
go install github.com/kprompt/kprompt-dash/cmd/kprompt-dash@latest
```

`kprompt dash` looks for `kprompt-dash` on `PATH` first. If it is not found, it
also checks the common Go install path at `~/go/bin/kprompt-dash`.

## Override the binary path

If the dashboard binary lives somewhere else, set `KPROMPT_DASH_BIN`:

```bash
export KPROMPT_DASH_BIN=/custom/path/kprompt-dash
kprompt dash
```

If `KPROMPT_DASH_BIN` points to a missing file, `kprompt dash` exits with an
error.

## Usage

Basic launch:

```bash
kprompt dash
```

This starts the local UI on `127.0.0.1:7474` by default.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--addr` | `127.0.0.1:7474` | Dashboard listen address |
| `--context` | empty | Kubeconfig context for the dashboard |
| `--open` | `true` | Ask `kprompt-dash` to print/open the local URL |

## Examples

Use a different address:

```bash
kprompt dash --addr 127.0.0.1:8080
```

Target a specific kubeconfig context:

```bash
kprompt dash --context staging
```

Disable automatic open behavior:

```bash
kprompt dash --open=false
```

## Safety

- Local-only UI: kubeconfig stays on this machine.
- Read-only dashboard: `kprompt dash` does not apply, patch, or delete cluster resources.
- Distinct from the main `kprompt` plan/apply flow: this is for local inspection only.
