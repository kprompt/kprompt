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

type routePreflight struct {
	Intents []json.RawMessage
	Plans   []planner.ExecutionPlan
	Risks   []safety.Result
}

func runRoute(
	ctx context.Context,
	cfg config.Resolved,
	out io.Writer,
	deps Deps,
	steps []string,
) error {
	prepared, err := prepareRoute(ctx, cfg, deps.Provider, steps)
	if err != nil {
		return err
	}
	deps.Provider = &preparedRouteProvider{
		name:       deps.Provider.Name(),
		structured: prepared.Intents,
	}
	result := newRouteResult(cfg.Prompt, len(steps))
	result.RequiresApproval = routeNeedsApproval(prepared.Plans)
	result.Risk = output.RiskPayload{
		Level:   string(aggregateRouteRisk(prepared.Risks).Risk),
		Denied:  false,
		Message: aggregateRouteRisk(prepared.Risks).Message,
	}
	jsonMode := cfg.JSONOutput()
	human := routeWriter(out, jsonMode)
	if !jsonMode {
		ui.PrintRoute(out, steps)
		ui.PrintRoutePlan(out, steps, prepared.Plans, prepared.Risks)
	}

	routeCfg := cfg
	if result.RequiresApproval {
		approved, err := resolveApproval(cfg.Approve, human, deps)
		if err != nil {
			return err
		}
		if !approved {
			first := firstMutatingStep(prepared.Plans)
			stopRoute(&result, first, "route was not approved")
			if !jsonMode {
				ui.PrintRouteStopped(out, result.StoppedAt, result.StopReason)
			}
			if jsonMode {
				return output.EncodeRoute(out, result)
			}
			return nil
		}
		// One consent covers every mutating step in the chain (T-058).
		routeCfg.Approve = true
	}

	for index, prompt := range steps {
		if !jsonMode {
			ui.PrintRouteStep(out, index+1, len(steps), prompt)
		}
		stepResult, err := runRouteStep(
			ctx,
			routeCfg,
			human,
			deps,
			index+1,
			prompt,
		)
		if err != nil {
			return err
		}
		result.Steps = append(result.Steps, stepResult)
		carryRouteScope(&routeCfg, stepResult)
		if reason := routeStopReason(stepResult); reason != "" {
			stopRoute(&result, index+1, reason)
			if !jsonMode {
				ui.PrintRouteStopped(out, result.StoppedAt, result.StopReason)
			}
			break
		}
	}
	if len(result.Steps) != len(steps) {
		result.Applied = false
	}
	if jsonMode {
		return output.EncodeRoute(out, result)
	}
	return nil
}

func prepareRoute(
	ctx context.Context,
	cfg config.Resolved,
	provider llm.Provider,
	steps []string,
) (routePreflight, error) {
	out := routePreflight{
		Intents: make([]json.RawMessage, 0, len(steps)),
		Plans:   make([]planner.ExecutionPlan, 0, len(steps)),
		Risks:   make([]safety.Result, 0, len(steps)),
	}
	for index, prompt := range steps {
		in, err := intent.Extract(ctx, provider, prompt)
		if err != nil {
			return routePreflight{}, fmt.Errorf("route step %d: %w", index+1, err)
		}
		in = intent.ApplyScope(in, intent.ScopePrefs{
			DefaultNamespace: cfg.Namespace,
			DefaultContext:   cfg.Context,
			ForceNamespace:   cfg.NamespaceFromCLI,
			ForceContext:     cfg.ContextFromCLI,
		})
		in = intent.NormalizeVerb(in, prompt)
		in = intent.ApplyOptimizeScope(in, prompt, intent.ScopePrefs{
			ForceNamespace: cfg.NamespaceFromCLI,
		})
		in = intent.ApplyGraphScope(in, prompt, intent.ScopePrefs{
			ForceNamespace: cfg.NamespaceFromCLI,
		})
		plan, err := planner.Build(in)
		if err != nil {
			return routePreflight{}, fmt.Errorf("route step %d: %w", index+1, err)
		}
		risk := safety.EvaluatePlanWithOrg(plan, loadOrgPolicy())
		if risk.Denied {
			return routePreflight{}, fmt.Errorf(
				"route step %d denied by safety policy: %s",
				index+1,
				risk.Message,
			)
		}
		raw, err := json.Marshal(in)
		if err != nil {
			return routePreflight{}, fmt.Errorf("route step %d intent: %w", index+1, err)
		}
		out.Intents = append(out.Intents, raw)
		out.Plans = append(out.Plans, plan)
		out.Risks = append(out.Risks, risk)
		carryIntentScope(&cfg, in)
	}
	return out, nil
}

