package pipeline

import (
	"context"
	"fmt"
	"io"

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

	if !cfg.Approve {
		return nil
	}
	if !plan.RequiresApproval && risk.Risk == safety.RiskLow {
		fmt.Fprintln(out, "Nothing to apply (read-only / no mutation).")
		return nil
	}

	client := deps.Client
	if client == nil {
		clients, err := cluster.Connect(cfg.Context)
		if err != nil {
			return err
		}
		client = clients.Clientset
	}
	runner := &executor.Runner{Client: client}
	if err := runner.Apply(ctx, plan); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	ui.PrintApplied(out, plan)
	return nil
}
