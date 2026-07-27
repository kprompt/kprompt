# Themes

kprompt supports built-in terminal themes for human-readable output.

## Select a theme

Use a one-off flag:

```bash
kprompt --theme nord "list pods"
```

Set a persistent preference:

```bash
kprompt config set theme nord
```

Or use an environment variable:

```bash
export KPROMPT_THEME=dracula
```

Available theme names:

- `auto`
- `dracula`
- `nord`
- `gruvbox`
- `mono`
- `none`

## Color overrides

Disable ANSI color entirely:

```bash
export NO_COLOR=1
```

Force color even when output is not a TTY:

```bash
export KPROMPT_FORCE_COLOR=1
```

`auto` is the default. When no explicit theme is set, kprompt falls back to `KPROMPT_THEME`, then `auto`.
