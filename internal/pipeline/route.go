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
		structured: prepared,
	}
	result := newRouteResult(cfg.Prompt, len(steps))
	jsonMode := cfg.JSONOutput()
	if !jsonMode {
		ui.PrintRoute(out, steps)
	}

	routeCfg := cfg
	for index, prompt := range steps {
		if !jsonMode {
			ui.PrintRouteStep(out, index+1, len(steps), prompt)
		}
		stepResult, err := runRouteStep(
			ctx,
			routeCfg,
			routeWriter(out, jsonMode),
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
) ([]json.RawMessage, error) {
	prepared := make([]json.RawMessage, 0, len(steps))
	for index, prompt := range steps {
		in, err := intent.Extract(ctx, provider, prompt)
		if err != nil {
			return nil, fmt.Errorf("route step %d: %w", index+1, err)
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
		plan, err := planner.Build(in)
		if err != nil {
			return nil, fmt.Errorf("route step %d: %w", index+1, err)
		}
		if risk := safety.EvaluatePlan(plan); risk.Denied {
			return nil, fmt.Errorf(
				"route step %d denied by safety policy: %s",
				index+1,
				risk.Message,
			)
		}
		raw, err := json.Marshal(in)
		if err != nil {
			return nil, fmt.Errorf("route step %d intent: %w", index+1, err)
		}
		prepared = append(prepared, raw)
		carryIntentScope(&cfg, in)
	}
	return prepared, nil
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
