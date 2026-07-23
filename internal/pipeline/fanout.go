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

	if plan.RequiresApproval || !isReadOnly(plan) {
		msg := "multi-context fan-out refuses mutating plans — run one context at a time (or wait for per-context approve)"
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
	if !supportsReadFanOut(plan.Intent.Kind) {
		msg := fmt.Sprintf(
			"multi-context fan-out supports get/list/explain/logs/describe only (got %s)",
			plan.Intent.Kind,
		)
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

	if !jsonMode && !cfg.FanOutChild {
		ui.PrintPlan(human, plan, risk)
		fmt.Fprintf(human, "Fan-out across %d contexts (read-only).\n", len(contexts))
	}

	in := plan.Intent
	in.Raw = cfg.Prompt
	rawIntent, err := json.Marshal(in)
	if err != nil {
		return err
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

	for i, cName := range contexts {
		if !jsonMode {
			ui.PrintContextSection(human, cName, i+1, len(contexts))
		}
		stepCfg := cfg
		stepCfg.Context = cName
		stepCfg.Contexts = nil
		stepCfg.ContextAlias = ""
		stepCfg.ContextFromCLI = true
		stepCfg.FanOutChild = true
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
		if stepDoc.Risk.Denied {
			result.Applied = false
		}
		result.Steps = append(result.Steps, stepDoc)
	}

	if jsonMode {
		return output.EncodeMultiContext(out, result)
	}
	return nil
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
