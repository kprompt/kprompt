package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"

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

// ConfirmFunc asks the user whether to apply a mutating plan.
type ConfirmFunc func(out io.Writer) (bool, error)

// Deps allows tests to inject LLM, Kubernetes clients, and approval behavior.
type Deps struct {
	Provider   llm.Provider
	Client     kubernetes.Interface
	Confirm    ConfirmFunc // if set, used instead of TTY prompt
	IsTerminal *bool       // override ui.StdinIsTerminal when non-nil
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

	client := deps.Client
	if client == nil {
		clients, err := cluster.Connect(cfg.Context)
		if err != nil {
			return err
		}
		client = clients.Clientset
	}

	if plan.RequiresApproval {
		planner.EnrichDiffs(ctx, client, &plan)
	}

	ui.PrintPlan(out, plan, risk)

	// Read-only paths run immediately (no --approve).
	if isReadOnly(plan) {
		switch plan.Intent.Kind {
		case intent.KindExplain:
			req, err := explainFromPlan(plan)
			if err != nil {
				return err
			}
			rep, err := (&cluster.Explainer{Client: client}).Explain(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("explain: %w", err))
			}
			ui.PrintExplain(out, rep)
			return nil
		case intent.KindLogs:
			req, err := logsFromPlan(plan)
			if err != nil {
				return err
			}
			res, err := (&cluster.LogReader{Client: client}).Logs(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("logs: %w", err))
			}
			ui.PrintLogs(out, res)
			return nil
		case intent.KindDescribe:
			req, err := describeFromPlan(plan)
			if err != nil {
				return err
			}
			rep, err := (&cluster.Describer{Client: client}).Describe(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("describe: %w", err))
			}
			ui.PrintDescribe(out, rep)
			return nil
		case intent.KindGet:
			q, err := queryFromPlan(plan)
			if err != nil {
				return err
			}
			res, err := (&cluster.Reader{Client: client}).List(ctx, q)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("query: %w", err))
			}
			ui.PrintQueryResult(out, res)
			return nil
		}
	}

	approved, err := resolveApproval(cfg.Approve, out, deps)
	if err != nil {
		return err
	}
	if !approved {
		return nil
	}

	runner := &executor.Runner{Client: client}
	if err := runner.Apply(ctx, plan); err != nil {
		return cluster.Friendlier(fmt.Errorf("apply: %w", err))
	}
	ui.PrintApplied(out, plan)

	if cfg.Wait {
		targets := deploymentWaitTargets(plan)
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = cluster.DefaultWaitTimeout
		}
		waiter := &cluster.Waiter{Client: client, Out: out}
		for _, t := range targets {
			if err := waiter.WaitDeployment(ctx, t.Namespace, t.Name, timeout); err != nil {
				return cluster.Friendlier(err)
			}
		}
	}
	return nil
}

func deploymentWaitTargets(plan planner.ExecutionPlan) []planner.ObjectRef {
	seen := map[string]struct{}{}
	var out []planner.ObjectRef
	for _, a := range plan.Actions {
		switch a.Op {
		case planner.OpScale, planner.OpRollback, planner.OpCreate, planner.OpUpdate:
			if a.Object.Kind != "Deployment" && a.Object.Kind != "" {
				continue
			}
			key := a.Object.Namespace + "/" + a.Object.Name
			if a.Object.Name == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			ref := a.Object
			if ref.Kind == "" {
				ref.Kind = "Deployment"
			}
			out = append(out, ref)
		}
	}
	return out
}

func resolveApproval(flagApprove bool, out io.Writer, deps Deps) (bool, error) {
	if flagApprove {
		return true, nil
	}
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
		ui.PrintNeedsApprove(out)
		return false, nil
	}
	ok, err := ui.ConfirmApply(os.Stdin, out)
	if err != nil {
		return false, err
	}
	if !ok {
		ui.PrintAborted(out)
	}
	return ok, nil
}

func isReadOnly(plan planner.ExecutionPlan) bool {
	if plan.RequiresApproval {
		return false
	}
	switch plan.Intent.Kind {
	case intent.KindGet, intent.KindExplain, intent.KindLogs, intent.KindDescribe:
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

func explainFromPlan(plan planner.ExecutionPlan) (cluster.ExplainRequest, error) {
	if len(plan.Actions) == 0 {
		return cluster.ExplainRequest{}, fmt.Errorf("explain plan has no actions")
	}
	a := plan.Actions[0]
	if a.Object.Name == "" {
		return cluster.ExplainRequest{}, fmt.Errorf("explain requires a named target")
	}
	return cluster.ExplainRequest{
		Name:      a.Object.Name,
		Namespace: a.Object.Namespace,
		Kind:      a.Object.Kind,
	}, nil
}

func logsFromPlan(plan planner.ExecutionPlan) (cluster.LogsRequest, error) {
	if len(plan.Actions) == 0 {
		return cluster.LogsRequest{}, fmt.Errorf("logs plan has no actions")
	}
	a := plan.Actions[0]
	if a.Object.Name == "" {
		return cluster.LogsRequest{}, fmt.Errorf("logs requires a named target")
	}
	req := cluster.LogsRequest{
		Name:      a.Object.Name,
		Namespace: a.Object.Namespace,
		Kind:      a.Object.Kind,
		Tail:      100,
	}
	if t, ok := plan.Intent.TailLines(); ok && t > 0 {
		req.Tail = t
	}
	if c, ok := plan.Intent.Container(); ok {
		req.Container = c
	}
	return req, nil
}

func describeFromPlan(plan planner.ExecutionPlan) (cluster.DescribeRequest, error) {
	if len(plan.Actions) == 0 {
		return cluster.DescribeRequest{}, fmt.Errorf("describe plan has no actions")
	}
	a := plan.Actions[0]
	if a.Object.Name == "" {
		return cluster.DescribeRequest{}, fmt.Errorf("describe requires a named target")
	}
	return cluster.DescribeRequest{
		Name:      a.Object.Name,
		Namespace: a.Object.Namespace,
		Kind:      a.Object.Kind,
	}, nil
}
