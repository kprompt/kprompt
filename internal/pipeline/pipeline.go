package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/executor"
	"github.com/kprompt/kprompt/internal/history"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/optimize"
	"github.com/kprompt/kprompt/internal/output"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
	"github.com/kprompt/kprompt/internal/suggest"
	"github.com/kprompt/kprompt/internal/tools"
	"github.com/kprompt/kprompt/internal/tools/argo"
	toolgrafana "github.com/kprompt/kprompt/internal/tools/grafana"
	toolotel "github.com/kprompt/kprompt/internal/tools/otel"
	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
	"github.com/kprompt/kprompt/internal/ui"
)

// ConfirmFunc asks the user whether to apply a mutating plan.
type ConfirmFunc func(out io.Writer) (bool, error)

// Deps allows tests to inject LLM, Kubernetes clients, and approval behavior.
type Deps struct {
	Provider   llm.Provider
	Client     kubernetes.Interface
	Dynamic    dynamic.Interface       // optional; built from rest config when unset (T-050)
	Resolver   *cluster.Resolver       // optional discovery resolver (T-049); built from rest config when unset
	Prometheus toolprometheus.Querier
	OTel       toolotel.Querier
	Grafana    toolgrafana.Querier
	Confirm    ConfirmFunc             // if set, used instead of TTY prompt
	IsTerminal *bool                   // override ui.StdinIsTerminal when non-nil
	OnResult   func(output.PlanResult) // optional per-plan completion observer
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
	jsonMode := cfg.JSONOutput()
	human := out
	if jsonMode {
		human = os.Stderr
	}

	if denied := safety.CheckPrompt(cfg.Prompt); denied.Denied {
		doc := output.PlanResult{
			APIVersion:    output.APIVersion,
			Kind:          output.KindPlanResult,
			SchemaVersion: output.SchemaVersion,
			Prompt:        cfg.Prompt,
			Risk: output.RiskPayload{
				Level:   string(safety.RiskDenied),
				Denied:  true,
				Message: denied.Message,
			},
		}
		if deps.OnResult != nil {
			deps.OnResult(doc)
		}
		if jsonMode {
			return output.Encode(out, doc)
		}
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

	routeSteps := intent.SplitRoutePrompt(cfg.Prompt)
	if len(routeSteps) > intent.MaxRouteSteps {
		return fmt.Errorf(
			"route has %d steps; maximum is %d",
			len(routeSteps),
			intent.MaxRouteSteps,
		)
	}
	if len(routeSteps) > 1 {
		deps.Provider = provider
		return runRoute(ctx, cfg, out, deps, routeSteps)
	}

	in, err := intent.Extract(ctx, provider, cfg.Prompt)
	if err != nil {
		return err
	}
	in = intent.ApplyScope(in, intent.ScopePrefs{
		DefaultNamespace: cfg.Namespace,
		DefaultContext:   cfg.Context,
		ForceNamespace:   cfg.NamespaceFromCLI,
		ForceContext:     cfg.ContextFromCLI,
	})
	in = intent.NormalizeVerb(in, cfg.Prompt)
	in = intent.ApplyOptimizeScope(in, cfg.Prompt, intent.ScopePrefs{
		DefaultNamespace: cfg.Namespace,
		ForceNamespace:   cfg.NamespaceFromCLI,
	})
	cfg.Namespace = in.Target.Namespace
	if in.Context != "" {
		cfg.Context = in.Context
	}

	if intent.LooksLikeWorkflowPrompt(cfg.Prompt) || in.Kind == intent.KindWorkflow {
		if err := tools.RequireArgoWorkflows(ctx, cfg.Context, nil); err != nil {
			return err
		}
	}

	plan, err := planner.Build(in)
	if err != nil {
		return err
	}

	risk := safety.EvaluatePlan(plan)
	if risk.Denied {
		doc := output.FromPlan(cfg.Prompt, cfg.Context, plan, risk, false)
		if deps.OnResult != nil {
			deps.OnResult(doc)
		}
		if jsonMode {
			return output.Encode(out, doc)
		}
		ui.PrintDenied(out, risk.Message)
		return nil
	}

	client := deps.Client
	var restCfg *rest.Config
	if client == nil &&
		plan.Intent.Kind != intent.KindPerformance &&
		plan.Intent.Kind != intent.KindTrace &&
		plan.Intent.Kind != intent.KindDashboard {
		if cfg.Context != "" {
			if err := cluster.EnsureContext(cfg.Context); err != nil {
				return err
			}
		}
		clients, err := cluster.Connect(cfg.Context)
		if err != nil {
			return err
		}
		client = clients.Clientset
		restCfg = clients.Config
	}

	if plan.RequiresApproval {
		if executor.IsHelmPlan(plan) {
			planner.EnrichHelmPlan(ctx, &plan)
		} else if !executor.IsArgoWorkflowPlan(plan) {
			planner.EnrichDiffs(ctx, client, &plan)
		}
	}

	doc := output.FromPlan(cfg.Prompt, cfg.Context, plan, risk, false)
	if !jsonMode {
		ui.PrintPlan(out, plan, risk)
	}

	applied := false
	defer func() {
		_ = history.Append(history.FromPlan(cfg.Prompt, cfg.Context, plan, risk, applied))
		_ = history.Truncate()
		doc.Applied = applied
		if deps.OnResult != nil {
			deps.OnResult(doc)
		}
		if jsonMode {
			_ = output.Encode(out, doc)
		}
	}()

	// Read-only paths run immediately (no --approve).
	if isReadOnly(plan) {
		switch plan.Intent.Kind {
		case intent.KindDashboard:
			querier := deps.Grafana
			if querier == nil {
				settings := tools.LoadSettings(config.File{Tools: cfg.Tools})
				grafanaClient, err := tools.NewGrafanaClient(settings)
				if err != nil {
					return err
				}
				querier = grafanaClient
			}
			uid, _ := plan.Intent.DashboardUID()
			result, err := toolgrafana.ShowDashboard(ctx, querier, toolgrafana.ShowRequest{
				UID:   uid,
				Query: plan.Intent.Target.Name,
				Limit: 20,
			})
			if err != nil {
				return fmt.Errorf("show dashboard: %w", err)
			}
			doc = doc.WithDashboardResult(result)
			if !jsonMode {
				ui.PrintDashboardResult(out, result)
			}
			applied = true
			return nil
		case intent.KindTrace:
			querier := deps.OTel
			if querier == nil {
				settings := tools.LoadSettings(config.File{Tools: cfg.Tools})
				traceClient, err := tools.NewOTelClient(settings)
				if err != nil {
					return err
				}
				querier = traceClient
			}
			window := time.Hour
			if raw, ok := plan.Intent.Window(); ok {
				parsed, err := time.ParseDuration(raw)
				if err != nil {
					return fmt.Errorf("params.window: %w", err)
				}
				window = parsed
			}
			end := time.Now()
			operation, _ := plan.Intent.Operation()
			trace, err := toolotel.LatestTrace(ctx, querier, toolotel.SearchRequest{
				Service:   plan.Intent.Target.Name,
				Operation: operation,
				Start:     end.Add(-window),
				End:       end,
				Limit:     20,
			})
			if err != nil {
				return fmt.Errorf("trace walk: %w", err)
			}
			report := toolotel.AnalyzeTrace(trace)
			doc = doc.WithTraceResult(report)
			if !jsonMode {
				ui.PrintTrace(out, report)
			}
			applied = true
			return nil
		case intent.KindPerformance:
			querier := deps.Prometheus
			if querier == nil {
				settings := tools.LoadSettings(config.File{Tools: cfg.Tools})
				promClient, err := tools.NewPrometheusClient(settings)
				if err != nil {
					return err
				}
				querier = promClient
			}
			window := 15 * time.Minute
			if raw, ok := plan.Intent.Window(); ok {
				parsed, err := time.ParseDuration(raw)
				if err != nil {
					return fmt.Errorf("params.window: %w", err)
				}
				window = parsed
			}
			report, err := toolprometheus.ExplainPerformance(ctx, querier, toolprometheus.PerformanceRequest{
				Workload:  plan.Intent.Target.Name,
				Namespace: plan.Intent.Target.Namespace,
				Window:    window,
			})
			if err != nil {
				return fmt.Errorf("performance explain: %w", err)
			}
			doc = doc.WithPerformanceResult(report)
			if !jsonMode {
				ui.PrintPerformanceReport(out, report)
			}
			applied = true
			return nil
		case intent.KindOptimize:
			window := time.Hour
			if raw, ok := plan.Intent.Window(); ok {
				parsed, err := time.ParseDuration(raw)
				if err != nil {
					return fmt.Errorf("params.window: %w", err)
				}
				window = parsed
			}
			report := optimize.BuildScaffold(optimize.Request{
				Namespace: plan.Intent.Target.Namespace,
				Window:    window,
			})
			inv, err := optimize.CollectInventory(ctx, client, optimize.Request{
				Namespace: plan.Intent.Target.Namespace,
				Window:    window,
			})
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("optimize inventory: %w", err))
			}
			optimize.ApplyInventory(&report, inv)
			querier := deps.Prometheus
			if querier == nil {
				settings := tools.LoadSettings(config.File{Tools: cfg.Tools})
				if promClient, err := tools.NewPrometheusClient(settings); err == nil {
					querier = promClient
				}
			}
			idle := optimize.DetectIdle(ctx, querier, report.Workloads, window)
			optimize.ApplyIdle(&report, idle)
			rs := optimize.SuggestRightsizing(ctx, querier, report.Workloads, window)
			optimize.ApplyRightsizing(&report, rs)
			hpa := optimize.CollectHPAHints(ctx, client, querier, report.Workloads, plan.Intent.Target.Namespace)
			optimize.ApplyHPA(&report, hpa)
			doc = doc.WithOptimizeResult(report)
			if !jsonMode {
				ui.PrintOptimizeReport(out, report)
			}
			suggestions, err := suggest.FromOptimize(ctx, client, report)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("optimize suggest: %w", err))
			}
			if !jsonMode {
				ui.PrintSuggestions(out, suggestions)
			}
			actionable := suggest.ActionablePlans(suggestions)
			if len(actionable) == 0 {
				applied = true
				return nil
			}
			fix := *actionable[0].Plan
			fixRisk := safety.EvaluatePlan(fix)
			if fixRisk.Denied {
				if !jsonMode {
					ui.PrintDenied(out, fixRisk.Message)
				}
				applied = true
				return nil
			}
			if jsonMode {
				// Read-only optimize remains the JSON result; mutations need a separate approved prompt.
				applied = true
				return nil
			}
			fmt.Fprintln(out, "Optional fix (requires approval; optimize --approve does not auto-apply):")
			planner.EnrichDiffs(ctx, client, &fix)
			ui.PrintPlan(out, fix, fixRisk)
			// Never treat the parent optimize --approve flag as consent to mutate.
			approved, err := resolveApproval(false, out, deps)
			if err != nil {
				return err
			}
			if !approved {
				applied = true
				return nil
			}
			runner := &executor.Runner{Client: client}
			if err := runner.Apply(ctx, fix); err != nil {
				return cluster.Friendlier(fmt.Errorf("apply optimize suggestion: %w", err))
			}
			ui.PrintApplied(out, fix)
			applied = true
			return nil
		case intent.KindExplain:
			req, err := explainFromPlan(plan)
			if err != nil {
				return err
			}
			rep, err := (&cluster.Explainer{Client: client}).Explain(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("explain: %w", err))
			}
			doc = doc.WithExplainResult(rep)
			if jsonMode {
				applied = true
				return nil
			}
			ui.PrintExplain(out, rep)

			suggestions, err := suggest.FromExplain(ctx, client, rep)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("suggest: %w", err))
			}
			ui.PrintSuggestions(out, suggestions)

			actionable := suggest.ActionablePlans(suggestions)
			if len(actionable) == 0 {
				applied = true
				return nil
			}
			patch := *actionable[0].Plan
			patchRisk := safety.EvaluatePlan(patch)
			if patchRisk.Denied {
				ui.PrintDenied(out, patchRisk.Message)
				applied = true
				return nil
			}
			fmt.Fprintln(out, "Suggested fix (requires approval):")
			ui.PrintPlan(out, patch, patchRisk)
			approved, err := resolveApproval(cfg.Approve, out, deps)
			if err != nil {
				return err
			}
			if !approved {
				applied = true
				return nil
			}
			runner := &executor.Runner{Client: client}
			if err := runner.Apply(ctx, patch); err != nil {
				return cluster.Friendlier(fmt.Errorf("apply suggested patch: %w", err))
			}
			ui.PrintApplied(out, patch)
			applied = true
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
			doc = doc.WithLogsResult(res)
			if !jsonMode {
				ui.PrintLogs(out, res)
			}
			applied = true
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
			doc = doc.WithDescribeResult(rep)
			if !jsonMode {
				ui.PrintDescribe(out, rep)
			}
			applied = true
			return nil
		case intent.KindGet:
			// Named Workflow get keeps specialized Argo status; list/other kinds use dynamic/typed Reader.
			if isWorkflowGetPlan(plan) && strings.TrimSpace(plan.Actions[0].Object.Name) != "" {
				if err := tools.RequireArgoWorkflows(ctx, cfg.Context, nil); err != nil {
					return err
				}
				cfgREST, err := restConfigForArgo(cfg.Context, restCfg)
				if err != nil {
					return err
				}
				st, err := workflowStatusFromPlan(ctx, cfgREST, plan)
				if err != nil {
					return cluster.Friendlier(fmt.Errorf("workflow status: %w", err))
				}
				doc = doc.WithWorkflowResult(st)
				if !jsonMode {
					ui.PrintWorkflowStatus(out, st)
				}
				applied = true
				return nil
			}
			q, err := queryFromPlan(plan)
			if err != nil {
				return err
			}
			q, err = enrichQueryWithDiscovery(ctx, deps.Resolver, restCfg, q)
			if err != nil {
				return cluster.Friendlier(err)
			}
			dyn := deps.Dynamic
			if dyn == nil && restCfg != nil {
				dyn, err = cluster.DynamicForConfig(restCfg)
				if err != nil {
					return cluster.Friendlier(fmt.Errorf("dynamic client: %w", err))
				}
			}
			res, err := (&cluster.Reader{Client: client, Dynamic: dyn}).List(ctx, q)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("query: %w", err))
			}
			doc = doc.WithQueryResult(res)
			if !jsonMode {
				ui.PrintQueryResult(out, res)
			}
			applied = true
			return nil
		}
	}

	approved, err := resolveApproval(cfg.Approve, human, deps)
	if err != nil {
		return err
	}
	if !approved {
		return nil
	}

	runner := &executor.Runner{Client: client}
	if executor.IsArgoWorkflowPlan(plan) {
		cfgREST, err := restConfigForArgo(cfg.Context, restCfg)
		if err != nil {
			return err
		}
		st, err := executor.ApplyArgo(ctx, cfgREST, plan)
		if err != nil {
			return cluster.Friendlier(fmt.Errorf("apply: %w", err))
		}
		doc = doc.WithWorkflowResult(st)
		if !jsonMode {
			ui.PrintWorkflowApplied(human, plan, st)
		}
		applied = true
		if cfg.Wait {
			timeout := cfg.Timeout
			if timeout <= 0 {
				timeout = argo.DefaultWaitTimeout
			}
			for _, t := range executor.WorkflowTargets(plan) {
				st, err = argo.Wait(ctx, cfgREST, t.Namespace, t.Name, timeout, human)
				if err != nil {
					return cluster.Friendlier(err)
				}
				doc = doc.WithWorkflowResult(st)
			}
		}
		return nil
	}
	if executor.IsHelmPlan(plan) {
		if err := executor.ApplyHelm(ctx, plan); err != nil {
			return cluster.Friendlier(fmt.Errorf("apply: %w", err))
		}
	} else if err := runner.Apply(ctx, plan); err != nil {
		return cluster.Friendlier(fmt.Errorf("apply: %w", err))
	}
	if !jsonMode {
		ui.PrintApplied(out, plan)
	}
	applied = true

	if cfg.Wait {
		targets := deploymentWaitTargets(plan)
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = cluster.DefaultWaitTimeout
		}
		waiter := &cluster.Waiter{Client: client, Out: human}
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
	case intent.KindGet, intent.KindExplain, intent.KindLogs, intent.KindDescribe, intent.KindPerformance, intent.KindTrace, intent.KindDashboard, intent.KindOptimize:
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
	rawKind := a.Object.Kind
	if rawKind == "" {
		rawKind = plan.Intent.Target.Kind
	}
	ref, err := cluster.ParseResourceRef(firstNonEmpty(rawKind, "Pod"))
	if err != nil {
		return cluster.Query{}, err
	}
	if g, ok := plan.Intent.StringParam("group"); ok && ref.Group == "" {
		ref.Group = g
	}
	if r, ok := plan.Intent.StringParam("resource"); ok {
		// Prefer planner-normalized qualified resource when present.
		if parsed, perr := cluster.ParseResourceRef(r); perr == nil && parsed.Resource != "" {
			ref = parsed
		}
	}
	req := cluster.ReadRequest{
		Resource:  ref,
		Namespace: a.Object.Namespace,
		Name:      a.Object.Name,
	}
	if sel, ok := plan.Intent.LabelSelector(); ok {
		req.LabelSelector = sel
	}
	if limit, ok := plan.Intent.Limit(); ok {
		req.Limit = limit
	}
	if timeout, ok := plan.Intent.Timeout(); ok {
		req.Timeout = timeout
	}
	req, err = cluster.NormalizeReadRequest(req)
	if err != nil {
		return cluster.Query{}, err
	}
	q := cluster.QueryFromReadRequest(req)
	if mem, ok := plan.Intent.MinMemory(); ok {
		qty, err := resource.ParseQuantity(mem)
		if err != nil {
			return cluster.Query{}, fmt.Errorf("params.minMemory: %w", err)
		}
		q.MinMemory = qty
	}
	return q, nil
}

