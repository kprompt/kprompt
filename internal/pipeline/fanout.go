package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/output"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
	"github.com/kprompt/kprompt/internal/ui"
)

func fanOutContexts(cfg config.Resolved) []string {
	if len(cfg.Contexts) > 1 {
		return append([]string(nil), cfg.Contexts...)
	}
	return nil
}

func supportsReadFanOut(kind intent.Kind) bool {
	switch kind {
	case intent.KindGet, intent.KindExplain, intent.KindLogs, intent.KindDescribe:
		return true
	default:
		return false
	}
}

func runMultiContextFanOut(
	ctx context.Context,
	cfg config.Resolved,
	out io.Writer,
	deps Deps,
	provider llm.Provider,
	plan planner.ExecutionPlan,
	risk safety.Result,
	contexts []string,
) error {
	if plan.RequiresApproval || !isReadOnly(plan) {
		return runMultiContextMutates(ctx, cfg, out, deps, provider, plan, risk, contexts)
	}
	return runMultiContextReads(ctx, cfg, out, deps, provider, plan, risk, contexts)
}

func runMultiContextReads(
	ctx context.Context,
	cfg config.Resolved,
	out io.Writer,
	deps Deps,
	provider llm.Provider,
	plan planner.ExecutionPlan,
	risk safety.Result,
	contexts []string,
) error {
	jsonMode := cfg.JSONOutput()
	human := out
	if jsonMode {
		human = os.Stderr
	}

	if !supportsReadFanOut(plan.Intent.Kind) {
		msg := fmt.Sprintf(
			"multi-context fan-out supports get/list/explain/logs/describe only (got %s)",
			plan.Intent.Kind,
		)
		return denyFanOut(out, deps, cfg, plan, jsonMode, msg)
	}

	if !jsonMode && !cfg.FanOutChild {
		ui.PrintPlan(human, plan, risk)
		fmt.Fprintf(human, "Fan-out across %d contexts (read-only).\n", len(contexts))
	}

	result, err := fanOutSteps(ctx, cfg, human, deps, provider, plan, contexts, false)
	if err != nil {
		return err
	}
	if jsonMode {
		return output.EncodeMultiContext(out, result)
	}
	return nil
}

func runMultiContextMutates(
	ctx context.Context,
	cfg config.Resolved,
	out io.Writer,
	deps Deps,
	provider llm.Provider,
	plan planner.ExecutionPlan,
	risk safety.Result,
	contexts []string,
) error {
	jsonMode := cfg.JSONOutput()
	human := out
	if jsonMode {
		human = os.Stderr
	}

	// Plain --approve must never silently mutate every listed context (ADR-0012).
	if cfg.Approve && !cfg.ApproveEachContext {
		msg := "refusing --approve across multiple contexts — confirm each context interactively, or pass --approve-each-context"
		return denyFanOut(out, deps, cfg, plan, jsonMode, msg)
	}
	if !cfg.Approve && !cfg.ApproveEachContext {
		isTTY := ui.StdinIsTerminal()
		if deps.IsTerminal != nil {
			isTTY = *deps.IsTerminal
		}
		if !isTTY && deps.Confirm == nil {
			if jsonMode {
				return denyFanOut(out, deps, cfg, plan, true, "multi-context mutate requires a TTY per-context confirm or --approve-each-context")
			}
			ui.PrintNeedsApproveEachContext(out)
			doc := output.FromPlan(cfg.Prompt, "", plan, safety.Result{
				Risk:    safety.RiskDenied,
				Denied:  true,
				Message: "multi-context mutate requires per-context approval",
			}, false)
			if deps.OnResult != nil {
				deps.OnResult(doc)
			}
			return nil
		}
	}

	if !jsonMode && !cfg.FanOutChild {
		ui.PrintPlan(human, plan, risk)
		fmt.Fprintf(human, "Multi-context mutate across %d contexts — approval is per context.\n", len(contexts))
	}

	result, err := fanOutSteps(ctx, cfg, human, deps, provider, plan, contexts, true)
	if err != nil {
		return err
	}
	if jsonMode {
		return output.EncodeMultiContext(out, result)
	}
	return nil
}

func denyFanOut(
	out io.Writer,
	deps Deps,
	cfg config.Resolved,
	plan planner.ExecutionPlan,
	jsonMode bool,
	msg string,
) error {
	denied := safety.Result{Risk: safety.RiskDenied, Denied: true, Message: msg}
	doc := output.FromPlan(cfg.Prompt, "", plan, denied, false)
	if deps.OnResult != nil {
		deps.OnResult(doc)
	}
	if jsonMode {
		return output.Encode(out, doc)
	}
	ui.PrintDenied(out, msg)
	return nil
}