func routeNeedsApproval(plans []planner.ExecutionPlan) bool {
	for _, plan := range plans {
		if plan.RequiresApproval {
			return true
		}
	}
	return false
}

func firstMutatingStep(plans []planner.ExecutionPlan) int {
	for i, plan := range plans {
		if plan.RequiresApproval {
			return i + 1
		}
	}
	return 1
}

func aggregateRouteRisk(risks []safety.Result) safety.Result {
	if len(risks) == 0 {
		return safety.Result{Risk: safety.RiskLow}
	}
	rank := map[safety.Risk]int{
		safety.RiskLow:    1,
		safety.RiskMedium: 2,
		safety.RiskHigh:   3,
		safety.RiskDenied: 4,
	}
	best := risks[0]
	for _, r := range risks[1:] {
		if rank[r.Risk] > rank[best.Risk] {
			best = r
		}
	}
	if best.Message == "" && best.Risk == safety.RiskMedium {
		best.Message = "Mutation requires approval"
	}
	if best.Message == "" && best.Risk == safety.RiskHigh {
		best.Message = "High-risk mutation requires approval"
	}
	return best
}

type preparedRouteProvider struct {
	name       string
	structured []json.RawMessage
	index      int
}

func (p *preparedRouteProvider) Name() string {
	return p.name
}

func (p *preparedRouteProvider) Complete(
	context.Context,
	llm.CompletionRequest,
) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{}, nil
}

func (p *preparedRouteProvider) CompleteStructured(
	_ context.Context,
	_ llm.CompletionRequest,
	_ json.RawMessage,
) (json.RawMessage, error) {
	if p.index >= len(p.structured) {
		return nil, fmt.Errorf(
			"route provider exhausted after %d steps",
			len(p.structured),
		)
	}
	result := p.structured[p.index]
	p.index++
	return result, nil
}

func newRouteResult(prompt string, capacity int) output.RouteResult {
	return output.RouteResult{
		APIVersion:    output.APIVersion,
		Kind:          output.KindRouteResult,
		SchemaVersion: output.SchemaVersion,
		Prompt:        prompt,
		Applied:       true,
		Steps:         make([]output.PlanResult, 0, capacity),
	}
}

func routeWriter(out io.Writer, jsonMode bool) io.Writer {
	if jsonMode {
		return os.Stderr
	}
	return out
}

func runRouteStep(
	ctx context.Context,
	cfg config.Resolved,
	out io.Writer,
	deps Deps,
	index int,
	prompt string,
) (output.PlanResult, error) {
	cfg.Prompt = prompt
	cfg.Output = "text"
	var (
		result   output.PlanResult
		observed bool
	)
	parentObserver := deps.OnResult
	deps.OnResult = func(doc output.PlanResult) {
		result = doc
		observed = true
		if parentObserver != nil {
			parentObserver(doc)
		}
	}
	if err := RunWith(ctx, cfg, out, deps); err != nil {
		return output.PlanResult{}, fmt.Errorf("route step %d: %w", index, err)
	}
	if !observed {
		return output.PlanResult{}, fmt.Errorf(
			"route step %d completed without a result",
			index,
		)
	}
	return result, nil
}

func carryIntentScope(cfg *config.Resolved, in intent.Intent) {
	if in.Target.Namespace != "" {
		cfg.Namespace = in.Target.Namespace
	}
	if in.Context != "" {
		cfg.Context = in.Context
	}
}

func carryRouteScope(cfg *config.Resolved, result output.PlanResult) {
	if result.Plan.Namespace != "" {
		cfg.Namespace = result.Plan.Namespace
	}
	if result.Plan.Context != "" {
		cfg.Context = result.Plan.Context
	}
}

func routeStopReason(result output.PlanResult) string {
	if result.Risk.Denied {
		return "step denied by safety policy"
	}
	if result.Applied {
		return ""
	}
	if result.Plan.RequiresApproval {
		return "step was not approved"
	}
	return "step did not complete"
}

func stopRoute(result *output.RouteResult, index int, reason string) {
	result.Applied = false
	result.StoppedAt = index
	result.StopReason = reason
}
