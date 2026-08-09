# Bugbot review rules — kprompt CLI

This repo is **[kprompt/kprompt](https://github.com/kprompt/kprompt)**: the public Go CLI and in-cluster Observe / Namespace Agent (module `github.com/kprompt/kprompt`). Review PRs against the invariants below. Prefer a small number of high-signal findings over noise; skip pure style nits already handled by `gofmt`.

## Safety invariants (highest priority — these are product DNA, ADR-0003)

- **Mutating paths must stay behind PlanResult → safety → explicit approval.** Flag any change under `internal/{pipeline,planner,intent,executor,verify}` that applies cluster changes without going through plan + approval, or that makes `--approve` implicit/default.
- **Hard-deny must hold.** Changes under `internal/safety/**` that weaken or bypass wipe-class / destructive prompt denial are bugs. Deny rules should fail closed.
- **Observe / Namespace Agent is propose-first.** Under `internal/agent/**`, watch/analyze paths must notify/propose and never silently mutate unless an explicit Autopilot gate + policy is present.
- Never widen RBAC in `charts/**` or `deploy/crd/**` beyond what a read-only Observe surface needs without justification.

## Security

- **No secrets in the tree or logs.** Flag committed API keys, tokens, kubeconfigs, or code that prints/logs secret values (provider keys, `kp_…` tokens, bearer headers). Env vars override pulled keys — do not log either.
- **Exec safety (see SEC-006).** Commands must be executed via `execve`-style argv, never through a host shell. Reject anything that routes user/model-controlled strings into `/bin/sh -c`, `bash -lc`, etc. When validating shell launchers, the check must inspect the combined `command`+`args` argv (not a single fixed index) and cover flag clusters carrying `-c` (`-lc`, `-ec`, `-cx`, …).
- Argument/URL allowlists should be validated token-by-token (exact argv), not by substring `Contains`.
- Watch for path traversal, SSRF (control-plane/API URLs), and unbounded reads when handling untrusted input.

## Go correctness & style

- Match neighboring style; assume `gofmt` runs via hook — don't report formatting-only nits.
- Wrap errors with context (`fmt.Errorf("...: %w", err)`); don't swallow errors or `panic` in library/`internal` code.
- Propagate `context.Context`; no `context.Background()` in request/command paths that already have one.
- Guard against nil derefs on k8s objects and map/slice access; check type assertions.
- Goroutines must have clear lifetimes (no leaks); protect shared state.
- `client-go` usage: prefer typed informers/clients already used in `internal/cluster`; avoid per-call client construction in hot paths.

## CLI / Cobra conventions (`cmd/kprompt`)

- New flags or behavior changes **must** update matching `docs/*.md` and command help text (docs-sync). Flag a PR that changes a flag but not its docs.
- `Example:` blocks: keep every line indented with the same leading two spaces; reference only flags the command actually registers.
- Keep one concern per PR.

## CI / workflows (`.github/workflows/**`)

- GitHub Actions **must be pinned to a full commit SHA** with a trailing `# vX` comment (repo policy). Flag tag-only or branch refs.
- Go version should come from `go.mod` (`go-version-file: go.mod`), not a hardcoded `go-version` that can drift from the module's `go` directive.
- Don't run steps needing repo secrets (e.g. SonarCloud `SONAR_TOKEN`) on Dependabot or forked-PR events where secrets are unavailable.

## Out of scope for this repo

Marketing site, Team app/admin UI, control-plane API, and architecture markdown boards live in other repos — don't request changes to them here.
