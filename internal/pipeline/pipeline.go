package pipeline

import (
	"context"
	"fmt"
	"io"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/executor"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
	"github.com/kprompt/kprompt/internal/ui"
)

// Run executes the full prompt → plan → safety → optional apply flow.
func Run(ctx context.Context, cfg config.Resolved, out io.Writer) error {
	if cfg.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}

	if denied := safety.CheckPrompt(cfg.Prompt); denied.Denied {
		ui.PrintDenied(out, denied.Message)
		return nil
	}

	provider, err := llm.New(cfg.Provider, config.APIKeyFor(cfg.Provider), cfg.BaseURL, cfg.Model)
	if err != nil {
		return err
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

	clients, err := cluster.Connect(cfg.Context)
	if err != nil {
		return err
	}
	runner := &executor.Runner{Client: clients.Clientset}
	if err := runner.Apply(ctx, plan); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	ui.PrintApplied(out, plan)
	return nil
}