// enrichQueryWithDiscovery resolves kind/plural/shortName against cluster discovery when available.
// When neither an injected Resolver nor rest.Config is present (unit tests), returns q unchanged.
func enrichQueryWithDiscovery(ctx context.Context, resolver *cluster.Resolver, restCfg *rest.Config, q cluster.Query) (cluster.Query, error) {
	if resolver == nil && restCfg != nil {
		var err error
		resolver, err = cluster.NewResolverForConfig(restCfg)
		if err != nil {
			return cluster.Query{}, fmt.Errorf("discovery: %w", err)
		}
	}
	if resolver == nil {
		return q, nil
	}
	query := q.Resource
	if q.Group != "" && q.Resource != "" {
		query = q.Resource + "." + q.Group
	}
	if query == "" {
		query = q.Kind
	}
	ref, err := resolver.Resolve(ctx, query)
	if err != nil {
		return cluster.Query{}, err
	}
	req := cluster.ReadRequest{
		Resource:      ref,
		Namespace:     q.Namespace,
		Name:          q.Name,
		LabelSelector: q.LabelSelector,
		Limit:         q.Limit,
		Continue:      q.Continue,
		Timeout:       q.Timeout,
	}
	req, err = cluster.NormalizeReadRequest(req)
	if err != nil {
		return cluster.Query{}, err
	}
	out := cluster.QueryFromReadRequest(req)
	out.MinMemory = q.MinMemory
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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

func restConfigForArgo(kubeContext string, cached *rest.Config) (*rest.Config, error) {
	if cached != nil {
		return cached, nil
	}
	clients, err := cluster.Connect(kubeContext)
	if err != nil {
		return nil, err
	}
	return clients.Config, nil
}

func isWorkflowGetPlan(plan planner.ExecutionPlan) bool {
	if plan.Intent.Kind != intent.KindGet || len(plan.Actions) == 0 {
		return false
	}
	return plan.Actions[0].Object.Kind == "Workflow"
}

func workflowStatusFromPlan(ctx context.Context, cfg *rest.Config, plan planner.ExecutionPlan) (argo.WorkflowStatus, error) {
	if len(plan.Actions) == 0 {
		return argo.WorkflowStatus{}, fmt.Errorf("workflow get plan has no actions")
	}
	a := plan.Actions[0]
	if a.Object.Name == "" {
		return argo.WorkflowStatus{}, fmt.Errorf("workflow get requires a named target")
	}
	return argo.GetStatus(ctx, cfg, a.Object.Namespace, a.Object.Name)
}
