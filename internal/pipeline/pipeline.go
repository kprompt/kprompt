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

	"github.com/kprompt/kprompt/internal/architecture"
	"github.com/kprompt/kprompt/internal/audit"
	"github.com/kprompt/kprompt/internal/cleanup"
	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/drift"
	"github.com/kprompt/kprompt/internal/executor"
	"github.com/kprompt/kprompt/internal/gitopspr"
	"github.com/kprompt/kprompt/internal/graph"
	"github.com/kprompt/kprompt/internal/history"
	"github.com/kprompt/kprompt/internal/impact"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/investigate"
	"github.com/kprompt/kprompt/internal/learn"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/optimize"
	"github.com/kprompt/kprompt/internal/output"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/pretrust"
	"github.com/kprompt/kprompt/internal/recipe"
	"github.com/kprompt/kprompt/internal/remember"
	"github.com/kprompt/kprompt/internal/safety"
	"github.com/kprompt/kprompt/internal/score"
	"github.com/kprompt/kprompt/internal/search"
	"github.com/kprompt/kprompt/internal/suggest"
	"github.com/kprompt/kprompt/internal/team"
	"github.com/kprompt/kprompt/internal/timeline"
	"github.com/kprompt/kprompt/internal/tools"
	"github.com/kprompt/kprompt/internal/tools/argo"
	toolgitops "github.com/kprompt/kprompt/internal/tools/gitops"
	toolgrafana "github.com/kprompt/kprompt/internal/tools/grafana"
	toolistio "github.com/kprompt/kprompt/internal/tools/istio"
	toolotel "github.com/kprompt/kprompt/internal/tools/otel"
	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
	"github.com/kprompt/kprompt/internal/ui"
	"github.com/kprompt/kprompt/internal/verify"
	"github.com/kprompt/kprompt/internal/why"
)

// ConfirmFunc asks the user whether to apply a mutating plan.
type ConfirmFunc func(out io.Writer) (bool, error)

