# IDE PlanResult (A-076)

VS Code extension that shows a **PlanResult** (risk + action diffs) and can
hand off **approve** to the local `kprompt` CLI. Not a chat panel and not an
in-IDE apiserver client.

## Extension path

Source: [`ide/vscode/`](../ide/vscode/) in this repo.

```bash
cd ide/vscode && npm install && npm run compile
code --install-extension .
```

## Flow

```text
kprompt "…" -o json  →  Open PlanResult  →  review diffs  →  Approve via CLI
                                                      └─ terminal: kprompt '…' --approve
```

Same bridge as MCP: the editor shows a PlanResult; apply stays a local CLI
`--approve` you run yourself. See [mcp.md](./mcp.md) (Bridge story).

## Non-goals (MVP)

- Chat / Copilot-style REPL inside the editor
- Calling `api.kprompt.ai` or applying without the CLI
- JetBrains (later)
- Any apply path that bypasses the CLI (including over MCP)

See [ide/vscode/README.md](../ide/vscode/README.md).