func fanOutSteps(
	ctx context.Context,
	cfg config.Resolved,
	human io.Writer,
	deps Deps,
	provider llm.Provider,
	plan planner.ExecutionPlan,
	contexts []string,
	mutating bool,
) (output.MultiContextResult, error) {
	in := plan.Intent
	in.Raw = cfg.Prompt
	rawIntent, err := json.Marshal(in)
	if err != nil {
		return output.MultiContextResult{}, err
	}
	provName := "fanout"
	if provider != nil {
		provName = provider.Name()
	}

	result := output.MultiContextResult{
		APIVersion:    output.APIVersion,
		Kind:          output.KindMultiContextResult,
		SchemaVersion: output.SchemaVersion,
		Prompt:        cfg.Prompt,
		Contexts:      append([]string(nil), contexts...),
		Applied:       true,
		Steps:         make([]output.PlanResult, 0, len(contexts)),
	}
	jsonMode := cfg.JSONOutput()

	for i, cName := range contexts {
		if !jsonMode {
			ui.PrintContextSection(human, cName, i+1, len(contexts))
		}

		stepApprove := false
		if mutating {
			if cfg.ApproveEachContext {
				stepApprove = true
			} else {
				ok, err := resolveApprovalForContext(cName, human, deps)
				if err != nil {
					return result, err
				}
				if !ok {
					result.Applied = false
					skip := output.FromPlan(cfg.Prompt, cName, plan, safety.Result{
						Risk:    riskLevel(plan),
						Denied:  false,
						Message: "skipped — not approved for this context",
					}, false)
					result.Steps = append(result.Steps, skip)
					continue
				}
				stepApprove = true
			}
		}

		stepCfg := cfg
		stepCfg.Context = cName
		stepCfg.Contexts = nil
		stepCfg.ContextAlias = ""
		stepCfg.ContextFromCLI = true
		stepCfg.FanOutChild = true
		stepCfg.Approve = stepApprove
		stepCfg.ApproveEachContext = false
		stepCfg.Output = "text"

		var stepDoc output.PlanResult
		observed := false
		stepDeps := deps
		stepDeps.Provider = &fixedIntentProvider{name: provName, raw: append(json.RawMessage(nil), rawIntent...)}
		if deps.Client == nil {
			stepDeps.Client = nil
			stepDeps.Dynamic = nil
			stepDeps.Resolver = nil
		}
		// Child already approved at fan-out layer.
		stepDeps.Confirm = func(io.Writer) (bool, error) { return true, nil }
		stepDeps.OnResult = func(doc output.PlanResult) {
			stepDoc = doc
			observed = true
		}
		runErr := RunWith(ctx, stepCfg, human, stepDeps)
		if runErr != nil {
			result.Applied = false
			errDoc := output.FromPlan(cfg.Prompt, cName, plan, safety.Result{
				Risk:    safety.RiskDenied,
				Denied:  true,
				Message: runErr.Error(),
			}, false)
			result.Steps = append(result.Steps, errDoc)
			if !jsonMode {
				ui.PrintDenied(human, fmt.Sprintf("%s: %v", cName, runErr))
			}
			continue
		}
		if !observed {
			result.Applied = false
			result.Steps = append(result.Steps, output.FromPlan(cfg.Prompt, cName, plan, safety.Result{
				Risk:    safety.RiskDenied,
				Denied:  true,
				Message: "no result from context",
			}, false))
			continue
		}
		if stepDoc.Plan.Context == "" {
			stepDoc.Plan.Context = cName
		}
		if stepDoc.ClusterContext == "" {
			stepDoc.ClusterContext = cName
		}
		if stepDoc.Risk.Denied || (mutating && !stepDoc.Applied) {
			result.Applied = false
		}
		result.Steps = append(result.Steps, stepDoc)
	}
	return result, nil
}

func riskLevel(plan planner.ExecutionPlan) safety.Risk {
	if plan.RequiresApproval {
		return safety.RiskMedium
	}
	return safety.RiskLow
}

func resolveApprovalForContext(contextName string, out io.Writer, deps Deps) (bool, error) {
	if deps.Confirm != nil {
		ok, err := deps.Confirm(out)
		if err != nil {
			return false, err
		}
		if !ok {
			ui.PrintAborted(out)
		}
		return ok, nil
	}
	isTTY := ui.StdinIsTerminal()
	if deps.IsTerminal != nil {
		isTTY = *deps.IsTerminal
	}
	if !isTTY {
		ui.PrintNeedsApproveEachContext(out)
		return false, nil
	}
	ok, err := ui.ConfirmApplyContext(os.Stdin, out, contextName)
	if err != nil {
		return false, err
	}
	if !ok {
		ui.PrintAborted(out)
	}
	return ok, nil
}

type fixedIntentProvider struct {
	name string
	raw  json.RawMessage
	used bool
}

func (p *fixedIntentProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "fixed-intent"
}

func (p *fixedIntentProvider) Complete(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{}, nil
}

func (p *fixedIntentProvider) CompleteStructured(
	context.Context,
	llm.CompletionRequest,
	json.RawMessage,
) (json.RawMessage, error) {
	if p.used {
		return nil, fmt.Errorf("fixed intent provider already consumed")
	}
	p.used = true
	return p.raw, nil
}