// Deps allows tests to inject LLM, Kubernetes clients, and approval behavior.
type Deps struct {
	Provider      llm.Provider
	Client        kubernetes.Interface
	Dynamic       dynamic.Interface // optional; built from rest config when unset (T-050)
	Resolver      *cluster.Resolver // optional discovery resolver (T-049); built from rest config when unset
	Prometheus    toolprometheus.Querier
	OTel          toolotel.Querier
	Grafana       toolgrafana.Querier
	Confirm       ConfirmFunc                                        // if set, used instead of TTY prompt
	IsTerminal    *bool                                              // override ui.StdinIsTerminal when non-nil
	OnResult      func(output.PlanResult)                            // optional per-plan completion observer
	SkipOrgPolicy bool                                               // tests: ignore Team org policy (Free CLI path)
	BuildPlan     func(intent.Intent) (planner.ExecutionPlan, error) // tests: override planner.Build
	// RouteSteps forces a multi-step route (recipe run / tests). When set, skips
	// SplitRoutePrompt / recipe.TryRoute matching on cfg.Prompt.
	RouteSteps []string
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
	if !deps.SkipOrgPolicy {
		team.ApplyOrgContextPolicy(&cfg)
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
		team.PushAuditBestEffort(ctx, team.AuditEventInput{
			Prompt:         cfg.Prompt,
			PlanSummary:    "prompt denied by local safety",
			Risk:           string(safety.RiskDenied),
			Decision:       "denied",
			ClusterContext: cfg.Context,
			Namespace:      cfg.Namespace,
		})
		if deps.OnResult != nil {
			deps.OnResult(doc)
		}
		if jsonMode {
			return output.Encode(out, doc)
		}
		ui.PrintDenied(out, denied.Message)
		return nil
	}

	// Roast / vibe-check: local health roast — no LLM key required.
	if intent.LooksLikeRoastPrompt(cfg.Prompt) {
		return runRoastPrompt(ctx, cfg, out, deps)
	}
	if intent.LooksLikeSessionPrompt(cfg.Prompt) {
		return runSessionPrompt(ctx, cfg, out, deps)
	}
	if intent.LooksLikeRememberPrompt(cfg.Prompt) {
		return runRememberPrompt(ctx, cfg, out, deps)
	}
	if intent.LooksLikeWatchPrompt(cfg.Prompt) {
		return runWatchOncePrompt(ctx, cfg, out, deps)
	}

	provider := deps.Provider
	if provider == nil {
		var err error
		provider, err = llm.New(cfg.Provider, config.APIKeyFor(cfg.Provider), cfg.BaseURL, cfg.Model)
		if err != nil {
			return err
		}
	}

	routeSteps := append([]string(nil), deps.RouteSteps...)
	var matchedRecipe recipe.Recipe
	if len(routeSteps) == 0 {
		if steps, r, ok, err := recipe.TryRoute(cfg.Prompt, cfg.Namespace, ""); ok {
			if err != nil {
				return err
			}
			routeSteps = steps
			matchedRecipe = r
		} else {
			routeSteps = intent.SplitRoutePrompt(cfg.Prompt)
		}
	}
	if len(routeSteps) > intent.MaxRouteSteps {
		return fmt.Errorf(
			"route has %d steps; maximum is %d",
			len(routeSteps),
			intent.MaxRouteSteps,
		)
	}
	if len(routeSteps) > 1 {
		deps.Provider = provider
		if matchedRecipe.ID != "" && !jsonMode {
			ui.PrintRecipe(out, matchedRecipe)
		}
		return runRoute(ctx, cfg, out, deps, routeSteps)
	}

	in, err := intent.ExtractWith(ctx, provider, cfg.Prompt, intent.ExtractOptions{
		ProfileHint: learnProfileHint(cfg.Context),
	})
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
	in = intent.ApplyRoastScope(in, cfg.Prompt, intent.ScopePrefs{
		DefaultNamespace: cfg.Namespace,
		ForceNamespace:   cfg.NamespaceFromCLI,
	})
	in = intent.ApplyGraphScope(in, cfg.Prompt, intent.ScopePrefs{
		DefaultNamespace: cfg.Namespace,
		ForceNamespace:   cfg.NamespaceFromCLI,
	})
	in = intent.ApplyAuditScope(in, cfg.Prompt, intent.ScopePrefs{
		DefaultNamespace: cfg.Namespace,
		ForceNamespace:   cfg.NamespaceFromCLI,
	})
	in = intent.ApplyCleanupScope(in, cfg.Prompt, intent.ScopePrefs{
		DefaultNamespace: cfg.Namespace,
		ForceNamespace:   cfg.NamespaceFromCLI,
	})
	in = intent.ApplySearchScope(in, cfg.Prompt, intent.ScopePrefs{
		DefaultNamespace: cfg.Namespace,
		ForceNamespace:   cfg.NamespaceFromCLI,
	})
	in = intent.ApplyScoreScope(in, cfg.Prompt, intent.ScopePrefs{
		DefaultNamespace: cfg.Namespace,
		ForceNamespace:   cfg.NamespaceFromCLI,
	})
	in = intent.ApplyArchitectureScope(in, cfg.Prompt, intent.ScopePrefs{
		DefaultNamespace: cfg.Namespace,
		ForceNamespace:   cfg.NamespaceFromCLI,
	})
	in = intent.ApplyDriftScope(in, cfg.Prompt, intent.ScopePrefs{
		DefaultNamespace: cfg.Namespace,
		ForceNamespace:   cfg.NamespaceFromCLI,
	})
	cfg.Namespace = in.Target.Namespace
	if in.Context != "" {
		cfg.Context = in.Context
	}
	resolveCfgContext(&cfg)

	if intent.LooksLikeWorkflowPrompt(cfg.Prompt) || in.Kind == intent.KindWorkflow {
		if err := tools.RequireArgoWorkflows(ctx, cfg.Context, nil); err != nil {
			return err
		}
	}
	if intent.LooksLikeTektonPrompt(cfg.Prompt) || in.Kind == intent.KindTekton {
		if err := tools.RequireTekton(ctx, cfg.Context, nil); err != nil {
			return err
		}
	}
	if intent.LooksLikeKEDAPrompt(cfg.Prompt) || in.Kind == intent.KindKEDA {
		if err := tools.RequireKeda(ctx, cfg.Context, nil); err != nil {
			return err
		}
	}
	if intent.LooksLikeIstioPrompt(cfg.Prompt) || in.Kind == intent.KindIstio {
		if err := tools.RequireIstio(ctx, cfg.Context, nil); err != nil {
			return err
		}
	}
	if intent.LooksLikeCrossplanePrompt(cfg.Prompt) || in.Kind == intent.KindCrossplane {
		if err := tools.RequireCrossplane(ctx, cfg.Context, nil); err != nil {
			return err
		}
	}
	if intent.LooksLikeGitOpsPrompt(cfg.Prompt) || in.Kind == intent.KindGitOps {
		if err := tools.RequireGitOps(ctx, cfg.Context, nil); err != nil {
			return err
		}
	}

	buildPlan := deps.BuildPlan
	if buildPlan == nil {
		buildPlan = planner.Build
	}
	plan, err := buildPlan(in)
	if err != nil {
		return err
	}

	risk := safety.EvaluatePlanWithOrg(plan, orgPolicy(deps), cfg.Context)
	if risk.Denied {
		doc := output.FromPlan(cfg.Prompt, cfg.Context, plan, risk, false)
		team.PushAuditBestEffort(ctx, auditFromPlan(cfg, plan, risk, "denied"))
		if deps.OnResult != nil {
			deps.OnResult(doc)
		}
		if jsonMode {
			return output.Encode(out, doc)
		}
		ui.PrintDenied(out, risk.Message)
		return nil
	}

	if contexts := fanOutContexts(cfg); len(contexts) > 0 {
		return runMultiContextFanOut(ctx, cfg, out, deps, provider, plan, risk, contexts)
	}

	if plan.RequiresApproval {
		if err := enforceAliasMatch(cfg); err != nil {
			denied := safety.Result{
				Risk:    safety.RiskDenied,
				Denied:  true,
				Message: err.Error(),
			}
			doc := output.FromPlan(cfg.Prompt, cfg.Context, plan, denied, false)
			team.PushAuditBestEffort(ctx, auditFromPlan(cfg, plan, denied, "denied"))
			if deps.OnResult != nil {
				deps.OnResult(doc)
			}
			if jsonMode {
				return output.Encode(out, doc)
			}
			ui.PrintDenied(out, denied.Message)
			return nil
		}
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
		} else if !executor.IsArgoWorkflowPlan(plan) && !executor.IsTektonPlan(plan) && !executor.IsKEDAPlan(plan) && !executor.IsCrossplanePlan(plan) && !executor.IsGitOpsSyncPlan(plan) {
			planner.EnrichDiffs(ctx, client, &plan)
			planner.EnrichBlastRadius(ctx, client, &plan)
		}
	}

	doc := output.FromPlan(cfg.Prompt, cfg.Context, plan, risk, false)
	gitopsSettings := gitopspr.LoadSettings(config.File{GitOps: cfg.EffectiveGitOps()})
	if cfg.GitOpsPR {
		gitopsSettings.Mode = gitopspr.ModePR
	}
	if !jsonMode && !cfg.FanOutChild {
		if gitopsSettings.Enabled() && plan.RequiresApproval {
			ui.PrintGitOpsPRTarget(out, gitopspr.Target{
				Repo:       gitopsSettings.Repo,
				Path:       gitopsSettings.Path,
				BaseBranch: gitopsSettings.BaseBranch,
			})
		}
		ui.PrintPlan(out, plan, risk)
	}

	applied := false
	decision := "planned"
	var verifyRep *verify.Report
	defer func() {
		entry := history.FromPlan(cfg.Prompt, cfg.Context, plan, risk, applied)
		if verifyRep != nil {
			entry.VerifyStatus = verifyRep.Status
			entry.VerifyMessage = verifyRep.Message
		}
		_ = history.Append(entry)
		_ = history.Truncate()
		doc.Applied = applied
		if verifyRep != nil {
			doc = doc.WithVerify(*verifyRep)
		}
		if decision == "planned" && applied && plan.RequiresApproval {
			decision = "applied"
		}
		if verifyRep != nil && verifyRep.Status == verify.Failed {
			decision = "verify_failed"
		}
		audit := auditFromPlan(cfg, plan, risk, decision)
		if verifyRep != nil {
			audit.VerifyStatus = verifyRep.Status
			audit.VerifyMessage = verifyRep.Message
			if verifyRep.Status != "" && verifyRep.Status != verify.Skipped {
				audit.PlanSummary = fmt.Sprintf("%s [verify:%s]", audit.PlanSummary, verifyRep.Status)
			}
		}
		team.PushAuditBestEffort(ctx, audit)
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
			optimize.ApplyCostNotes(&report, window)
			hpa := optimize.CollectHPAHints(ctx, client, querier, report.Workloads, plan.Intent.Target.Namespace)
			optimize.ApplyHPA(&report, hpa)
			optimize.StampClusterContext(&report, cfg.Context)
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
			if len(actionable) == 0 || cfg.FanOutChild {
				applied = true
				return nil
			}
			fix := *actionable[0].Plan
			fixRisk := safety.EvaluatePlanWithOrg(fix, orgPolicy(deps), cfg.Context)
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
			planner.EnrichBlastRadius(ctx, client, &fix)
			ui.PrintPlan(out, fix, fixRisk)
			// Never treat the parent optimize --approve flag as consent to mutate.
			approved, err := resolveApproval(false, out, deps, fixRisk)
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
			rep := verify.Plan(ctx, client, fix)
			ui.PrintVerify(out, rep)
			if rep.Status == verify.Failed {
				return fmt.Errorf("verify failed: %s", rep.Message)
			}
			applied = true
			return nil
		case intent.KindRoast:
			report, err := buildRoastReport(ctx, client, plan)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("roast: %w", err))
			}
			doc = doc.WithRoastResult(report)
			if !jsonMode {
				ui.PrintRoast(out, report)
			}
			applied = true
			return nil
		case intent.KindGraph:
			includeNP := true
			if v, ok := plan.Intent.Params["includeNetworkPolicy"]; ok {
				switch t := v.(type) {
				case bool:
					includeNP = t
				case string:
					includeNP = strings.EqualFold(t, "true") || t == "1"
				}
			}
			report, err := graph.Build(ctx, client, graph.Request{
				Namespace:            plan.Intent.Target.Namespace,
				IncludeNetworkPolicy: includeNP,
				IncludeIngress:       true,
				IncludePVC:           true,
				IncludeVolumeRefs:    true,
			})
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("service graph: %w", err))
			}
			querier := deps.OTel
			if querier == nil {
				settings := tools.LoadSettings(config.File{Tools: cfg.Tools})
				if otelClient, err := tools.NewOTelClient(settings); err == nil {
					querier = otelClient
				}
			}
			window := time.Hour
			if raw, ok := plan.Intent.Window(); ok {
				if parsed, err := time.ParseDuration(raw); err == nil {
					window = parsed
				}
			}
			graph.EnrichFromOTel(ctx, querier, &report, window)
			doc = doc.WithGraphResult(report)
			if !jsonMode {
				ui.PrintGraphReport(out, report)
			}
			applied = true
			return nil
		case intent.KindIstio:
			cfgREST, err := restConfigForArgo(cfg.Context, restCfg)
			if err != nil {
				return err
			}
			traffic, err := toolistio.SummarizeTraffic(ctx, cfgREST, toolistio.TrafficRequest{
				Namespace: plan.Intent.Target.Namespace,
				Name:      plan.Intent.Target.Name,
			})
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("istio traffic: %w", err))
			}
			doc = doc.WithIstioTrafficResult(traffic)
			if !jsonMode {
				ui.PrintIstioTrafficReport(out, traffic)
			}
			applied = true
			return nil
		case intent.KindGitOps:
			cfgREST, err := restConfigForArgo(cfg.Context, restCfg)
			if err != nil {
				return err
			}
			engine, _ := plan.Intent.StringParam("engine")
			status, err := toolgitops.SummarizeStatus(ctx, cfgREST, toolgitops.StatusRequest{
				Namespace: plan.Intent.Target.Namespace,
				Name:      plan.Intent.Target.Name,
				Engine:    engine,
			})
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("gitops status: %w", err))
			}
			doc = doc.WithGitOpsStatusResult(status)
			if !jsonMode {
				ui.PrintGitOpsStatusReport(out, status)
			}
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

			suggestions, err := suggest.FromExplain(ctx, client, rep, cfg.Prompt)
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
			patchRisk := safety.EvaluatePlanWithOrg(patch, orgPolicy(deps), cfg.Context)
			if patchRisk.Denied {
				ui.PrintDenied(out, patchRisk.Message)
				applied = true
				return nil
			}
			fmt.Fprintln(out, "Suggested fix (requires approval):")
			ui.PrintPlan(out, patch, patchRisk)
			approved, err := resolveApproval(cfg.Approve, out, deps, patchRisk)
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
		case intent.KindInvestigate:
			req, err := investigateFromPlan(plan, cfg.Prompt)
			if err != nil {
				return err
			}
			invDoc, rep, err := (&investigate.Investigator{Client: client}).Run(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("investigate: %w", err))
			}
			doc = doc.WithInvestigationResult(invDoc)
			if jsonMode {
				applied = true
				return nil
			}
			ui.PrintInvestigation(out, invDoc, rep)

			suggestions, err := suggest.FromExplain(ctx, client, rep, cfg.Prompt)
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
			prePlan := pretrust.SuggestedPlan(ctx, client, invDoc, patch)
			if prePlan.Denied {
				ui.PrintDenied(out, prePlan.DenyMessage)
				applied = true
				return nil
			}
			if !prePlan.OK {
				fmt.Fprintln(out, "Suggested fix withheld — Investigation failed independent verify (pretrust).")
				for _, n := range prePlan.Notes {
					fmt.Fprintln(out, " ", n)
				}
				applied = true
				return nil
			}
			patchRisk := safety.EvaluatePlanWithOrg(patch, orgPolicy(deps), cfg.Context)
			if patchRisk.Denied {
				ui.PrintDenied(out, patchRisk.Message)
				applied = true
				return nil
			}
			fmt.Fprintln(out, "Suggested fix (requires approval):")
			ui.PrintPlan(out, patch, patchRisk)
			approved, err := resolveApproval(cfg.Approve, out, deps, patchRisk)
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
			vrep := verify.Plan(ctx, client, patch)
			ui.PrintVerify(out, vrep)
			if vrep.Status == verify.Failed {
				return fmt.Errorf("verify failed: %s", vrep.Message)
			}
			applied = true
			return nil
		case intent.KindWhy:
			req, err := whyFromPlan(plan, cfg.Prompt)
			if err != nil {
				return err
			}
			invDoc, err := (&why.Analyzer{Client: client}).Run(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("why: %w", err))
			}
			doc = doc.WithInvestigationResult(invDoc)
			if jsonMode {
				applied = true
				return nil
			}
			ui.PrintInvestigation(out, invDoc, cluster.ExplainReport{})

			suggestions, err := suggest.FromInvestigation(ctx, client, invDoc, cfg.Prompt)
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
			prePlan := pretrust.SuggestedPlan(ctx, client, invDoc, patch)
			if prePlan.Denied {
				ui.PrintDenied(out, prePlan.DenyMessage)
				applied = true
				return nil
			}
			if !prePlan.OK {
				fmt.Fprintln(out, "Suggested fix withheld — Investigation failed independent verify (pretrust).")
				for _, n := range prePlan.Notes {
					fmt.Fprintln(out, " ", n)
				}
				applied = true
				return nil
			}
			patchRisk := safety.EvaluatePlanWithOrg(patch, orgPolicy(deps), cfg.Context)
			if patchRisk.Denied {
				ui.PrintDenied(out, patchRisk.Message)
				applied = true
				return nil
			}
			fmt.Fprintln(out, "Suggested fix (requires approval):")
			ui.PrintPlan(out, patch, patchRisk)
			approved, err := resolveApproval(cfg.Approve, out, deps, patchRisk)
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
			vrep := verify.Plan(ctx, client, patch)
			ui.PrintVerify(out, vrep)
			if vrep.Status == verify.Failed {
				return fmt.Errorf("verify failed: %s", vrep.Message)
			}
			applied = true
			return nil
		case intent.KindTimeline:
			req, err := timelineFromPlan(plan, cfg.Prompt)
			if err != nil {
				return err
			}
			invDoc, err := (&timeline.Builder{Client: client}).Run(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("timeline: %w", err))
			}
			doc = doc.WithInvestigationResult(invDoc)
			if jsonMode {
				applied = true
				return nil
			}
			ui.PrintInvestigation(out, invDoc, cluster.ExplainReport{})
			applied = true
			return nil
		case intent.KindImpact:
			req, err := impactFromPlan(plan, cfg.Prompt)
			if err != nil {
				return err
			}
			invDoc, err := (&impact.Analyzer{Client: client}).Run(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("impact: %w", err))
			}
			doc = doc.WithInvestigationResult(invDoc)
			if !jsonMode {
				ui.PrintInvestigation(out, invDoc, cluster.ExplainReport{})
			}
			applied = true
			return nil
		case intent.KindAudit:
			req, err := hygieneAuditFromPlan(plan, cfg.Prompt)
			if err != nil {
				return err
			}
			invDoc, err := (&audit.Analyzer{Client: client}).Run(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("audit: %w", err))
			}
			doc = doc.WithInvestigationResult(invDoc)
			if jsonMode {
				applied = true
				return nil
			}
			ui.PrintInvestigation(out, invDoc, cluster.ExplainReport{})

			suggestions, err := suggest.FromAudit(ctx, client, invDoc)
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
			patchRisk := safety.EvaluatePlanWithOrg(patch, orgPolicy(deps), cfg.Context)
			if patchRisk.Denied {
				ui.PrintDenied(out, patchRisk.Message)
				applied = true
				return nil
			}
			fmt.Fprintln(out, "Suggested hardening (requires approval):")
			ui.PrintPlan(out, patch, patchRisk)
			approved, err := resolveApproval(cfg.Approve, out, deps, patchRisk)
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
		case intent.KindLearn:
			prof, err := learn.Run(ctx, learn.Options{
				Context: cfg.Context,
				File:    config.File{Context: cfg.Context, Tools: cfg.Tools},
			})
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("learn: %w", err))
			}
			doc = doc.WithLearnResult(prof)
			if !jsonMode {
				ui.PrintLearnProfile(out, prof)
			}
			applied = true
			return nil
		case intent.KindDrift:
			cfgREST, err := restConfigForArgo(cfg.Context, restCfg)
			if err != nil {
				return err
			}
			req, err := driftFromPlan(plan, cfg.Prompt)
			if err != nil {
				return err
			}
			invDoc, err := (&drift.Analyzer{Config: cfgREST}).Run(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("drift: %w", err))
			}
			doc = doc.WithInvestigationResult(invDoc)
			if jsonMode {
				applied = true
				return nil
			}
			ui.PrintInvestigation(out, invDoc, cluster.ExplainReport{})

			suggestions, err := suggest.FromDrift(invDoc)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("suggest: %w", err))
			}
			ui.PrintSuggestions(out, suggestions)

			actionable := suggest.ActionablePlans(suggestions)
			if len(actionable) == 0 {
				applied = true
				return nil
			}
			for _, sug := range actionable {
				patch := *sug.Plan
				patchRisk := safety.EvaluatePlanWithOrg(patch, orgPolicy(deps), cfg.Context)
				if patchRisk.Denied {
					ui.PrintDenied(out, patchRisk.Message)
					continue
				}
				fmt.Fprintln(out, "Suggested GitOps sync (requires approval):")
				ui.PrintPlan(out, patch, patchRisk)
				approved, err := resolveApproval(cfg.Approve, out, deps, patchRisk)
				if err != nil {
					return err
				}
				if !approved {
					continue
				}
				if !executor.IsGitOpsSyncPlan(patch) {
					return fmt.Errorf("drift suggest: expected gitops sync plan")
				}
				st, err := executor.ApplyGitOpsSync(ctx, cfgREST, patch)
				if err != nil {
					return cluster.Friendlier(fmt.Errorf("apply suggested sync: %w", err))
				}
				ui.PrintGitOpsSyncApplied(out, patch, st)
			}
			applied = true
			return nil
		case intent.KindCleanup:
			req, err := cleanupFromPlan(plan, cfg.Prompt)
			if err != nil {
				return err
			}
			invDoc, err := (&cleanup.Analyzer{Client: client}).Run(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("cleanup: %w", err))
			}
			doc = doc.WithInvestigationResult(invDoc)
			if jsonMode {
				applied = true
				return nil
			}
			ui.PrintInvestigation(out, invDoc, cluster.ExplainReport{})

			suggestions, err := suggest.FromCleanup(ctx, invDoc, cfg.Prompt)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("suggest: %w", err))
			}
			ui.PrintSuggestions(out, suggestions)

			actionable := suggest.ActionablePlans(suggestions)
			if len(actionable) == 0 {
				applied = true
				return nil
			}
			runner := &executor.Runner{Client: client}
			for _, sug := range actionable {
				patch := *sug.Plan
				patchRisk := safety.EvaluatePlanWithOrg(patch, orgPolicy(deps), cfg.Context)
				if patchRisk.Denied {
					ui.PrintDenied(out, patchRisk.Message)
					continue
				}
				fmt.Fprintln(out, "Suggested cleanup (requires approval):")
				ui.PrintPlan(out, patch, patchRisk)
				var approved bool
				if suggest.IsCleanupOrphanPlan(patch) {
					approved, err = resolveOrphanApproval(cfg.Approve, out, deps)
				} else {
					approved, err = resolveApproval(cfg.Approve, out, deps, patchRisk)
				}
				if err != nil {
					return err
				}
				if !approved {
					continue
				}
				if err := runner.Apply(ctx, patch); err != nil {
					return cluster.Friendlier(fmt.Errorf("apply suggested cleanup: %w", err))
				}
				ui.PrintApplied(out, patch)
			}
			applied = true
			return nil
		case intent.KindSearch:
			req, err := searchFromPlan(plan, cfg.Prompt)
			if err != nil {
				return err
			}
			rep, err := (&search.Analyzer{Client: client}).Run(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("search: %w", err))
			}
			doc = doc.WithSearchResult(rep)
			if !jsonMode {
				ui.PrintSearch(out, rep)
			}
			applied = true
			return nil
		case intent.KindScore:
			req, err := scoreFromPlan(plan, cfg.Prompt)
			if err != nil {
				return err
			}
			querier := deps.Prometheus
			if querier == nil {
				settings := tools.LoadSettings(config.File{Tools: cfg.Tools})
				if promClient, err := tools.NewPrometheusClient(settings); err == nil {
					querier = promClient
				}
			}
			rep, err := (&score.Analyzer{Client: client, Prometheus: querier}).Run(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("score: %w", err))
			}
			if cfg.Context != "" {
				rep.ClusterContext = cfg.Context
			}
			doc = doc.WithScoreResult(rep)
			if !jsonMode {
				ui.PrintScorecard(out, rep)
			}
			applied = true
			return nil
		case intent.KindArchitecture:
			req, err := architectureFromPlan(plan, cfg)
			if err != nil {
				return err
			}
			rep, err := (&architecture.Analyzer{Client: client}).Run(ctx, req)
			if err != nil {
				return cluster.Friendlier(fmt.Errorf("architecture: %w", err))
			}
			doc = doc.WithArchitectureResult(rep)
			if !jsonMode {
				ui.PrintArchitecture(out, rep)
			}
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

	approved, err := resolveApprovalMode(cfg.Approve, human, deps, gitopsSettings.Enabled(), risk)
	if err != nil {
		return err
	}
	if !approved {
		decision = "denied"
		return nil
	}
	decision = "approved"

	if gitopsSettings.Enabled() {
		if !plan.RequiresApproval {
			return fmt.Errorf("gitops PR mode only applies to mutating plans")
		}
		res, err := gitopspr.OpenFromPlan(ctx, plan, gitopspr.Options{
			Settings: gitopsSettings,
			Prompt:   cfg.Prompt,
		})
		if err != nil {
			return cluster.Friendlier(fmt.Errorf("gitops pr: %w", err))
		}
		doc = doc.WithGitOpsPRResult(res)
		if !jsonMode {
			ui.PrintGitOpsPROpened(human, res)
		}
		applied = true
		decision = "pr_opened"
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
	if executor.IsTektonPlan(plan) {
		cfgREST, err := restConfigForArgo(cfg.Context, restCfg)
		if err != nil {
			return err
		}
		st, err := executor.ApplyTekton(ctx, cfgREST, plan)
		if err != nil {
			return cluster.Friendlier(fmt.Errorf("apply: %w", err))
		}
		doc = doc.WithPipelineRunResult(st)
		if !jsonMode {
			ui.PrintPipelineRunApplied(human, plan, st)
		}
		applied = true
		return nil
	}
	if executor.IsKEDAPlan(plan) {
		cfgREST, err := restConfigForArgo(cfg.Context, restCfg)
		if err != nil {
			return err
		}
		st, err := executor.ApplyKEDA(ctx, cfgREST, plan)
		if err != nil {
			return cluster.Friendlier(fmt.Errorf("apply: %w", err))
		}
		doc = doc.WithScaledObjectResult(st)
		if !jsonMode {
			ui.PrintScaledObjectApplied(human, plan, st)
		}
		applied = true
		return nil
	}
	if executor.IsCrossplanePlan(plan) {
		cfgREST, err := restConfigForArgo(cfg.Context, restCfg)
		if err != nil {
			return err
		}
		st, err := executor.ApplyCrossplane(ctx, cfgREST, plan)
		if err != nil {
			return cluster.Friendlier(fmt.Errorf("apply: %w", err))
		}
		doc = doc.WithClaimResult(st)
		if !jsonMode {
			ui.PrintClaimApplied(human, plan, st)
		}
		applied = true
		return nil
	}
	if executor.IsGitOpsSyncPlan(plan) {
		cfgREST, err := restConfigForArgo(cfg.Context, restCfg)
		if err != nil {
			return err
		}
		st, err := executor.ApplyGitOpsSync(ctx, cfgREST, plan)
		if err != nil {
			return cluster.Friendlier(fmt.Errorf("apply: %w", err))
		}
		doc = doc.WithGitOpsSyncResult(st)
		if !jsonMode {
			ui.PrintGitOpsSyncApplied(human, plan, st)
		}
		applied = true
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
	decision = "applied"

	if cfg.Wait {
		targets := deploymentWaitTargets(plan)
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = cluster.DefaultWaitTimeout
		}
		waiter := &cluster.Waiter{Client: client, Out: human}
		for _, t := range targets {
			var err error
			switch t.Kind {
			case "Deployment":
				err = waiter.WaitDeployment(ctx, t.Namespace, t.Name, timeout)
			case "StatefulSet":
				err = waiter.WaitStatefulSet(ctx, t.Namespace, t.Name, timeout)
			default:
				fmt.Fprintf(out, "Skipping --wait for unsupported resource kind %s\n", t.Kind)
				continue
			}
			if err != nil {
				rep := verify.Report{
					Status:  verify.Failed,
					Message: err.Error(),
				}
				verifyRep = &rep
				if !jsonMode {
					ui.PrintVerify(human, rep)
				}
				return cluster.Friendlier(err)
			}
		}
	}

	if client != nil {
		rep := verify.Plan(ctx, client, plan)
		// After --wait, pending should not happen; treat as failure for clarity.
		if cfg.Wait && rep.Status == verify.Pending {
			rep.Status = verify.Failed
			rep.Message = "still pending after --wait: " + rep.Message
		}
		verifyRep = &rep
		if !jsonMode {
			ui.PrintVerify(human, rep)
		}
		if rep.Status == verify.Failed {
			return fmt.Errorf("verify failed: %s", rep.Message)
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
			if a.Object.Kind != "Deployment" && a.Object.Kind != "StatefulSet" && a.Object.Kind != "" {
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

func resolveApproval(flagApprove bool, out io.Writer, deps Deps, risk safety.Result) (bool, error) {
	return resolveApprovalMode(flagApprove, out, deps, false, risk)
}

func resolveApprovalMode(flagApprove bool, out io.Writer, deps Deps, prMode bool, risk safety.Result) (bool, error) {
	ok, err := resolveApprovalConsent(flagApprove, out, deps, prMode)
	if err != nil || !ok {
		return ok, err
	}
	if !deps.SkipOrgPolicy {
		if msg := orgRoleApproveDeny(risk); msg != "" {
			ui.PrintDenied(out, msg)
			return false, nil
		}
	}
	return true, nil
}

func resolveApprovalConsent(flagApprove bool, out io.Writer, deps Deps, prMode bool) (bool, error) {
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
		if prMode {
			ui.PrintNeedsApprovePR(out)
		} else {
			ui.PrintNeedsApprove(out)
		}
		return false, nil
	}
	var ok bool
	var err error
	if prMode {
		ok, err = ui.ConfirmOpenPR(os.Stdin, out)
	} else {
		ok, err = ui.ConfirmApply(os.Stdin, out)
	}
	if err != nil {
		return false, err
	}
	if !ok {
		ui.PrintAborted(out)
	}
	return ok, nil
}

func orgRoleApproveDeny(risk safety.Result) string {
	org := loadOrgPolicy()
	if org == nil || len(org.ApproveByRole) == 0 {
		return ""
	}
	role := loadMemberRole()
	return safety.RoleApproveDenyMessage(org.ApproveByRole, role, risk.Risk)
}

func loadMemberRole() string {
	creds, ok, err := team.LoadCredentials()
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(creds.MemberRole)
}

// resolveOrphanApproval gates ConfigMap/Secret orphan deletes. Interactive
// sessions must type DELETE-ORPHANS; --approve is enough when the original
// prompt already contained a confirm-orphans phrase (otherwise no orphan plan).
func resolveOrphanApproval(flagApprove bool, out io.Writer, deps Deps) (bool, error) {
	ok, err := resolveOrphanConsent(flagApprove, out, deps)
	if err != nil || !ok {
		return ok, err
	}
	if !deps.SkipOrgPolicy {
		high := safety.Result{Risk: safety.RiskHigh}
		if msg := orgRoleApproveDeny(high); msg != "" {
			ui.PrintDenied(out, msg)
			return false, nil
		}
	}
	return true, nil
}

func resolveOrphanConsent(flagApprove bool, out io.Writer, deps Deps) (bool, error) {
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
	ok, err := ui.ConfirmPhrase(os.Stdin, out, "DELETE-ORPHANS")
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
	case intent.KindGet, intent.KindExplain, intent.KindInvestigate, intent.KindWhy, intent.KindTimeline, intent.KindImpact, intent.KindAudit, intent.KindCleanup, intent.KindSearch, intent.KindScore, intent.KindArchitecture, intent.KindLearn, intent.KindDrift, intent.KindLogs, intent.KindDescribe, intent.KindPerformance, intent.KindTrace, intent.KindDashboard, intent.KindOptimize, intent.KindRoast, intent.KindGraph, intent.KindIstio, intent.KindGitOps:
		return true
	default:
		return false
	}
}

func learnProfileHint(kubeCtx string) string {
	var b strings.Builder
	if p, ok := learn.LoadBestEffort(kubeCtx); ok {
		b.WriteString(p.PromptBias())
	}
	if mem := remember.PromptBias(); mem != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(mem)
	}
	return b.String()
}

func resolveCfgContext(cfg *config.Resolved) {
	resolved, alias := config.ResolveContext(cfg.Context, cfg.Aliases)
	cfg.Context = resolved
	if alias != "" {
		cfg.ContextAlias = alias
	}
}

// enforceAliasMatch refuses mutate when require_alias_match is on and the
// active kubeconfig current-context differs from the resolved target context.
func enforceAliasMatch(cfg config.Resolved) error {
	if !cfg.RequireAliasMatch {
		return nil
	}
	if strings.TrimSpace(cfg.Context) == "" {
		return nil
	}
	active, err := cluster.CurrentContext()
	if err != nil {
		return err
	}
	if active == cfg.Context {
		return nil
	}
	target := cfg.Context
	if cfg.ContextAlias != "" {
		target = fmt.Sprintf("%s (%s)", cfg.ContextAlias, cfg.Context)
	}
	return fmt.Errorf(
		"require_alias_match: active kube context %q ≠ target %s — run: kubectl config use-context %s (or: kprompt config set require_alias_match false)",
		active,
		target,
		cfg.Context,
	)
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

func investigateFromPlan(plan planner.ExecutionPlan, prompt string) (investigate.Request, error) {
	if len(plan.Actions) == 0 {
		return investigate.Request{}, fmt.Errorf("investigate plan has no actions")
	}
	a := plan.Actions[0]
	if a.Object.Name == "" {
		return investigate.Request{}, fmt.Errorf("investigate requires a named target")
	}
	return investigate.Request{
		Name:      a.Object.Name,
		Namespace: a.Object.Namespace,
		Kind:      a.Object.Kind,
		Prompt:    prompt,
	}, nil
}

func whyFromPlan(plan planner.ExecutionPlan, prompt string) (why.Request, error) {
	if len(plan.Actions) == 0 {
		return why.Request{}, fmt.Errorf("why plan has no actions")
	}
	a := plan.Actions[0]
	if a.Object.Name == "" {
		return why.Request{}, fmt.Errorf("why requires a named target")
	}
	return why.Request{
		Name:      a.Object.Name,
		Namespace: a.Object.Namespace,
		Kind:      a.Object.Kind,
		Prompt:    prompt,
	}, nil
}

func timelineFromPlan(plan planner.ExecutionPlan, prompt string) (timeline.Request, error) {
	if len(plan.Actions) == 0 {
		return timeline.Request{}, fmt.Errorf("timeline plan has no actions")
	}
	a := plan.Actions[0]
	if a.Object.Name == "" {
		return timeline.Request{}, fmt.Errorf("timeline requires a named target")
	}
	req := timeline.Request{
		Name:      a.Object.Name,
		Namespace: a.Object.Namespace,
		Kind:      a.Object.Kind,
		Prompt:    prompt,
	}
	if w, ok := plan.Intent.Window(); ok {
		if parsed, err := time.ParseDuration(w); err == nil {
			req.Window = parsed
		}
	}
	return req, nil
}

func impactFromPlan(plan planner.ExecutionPlan, prompt string) (impact.Request, error) {
	if len(plan.Actions) == 0 {
		return impact.Request{}, fmt.Errorf("impact plan has no actions")
	}
	a := plan.Actions[0]
	if a.Object.Name == "" {
		return impact.Request{}, fmt.Errorf("impact requires a named target")
	}
	return impact.Request{
		Name:      a.Object.Name,
		Namespace: a.Object.Namespace,
		Kind:      a.Object.Kind,
		Prompt:    prompt,
	}, nil
}

func hygieneAuditFromPlan(plan planner.ExecutionPlan, prompt string) (audit.Request, error) {
	if len(plan.Actions) == 0 {
		return audit.Request{}, fmt.Errorf("audit plan has no actions")
	}
	a := plan.Actions[0]
	return audit.Request{
		Namespace: a.Object.Namespace,
		Prompt:    prompt,
	}, nil
}

func cleanupFromPlan(plan planner.ExecutionPlan, prompt string) (cleanup.Request, error) {
	if len(plan.Actions) == 0 {
		return cleanup.Request{}, fmt.Errorf("cleanup plan has no actions")
	}
	a := plan.Actions[0]
	return cleanup.Request{
		Namespace: a.Object.Namespace,
		Prompt:    prompt,
	}, nil
}

func searchFromPlan(plan planner.ExecutionPlan, prompt string) (search.Request, error) {
	if len(plan.Actions) == 0 {
		return search.Request{}, fmt.Errorf("search plan has no actions")
	}
	a := plan.Actions[0]
	query, _ := plan.Intent.StringParam("query")
	if strings.TrimSpace(query) == "" {
		query = intent.InferSearchQuery(prompt)
	}
	match, _ := plan.Intent.StringParam("match")
	return search.Request{
		Namespace: a.Object.Namespace,
		Prompt:    prompt,
		Query:     query,
		Kind:      a.Object.Kind,
		Match:     match,
	}, nil
}

func scoreFromPlan(plan planner.ExecutionPlan, prompt string) (score.Request, error) {
	if len(plan.Actions) == 0 {
		return score.Request{}, fmt.Errorf("score plan has no actions")
	}
	a := plan.Actions[0]
	window := time.Hour
	if raw, ok := plan.Intent.Window(); ok {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return score.Request{}, fmt.Errorf("params.window: %w", err)
		}
		window = parsed
	}
	return score.Request{
		Namespace: a.Object.Namespace,
		Prompt:    prompt,
		Window:    window,
	}, nil
}

func architectureFromPlan(plan planner.ExecutionPlan, cfg config.Resolved) (architecture.Request, error) {
	if len(plan.Actions) == 0 {
		return architecture.Request{}, fmt.Errorf("architecture plan has no actions")
	}
	a := plan.Actions[0]
	return architecture.Request{
		Namespace: a.Object.Namespace,
		Prompt:    cfg.Prompt,
		Context:   cfg.Context,
		File:      config.File{Context: cfg.Context, Tools: cfg.Tools},
	}, nil
}

func driftFromPlan(plan planner.ExecutionPlan, prompt string) (drift.Request, error) {
	if len(plan.Actions) == 0 {
		return drift.Request{}, fmt.Errorf("drift plan has no actions")
	}
	a := plan.Actions[0]
	engine, _ := plan.Intent.StringParam("engine")
	return drift.Request{
		Namespace: a.Object.Namespace,
		Name:      a.Object.Name,
		Engine:    engine,
		Prompt:    prompt,
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

// loadOrgPolicy returns cached Team org policy when enrolled; nil otherwise (Free CLI path).
func loadOrgPolicy() *safety.OrgPolicy {
	pol, ok, err := team.LoadPolicy()
	if err != nil || !ok {
		return nil
	}
	return &safety.OrgPolicy{
		OrgID:           pol.OrgID,
		Version:         pol.Version,
		MaxRisk:         pol.MaxRisk,
		DenyIntents:     pol.DenyIntents,
		AllowNamespaces: pol.AllowNamespaces,
		DenyNamespaces:  pol.DenyNamespaces,
		RequireApprove:  pol.RequireApprove,
		ChangeWindows:   toSafetyWindows(pol.ChangeWindows),
		ApproveByRole:   pol.ApproveByRole,
	}
}

func toSafetyWindows(in []team.ChangeWindow) []safety.ChangeWindow {
	if len(in) == 0 {
		return nil
	}
	out := make([]safety.ChangeWindow, len(in))
	for i, w := range in {
		out[i] = safety.ChangeWindow{
			Contexts: w.Contexts,
			TZ:       w.TZ,
			Days:     w.Days,
			Start:    w.Start,
			End:      w.End,
		}
	}
	return out
}

func orgPolicy(deps Deps) *safety.OrgPolicy {
	if deps.SkipOrgPolicy {
		return nil
	}
	return loadOrgPolicy()
}

func auditFromPlan(cfg config.Resolved, plan planner.ExecutionPlan, risk safety.Result, decision string) team.AuditEventInput {
	ns := plan.Intent.Target.Namespace
	if ns == "" {
		for _, a := range plan.Actions {
			if a.Object.Namespace != "" {
				ns = a.Object.Namespace
				break
			}
		}
	}
	if ns == "" {
		ns = cfg.Namespace
	}
	return team.AuditEventInput{
		Prompt:         cfg.Prompt,
		PlanSummary:    plan.Summary,
		Risk:           string(risk.Risk),
		Decision:       decision,
		ClusterContext: cfg.Context,
		Namespace:      ns,
	}
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
