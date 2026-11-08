# Release assurance (green board)

**Job:** prove the shipped product still works — not invent features.  
PR CI stays fast; **tag / PH / public demos** wait on the nightly + pre-release board.

```text
PR (required)          Nightly / weekly           Pre-release (manual)
─────────────────      ─────────────────          ────────────────────
go test ./...          cold-coach                 same as nightly/weekly
kind e2e smoke         magic-moment               + confirm Actions green
                       PlanResult jq              then: git tag / release
                       (weekly) full e2e
                       (weekly) learn-second-hit
```

## Green board

| Gate | Workflow | When | Required for merge? |
|------|----------|------|---------------------|
| Unit + coverage | [ci.yml](../.github/workflows/ci.yml) `test` | every PR / push | **Yes** |
| Kind e2e smoke | `ci.yml` `e2e` | every PR / push (not Dependabot) | **Yes** |
| Kind e2e **full** | `ci.yml` `e2e` | **weekly cron** + `workflow_dispatch` → `full` | No (release blocker) |
| Cold coach | [magic-moment.yml](../.github/workflows/magic-moment.yml) | daily + dispatch | No (release blocker) |
| Magic-moment | same | daily + dispatch | No (release blocker) |
| Learn second-hit | same | **weekly cron** + dispatch flag | No (release blocker) |
| PlanResult jq | same `planresult-jq` job | daily + dispatch | No (release blocker) |
| Examples manifests | [kprompt-examples validate](https://github.com/kprompt/kprompt-examples/actions) | examples PR | examples merge |

## Local assure

From this repo:

```bash
./scripts/assure.sh           # unit + deny jq; kind smoke if kind/kubectl present
./scripts/assure.sh --unit    # go test ./... only
./scripts/assure.sh --jq      # build + PlanResult hard-deny jq only
```

Kind smoke (optional, same subset as CI):

```bash
go test -tags=e2e ./test/e2e/ \
  -run 'TestGenericReadMatrixOnKind|TestDeployRedisOnKind|TestAgentWatchPodsEvents' \
  -count=1 -v -timeout 10m
```

Magic-moment (optional, needs Docker + [kprompt-examples](https://github.com/kprompt/kprompt-examples)):

```bash
go build -o bin/kprompt ./cmd/kprompt
cd ../kprompt-examples && MAGIC_MOMENT_APPROVE=1 KPROMPT=$PWD/../kprompt/bin/kprompt make magic-moment
```

## Pre-release checklist

Before `git tag v*`:

1. [ ] Latest **ci** on `main` green (unit + smoke)
2. [ ] Trigger full e2e if weekly has not run recently:

   ```bash
   gh workflow run ci.yml -f run_full_e2e_tests=full
   ```

3. [ ] Trigger magic-moment board (includes cold-coach + jq; add Learn if desired):

   ```bash
   gh workflow run magic-moment.yml -f demo_seconds=28 -f run_learn_second_hit=true
   ```

4. [ ] Confirm both runs success in Actions
5. [ ] Then tag / GoReleaser ([release.yml](../.github/workflows/release.yml))

## Honesty

- These gates do **not** replace reviewing PlanResults on real clusters.
- Magic-moment / Learn use **heuristic** analysis (`$0`, no LLM key).
- PlanResult jq here locks the **hard-deny** binary contract; customer cookbook scale gates still need *their* kubeconfig + LLM ([ci-cookbook.md](./ci-cookbook.md)).
- Do not make magic-moment or full e2e required PR checks (flake / minute cost).

## Related

- [e2e.md](./e2e.md) · [demo.md](./demo.md) · [ci.md](./ci.md) · [ci-cookbook.md](./ci-cookbook.md)
- Scripts: [assure.sh](../scripts/assure.sh) · [planresult-jq-gate.sh](../scripts/planresult-jq-gate.sh)
