# kprompt over MCP

`kprompt mcp serve` exposes kprompt to IDE assistants (Cursor, Claude Desktop,
Windsurf, …) as a **read/plan-only** [Model Context Protocol](https://modelcontextprotocol.io)
tool provider.

kprompt here is an **MCP tool provider**, not an agent platform. Mutating
prompts return a typed `PlanResult` and stop — the MCP surface **never applies a
change to the cluster**. Approval stays a human action you run yourself
(`kprompt "…" --approve` in your own terminal). See
[ADR-0024](https://github.com/kprompt/kprompt-architecture) (MCP interop surface).

## Transport

Newline-delimited JSON-RPC 2.0 over stdio. Your editor spawns
`kprompt mcp serve` and talks to it on stdin/stdout; kprompt's human-readable
output goes to stderr, keeping stdout clean for the protocol.

```bash
kprompt mcp serve
```

## Tools

| Tool | What it does | Mutates? |
|------|--------------|----------|
| `kprompt.read` | Natural-language read against the active kubeconfig (`list pods`, `how many nodes`, `describe api`). Returns `PlanResult` JSON. | No |
| `kprompt.investigate` | Multi-hop root-cause walk (Service→Endpoints→Pods→Events→Logs) for a `target`. | No |
| `kprompt.why` | Causal analysis of why a `target` is failing/pending/crashing. | No |
| `kprompt.timeline` | Chronology of what happened to a `target`. | No |
| `kprompt.impact` | Reverse dependencies / blast radius for a `target`. | No |
| `kprompt.plan` | Compile a mutation prompt into a typed `PlanResult` (actions, diff, risk, blast radius). **Never applies.** Wipe-class intents are hard-denied. | No |
| `kprompt.tools` | List detected integrations (Helm, Argo, Prometheus, OTel, Grafana, GitOps, …) as JSON. | No |
| `kprompt.doctor` | Read-only environment health report (kube, LLM provider, integrations, Team). Never prints API keys. | No |

The reasoning tools (`kprompt.read`, `kprompt.investigate`, `kprompt.why`,
`kprompt.timeline`, `kprompt.impact`, `kprompt.plan`) need a configured LLM
provider (see [providers.md](./providers.md)), because they compile natural
language into intent. They honor your kubeconfig RBAC — no cluster credentials
leave your machine.

`kprompt.plan` is the explicit mutation surface: it returns the plan an
assistant can show you, but applying it stays a human action you run yourself
(`kprompt "…" --approve`). The MCP server exposes no approval path.

## Editor configuration

### Cursor (`~/.cursor/mcp.json` or project `.cursor/mcp.json`)

```json
{
  "mcpServers": {
    "kprompt": {
      "command": "kprompt",
      "args": ["mcp", "serve"]
    }
  }
}
```

### Claude Desktop (`claude_desktop_config.json`)

```json
{
  "mcpServers": {
    "kprompt": {
      "command": "kprompt",
      "args": ["mcp", "serve"]
    }
  }
}
```

Use an absolute path to the `kprompt` binary if it is not on the editor's `PATH`.

## Safety

- **No remote auto-apply.** The server never executes a mutation; `--approve` is
  not reachable over MCP.
- **Hard-denies intact.** Wipe-class / namespace-delete intents are refused
  regardless of caller.
- **Local trust.** stdio transport is scoped to the operator who launched the
  editor; there is no network listener.

## Quick manual check

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  | kprompt mcp serve
```
