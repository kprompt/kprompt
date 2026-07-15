package pipeline

import (
	"context"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/executor"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
	"github.com/kprompt/kprompt/internal/ui"
)

// Deps allows tests to inject LLM and Kubernetes clients.
type Deps struct {
	Provider llm.Provider
	Client   kubernetes.Interface
}

// Run executes the full prompt → plan → safety → optional apply flow.
func Run(ctx context.Context, cfg config.Resolved, out io.Writer) error {
	return RunWith(ctx, cfg, out, Deps{})
}

// RunWith is like Run but accepts injected dependencies.
func RunWith(ctx context.Context, cfg config.Resolved, out io.Writer, deps Deps) error {
	if cfg.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}

	if denied := safety.CheckPrompt(cfg.Prompt); denied.Denied {
		ui.PrintDenied(out, denied.Message)
		return nil
	}

	provider := deps.Provider
	if provider == nil {
		var err error
		provider, err = llm.New(cfg.Provider, config.APIKeyFor(cfg.Provider), cfg.BaseURL, cfg.Model)
		if err != nil {
			return err
		}
	}

	in, err := intent.Extract(ctx, provider, cfg.Prompt, cfg.Namespace)
	if err != nil {
		return err
	}

	plan, err := planner.Build(in)
	if err != nil {
		return err
	}

	risk := safety.EvaluatePlan(plan)
	if risk.Denied {
		ui.PrintDenied(out, risk.Message)
		return nil
	}

	ui.PrintPlan(out, plan, risk)

	client := deps.Client
	if client == nil {
		clients, err := cluster.Connect(cfg.Context)
		if err != nil {
			return err
		}
		client = clients.Clientset
	}

	// Read-only get/list runs immediately (no --approve).
	if isReadOnly(plan) {
		if plan.Intent.Kind == intent.KindExplain {
			fmt.Fprintln(out, "Explain-lite is not implemented yet (T-004).")
			return nil
		}
		q, err := queryFromPlan(plan)
		if err != nil {
			return err
		}
		res, err := (&cluster.Reader{Client: client}).List(ctx, q)
		if err != nil {
			return fmt.Errorf("query: %w", err)
		}
		ui.PrintQueryResult(out, res)
		return nil
	}

	if !cfg.Approve {
		return nil
	}

	runner := &executor.Runner{Client: client}
	if err := runner.Apply(ctx, plan); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	ui.PrintApplied(out, plan)
	return nil
}

func isReadOnly(plan planner.ExecutionPlan) bool {
	if plan.RequiresApproval {
		return false
	}
	switch plan.Intent.Kind {
	case intent.KindGet, intent.KindExplain:
		return true
	default:
		return false
	}
}

func queryFromPlan(plan planner.ExecutionPlan) (cluster.Query, error) {
	if len(plan.Actions) == 0 {
		return cluster.Query{}, fmt.Errorf("get plan has no actions")
	}
	a := plan.Actions[0]
	q := cluster.Query{
		Kind:      a.Object.Kind,
		Namespace: a.Object.Namespace,
		Name:      a.Object.Name,
	}
	if sel, ok := plan.Intent.LabelSelector(); ok {
		q.LabelSelector = sel
	}
	if mem, ok := plan.Intent.MinMemory(); ok {
		qty, err := resource.ParseQuantity(mem)
		if err != nil {
			return cluster.Query{}, fmt.Errorf("params.minMemory: %w", err)
		}
		q.MinMemory = qty
	}
	return q, nil
}
