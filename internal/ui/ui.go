package ui

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/graph"
	"github.com/kprompt/kprompt/internal/optimize"
	"github.com/kprompt/kprompt/internal/output"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
	"github.com/kprompt/kprompt/internal/suggest"
	"github.com/kprompt/kprompt/internal/tools/argo"
	"github.com/kprompt/kprompt/internal/tools/crossplane"
	"github.com/kprompt/kprompt/internal/tools/gitops"
	toolgrafana "github.com/kprompt/kprompt/internal/tools/grafana"
	"github.com/kprompt/kprompt/internal/tools/istio"
	"github.com/kprompt/kprompt/internal/tools/keda"
	toolotel "github.com/kprompt/kprompt/internal/tools/otel"
	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
	"github.com/kprompt/kprompt/internal/tools/tekton"
)

// PrintDenied writes a hard-deny message.
func PrintDenied(w io.Writer, msg string) {
	t := themeFor(w)
	fmt.Fprintln(w, t.Danger(msg))
}

// PrintContextSection prints a fan-out header for one kube context.
func PrintContextSection(w io.Writer, contextName string, index, total int) {
	t := themeFor(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, t.Heading(fmt.Sprintf("=== context %d/%d: %s ===", index, total, contextName)))
}

// PrintFleetOptimizeSummary prints the merged multi-context optimize rollup.
func PrintFleetOptimizeSummary(w io.Writer, sum *output.FleetOptimizeSummary) {
	if sum == nil {
		return
	}
	t := themeFor(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, t.Heading("=== fleet optimize summary ==="))
	fmt.Fprintf(w, "ok: %d  failed: %d  findings: %d\n", len(sum.ContextsOK), len(sum.ContextsFailed), sum.FindingCount)
	if len(sum.ContextsOK) > 0 {
		fmt.Fprintf(w, "  contexts ok: %s\n", strings.Join(sum.ContextsOK, ", "))
	}
	if len(sum.ContextsFailed) > 0 {
		fmt.Fprintf(w, "  contexts failed: %s\n", t.Danger(strings.Join(sum.ContextsFailed, ", ")))
	}
	const maxFindings = 30
	for i, f := range sum.Findings {
		if i >= maxFindings {
			fmt.Fprintf(w, "  … %d more findings\n", len(sum.Findings)-maxFindings)
			break
		}
		ctx := f.ClusterContext
		if ctx == "" {
			ctx = "?"
		}
		fmt.Fprintf(w, "  - [%s] [%s] %s: %s\n", t.Severity(f.Severity), ctx, t.Accent(f.Title), f.Message)
	}
}

// PrintPlan prints a human-readable execution plan.
func PrintPlan(w io.Writer, plan planner.ExecutionPlan, risk safety.Result) {
	t := themeFor(w)
	fmt.Fprintf(w, "%s %s\n", t.Heading("Intent:"), string(plan.Intent.Kind))
	if plan.Summary != "" {
		fmt.Fprintf(w, "%s %s\n", t.Heading("Plan:  "), plan.Summary)
	}
	fmt.Fprintf(w, "%s %s\n", t.Heading("Risk:  "), t.Risk(risk.Risk))
	if len(plan.Actions) > 0 {
		fmt.Fprintln(w, t.Heading("Actions:"))
		for i, a := range plan.Actions {
			var line string
			if len(a.Command) > 0 {
				line = fmt.Sprintf("  %d. %s", i+1, t.Accent("$ "+strings.Join(a.Command, " ")))
			} else {
				line = fmt.Sprintf("  %d. %s %s", i+1, a.Op, t.Accent(a.Object.Kind+"/"+a.Object.Name))
				if a.Object.Namespace != "" {
					line += " -n " + a.Object.Namespace
				}
				if a.Backend != "" {
					line += " via " + t.Accent(a.Backend)
				}
				if a.Replicas != nil && a.Op == planner.OpScale {
					line += fmt.Sprintf(" → %d replicas", *a.Replicas)
				}
			}
			fmt.Fprintln(w, line)
			if a.Diff != "" && (len(a.Command) == 0 || a.Diff != strings.Join(a.Command, " ")) {
				fmt.Fprintln(w, "     "+t.Muted("Diff:"))
				for _, dl := range strings.Split(a.Diff, "\n") {
					if dl == "" {
						continue
					}
					fmt.Fprintf(w, "       %s\n", colorizeDiffLine(t, dl))
				}
			}
			if strings.TrimSpace(a.Manifest) != "" {
				fmt.Fprintln(w, "     "+t.Muted("Preview:"))
				for _, line := range strings.Split(strings.TrimRight(a.Manifest, "\n"), "\n") {
					fmt.Fprintf(w, "       %s\n", t.Muted(line))
				}
			}
		}
	}
	if plan.RequiresApproval {
		fmt.Fprintln(w, t.Muted("Next: confirm interactively on a TTY, or re-run with --approve."))
	}
}

// PrintRoute prints the ordered NL requests in a multi-tool chain.
func PrintRoute(w io.Writer, steps []string) {
	t := themeFor(w)
	fmt.Fprintf(w, "%s %d sequential steps\n", t.Heading("Route:"), len(steps))
	for index, step := range steps {
		fmt.Fprintf(w, "  %d. %s\n", index+1, step)
	}
}

// PrintRoutePlan prints the aggregate preflighted plans for single-approval review (T-058).
func PrintRoutePlan(w io.Writer, steps []string, plans []planner.ExecutionPlan, risks []safety.Result) {
	t := themeFor(w)
	fmt.Fprintln(w, t.Heading("Aggregate plan:"))
	for i := range plans {
		prompt := ""
		if i < len(steps) {
			prompt = steps[i]
		}
		risk := safety.Result{Risk: safety.RiskLow}
		if i < len(risks) {
			risk = risks[i]
		}
		approval := ""
		if plans[i].RequiresApproval {
			approval = " [needs approval]"
		}
		summary := plans[i].Summary
		if summary == "" {
			summary = string(plans[i].Intent.Kind)
		}
		fmt.Fprintf(
			w,
			"  %d. %s — %s · risk=%s%s\n",
			i+1,
			prompt,
			summary,
			t.Risk(risk.Risk),
			approval,
		)
	}
}

// PrintRouteStep separates one routed plan from the next.
func PrintRouteStep(w io.Writer, index, total int, prompt string) {
	t := themeFor(w)
	fmt.Fprintf(
		w,
		"\n%s %s\n",
		t.Heading(fmt.Sprintf("Step %d/%d:", index, total)),
		t.Accent(prompt),
	)
}

// PrintRouteStopped reports why remaining steps were not executed.
func PrintRouteStopped(w io.Writer, index int, reason string) {
	t := themeFor(w)
	fmt.Fprintf(
		w,
		"%s step %d: %s\n",
		t.Warn("Route stopped at"),
		index,
		reason,
	)
}

// colorizeDiffLine tints unified-diff-style lines (+ green, - red).
func colorizeDiffLine(t Theme, line string) string {
	switch {
	case strings.HasPrefix(line, "+"):
		return t.Success(line)
	case strings.HasPrefix(line, "-"):
		return t.Danger(line)
	default:
		return t.Muted(line)
	}
}

// PrintWorkflowApplied confirms a submitted workflow and its phase.
func PrintWorkflowApplied(w io.Writer, plan planner.ExecutionPlan, st argo.WorkflowStatus) {
	t := themeFor(w)
	fmt.Fprintf(w, "%s %s\n", t.Success("✓ Submitted:"), plan.Summary)
	fmt.Fprintf(w, "  %s\n", st.Label())
}

// PrintWorkflowStatus prints a read-only workflow phase lookup.
func PrintWorkflowStatus(w io.Writer, st argo.WorkflowStatus) {
	t := themeFor(w)
	fmt.Fprintf(w, "%s\n", t.Heading(fmt.Sprintf("Workflow/%s -n %s", st.Name, st.Namespace)))
	fmt.Fprintf(w, "  phase: %s\n", st.Phase)
	if st.Message != "" {
		fmt.Fprintf(w, "  message: %s\n", st.Message)
	}
	if st.StartedAt != "" {
		fmt.Fprintf(w, "  started: %s\n", st.StartedAt)
	}
	if st.FinishedAt != "" {
		fmt.Fprintf(w, "  finished: %s\n", st.FinishedAt)
	}
}

// PrintPipelineRunApplied confirms a submitted Tekton PipelineRun and its phase.
func PrintPipelineRunApplied(w io.Writer, plan planner.ExecutionPlan, st tekton.PipelineRunStatus) {
	t := themeFor(w)
	fmt.Fprintf(w, "%s %s\n", t.Success("✓ Submitted:"), plan.Summary)
	fmt.Fprintf(w, "  %s\n", st.Label())
}

// PrintScaledObjectApplied confirms a created KEDA ScaledObject and its phase.
func PrintScaledObjectApplied(w io.Writer, plan planner.ExecutionPlan, st keda.ScaledObjectStatus) {
	t := themeFor(w)
	fmt.Fprintf(w, "%s %s\n", t.Success("✓ Applied:"), plan.Summary)
	fmt.Fprintf(w, "  %s\n", st.Label())
}

// PrintClaimApplied confirms a submitted Crossplane claim and its phase.
func PrintClaimApplied(w io.Writer, plan planner.ExecutionPlan, st crossplane.ClaimStatus) {
	t := themeFor(w)
	fmt.Fprintf(w, "%s %s\n", t.Success("✓ Claimed:"), plan.Summary)
	fmt.Fprintf(w, "  %s\n", st.Label())
}

// PrintGitOpsStatusReport prints a read-only Flux/Argo CD sync and health summary (T-043).
func PrintGitOpsStatusReport(w io.Writer, report gitops.StatusReport) {
	t := themeFor(w)
	scope := report.Scope
	if report.Namespace != "" {
		scope = fmt.Sprintf("%s/%s", report.Scope, report.Namespace)
	}
	fmt.Fprintf(w, "%s %s\n", t.Heading("GitOps status:"), t.Accent(scope))
	fmt.Fprintf(w, "%s %s\n", t.Heading("Summary: "), report.Summary)
	if len(report.Notes) > 0 {
		fmt.Fprintln(w, t.Heading("Notes:"))
		for _, n := range report.Notes {
			fmt.Fprintf(w, "  - %s\n", t.Muted(n))
		}
	}
	for _, app := range report.Apps {
		label := fmt.Sprintf("%s %s/%s", app.Engine, app.Kind, app.Name)
		if app.Namespace != "" {
			label += " -n " + app.Namespace
		}
		fmt.Fprintf(w, "%s %s\n", t.Heading("•"), t.Accent(label))
		parts := make([]string, 0, 3)
		if app.Sync != "" {
			parts = append(parts, "sync="+app.Sync)
		}
		if app.Health != "" {
			parts = append(parts, "health="+app.Health)
		}
		if app.Revision != "" {
			parts = append(parts, "rev="+app.Revision)
		}
		if len(parts) > 0 {
			fmt.Fprintf(w, "    %s\n", strings.Join(parts, " "))
		}
		if app.Message != "" {
			fmt.Fprintf(w, "    %s\n", t.Muted(app.Message))
		}
	}
}

// PrintGitOpsSyncApplied confirms a requested Flux reconcile or Argo CD sync.
func PrintGitOpsSyncApplied(w io.Writer, plan planner.ExecutionPlan, st gitops.SyncResult) {
	t := themeFor(w)
	fmt.Fprintf(w, "%s %s\n", t.Success("✓ Synced:"), plan.Summary)
	fmt.Fprintf(w, "  %s\n", st.Label())
}

// PrintIstioTrafficReport prints a read-only VirtualService traffic / canary summary (T-041).
func PrintIstioTrafficReport(w io.Writer, report istio.TrafficReport) {
	t := themeFor(w)
	scope := report.Scope
	if report.Namespace != "" {
		scope = fmt.Sprintf("%s/%s", report.Scope, report.Namespace)
	}
	fmt.Fprintf(w, "%s %s\n", t.Heading("Istio traffic:"), t.Accent(scope))
	fmt.Fprintf(w, "%s %s\n", t.Heading("Summary: "), report.Summary)
	if len(report.Notes) > 0 {
		fmt.Fprintln(w, t.Heading("Notes:"))
		for _, n := range report.Notes {
			fmt.Fprintf(w, "  - %s\n", t.Muted(n))
		}
	}
	for _, vs := range report.VirtualServices {
		label := fmt.Sprintf("VirtualService/%s", vs.Name)
		if vs.Namespace != "" {
			label += " -n " + vs.Namespace
		}
		if vs.Canary {
			label += " (canary)"
		}
		fmt.Fprintf(w, "%s %s\n", t.Heading("•"), t.Accent(label))
		if len(vs.Hosts) > 0 {
			fmt.Fprintf(w, "    hosts: %s\n", strings.Join(vs.Hosts, ", "))
		}
		if len(vs.Gateways) > 0 {
			fmt.Fprintf(w, "    gateways: %s\n", strings.Join(vs.Gateways, ", "))
		}
		for _, route := range vs.Routes {
			prefix := "    route"
			if route.Match != "" {
				prefix = "    route [" + route.Match + "]"
			}
			parts := make([]string, 0, len(route.Splits))
			for _, s := range route.Splits {
				dest := s.Host
				if s.Subset != "" {
					dest += "/" + s.Subset
				}
				parts = append(parts, fmt.Sprintf("%s %d%%", dest, s.Weight))
			}
			fmt.Fprintf(w, "%s: %s\n", prefix, strings.Join(parts, " | "))
		}
	}
}

// PrintPerformanceReport prints a Prometheus-backed workload diagnosis.
func PrintPerformanceReport(w io.Writer, report toolprometheus.PerformanceReport) {
	t := themeFor(w)
	fmt.Fprintf(
		w,
		"%s %s\n",
		t.Heading("Performance:"),
		t.Accent(fmt.Sprintf("Deployment/%s", report.Workload))+fmt.Sprintf(" -n %s (%s)", report.Namespace, report.Window),
	)
	fmt.Fprintf(w, "%s %s\n", t.Heading("Summary:    "), report.Summary)
	fmt.Fprintln(w, t.Heading("Metrics:"))
	for _, metric := range report.Metrics {
		switch {
		case metric.Value != nil:
			fmt.Fprintf(w, "  - %s: %s\n", metric.Name, formatPerformanceValue(*metric.Value, metric.Unit))
		case metric.Error != "":
			fmt.Fprintf(w, "  - %s: %s\n", metric.Name, t.Warn("unavailable ("+metric.Error+")"))
		default:
			fmt.Fprintf(w, "  - %s: %s\n", metric.Name, t.Muted("no matching series"))
		}
	}
	if len(report.Findings) > 0 {
		fmt.Fprintln(w, t.Heading("Findings:"))
		for _, finding := range report.Findings {
			fmt.Fprintf(w, "  - %s\n", finding)
		}
	}
	if report.Suggestion != nil {
		fmt.Fprintf(
			w,
			"%s scale Deployment/%s from %d to %d replicas (%s).\n",
			t.Accent("Suggestion:"),
			report.Workload,
			report.Suggestion.Current,
			report.Suggestion.Suggested,
			report.Suggestion.Reason,
		)
	}
}

// PrintOptimizeReport prints a read-only cluster optimize report (T-052+).
func PrintOptimizeReport(w io.Writer, report optimize.Report) {
	t := themeFor(w)
	scope := report.Scope
	if report.Namespace != "" {
		scope = fmt.Sprintf("%s/%s", report.Scope, report.Namespace)
	}
	fmt.Fprintf(w, "%s %s", t.Heading("Optimize:"), t.Accent(scope))
	if report.ClusterContext != "" {
		fmt.Fprintf(w, " @%s", t.Accent(report.ClusterContext))
	}
	if report.Window != "" {
		fmt.Fprintf(w, " (%s)", report.Window)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s %s\n", t.Heading("Summary: "), report.Summary)
	if len(report.Findings) > 0 {
		fmt.Fprintln(w, t.Heading("Findings:"))
		for _, f := range report.Findings {
			ctx := ""
			if f.ClusterContext != "" {
				ctx = fmt.Sprintf(" [%s]", f.ClusterContext)
			}
			fmt.Fprintf(w, "  - [%s]%s %s: %s\n", t.Severity(f.Severity), ctx, t.Accent(f.Title), f.Message)
		}
	}
	if len(report.Workloads) > 0 {
		fmt.Fprintln(w, t.Heading("Inventory:"))
		const maxRows = 40
		for i, wl := range report.Workloads {
			if i >= maxRows {
				fmt.Fprintf(w, "  … %d more workloads\n", len(report.Workloads)-maxRows)
				break
			}
			res := formatWorkloadResources(wl)
			fmt.Fprintf(w, "  - %s/%s %s  replicas %d/%d  %s\n",
				wl.Namespace, wl.Name, t.Muted(wl.Kind), wl.ReadyReplicas, wl.Replicas, res)
		}
	}
	if len(report.Idle) > 0 {
		fmt.Fprintln(w, t.Heading("Idle:"))
		for _, idle := range report.Idle {
			fmt.Fprintf(w, "  - %s\n", idle.Message)
		}
	}
	if len(report.Rightsizing) > 0 {
		fmt.Fprintln(w, t.Heading("Rightsizing:"))
		for _, d := range report.Rightsizing {
			fmt.Fprintf(w, "  - %s\n", d.Message)
		}
	}
	if len(report.HPA) > 0 {
		fmt.Fprintln(w, t.Heading("HPA:"))
		for _, h := range report.HPA {
			fmt.Fprintf(w, "  - %s\n", h.Message)
		}
	}
	if len(report.Suggestions) > 0 {
		fmt.Fprintln(w, t.Heading("Suggestions:"))
		for _, s := range report.Suggestions {
			line := fmt.Sprintf("  - %s: %s", t.Accent(s.Title), s.Message)
			if s.ActionHint != "" {
				line += fmt.Sprintf(" (%s)", t.Muted(s.ActionHint))
			}
			fmt.Fprintln(w, line)
		}
	}
	fmt.Fprintln(w, t.Heading("Sections:"))
	printOptimizeSection(w, t, "inventory", report.Sections.Inventory)
	printOptimizeSection(w, t, "idle", report.Sections.Idle)
	printOptimizeSection(w, t, "rightsizing", report.Sections.Rightsizing)
	printOptimizeSection(w, t, "hpa", report.Sections.HPA)
}

// PrintGraphReport prints a terminal-friendly service dependency adjacency list (T-059).
func PrintGraphReport(w io.Writer, report graph.Report) {
	t := themeFor(w)
	scope := report.Scope
	if report.Namespace != "" {
		scope = fmt.Sprintf("%s/%s", report.Scope, report.Namespace)
	}
	fmt.Fprintf(w, "%s %s\n", t.Heading("Service graph:"), t.Accent(scope))
	fmt.Fprintf(w, "%s %s\n", t.Heading("Summary: "), report.Summary)
	if len(report.Notes) > 0 {
		fmt.Fprintln(w, t.Heading("Notes:"))
		for _, n := range report.Notes {
			fmt.Fprintf(w, "  - %s\n", t.Muted(n))
		}
	}
	if len(report.Nodes) > 0 {
		fmt.Fprintln(w, t.Heading("Nodes:"))
		const maxNodes = 50
		for i, n := range report.Nodes {
			if i >= maxNodes {
				fmt.Fprintf(w, "  … %d more nodes\n", len(report.Nodes)-maxNodes)
				break
			}
			fmt.Fprintf(w, "  - %s %s\n", t.Muted(n.Kind), t.Accent(n.ID))
		}
	}
	if len(report.Edges) > 0 {
		fmt.Fprintln(w, t.Heading("Edges:"))
		const maxEdges = 80
		for i, e := range report.Edges {
			if i >= maxEdges {
				fmt.Fprintf(w, "  … %d more edges\n", len(report.Edges)-maxEdges)
				break
			}
			line := fmt.Sprintf("  - %s -[%s/%s]→ %s", e.From, e.Source, e.Type, e.To)
			if e.Detail != "" {
				line += " " + t.Muted("("+e.Detail+")")
			}
			fmt.Fprintln(w, line)
		}
	} else {
		fmt.Fprintln(w, t.Muted("No edges found in scope."))
	}
}

func formatWorkloadResources(wl optimize.Workload) string {
	parts := make([]string, 0, 4)
	if wl.CPURequest != "" {
		parts = append(parts, "cpuReq="+wl.CPURequest)
	}
	if wl.MemoryRequest != "" {
		parts = append(parts, "memReq="+wl.MemoryRequest)
	}
	if wl.CPULimit != "" {
		parts = append(parts, "cpuLim="+wl.CPULimit)
	}
	if wl.MemoryLimit != "" {
		parts = append(parts, "memLim="+wl.MemoryLimit)
	}
	if len(parts) == 0 {
		if wl.MissingReq {
			return "no requests/limits"
		}
		return "-"
	}
	return strings.Join(parts, " ")
}

func printOptimizeSection(w io.Writer, t Theme, name string, sec optimize.SectionStatus) {
	msg := sec.Message
	if msg == "" {
		msg = sec.Status
	}
	fmt.Fprintf(w, "  - %s: %s — %s\n", name, t.Muted(sec.Status), msg)
}

// PrintTrace prints a parent-before-child distributed span tree and bottlenecks.
func PrintTrace(w io.Writer, report toolotel.TraceReport) {
	t := themeFor(w)
	trace := report.Trace
	fmt.Fprintf(w, "%s %s\n", t.Heading("Trace:"), t.Accent(trace.TraceID))
	root := trace.RootService
	if trace.RootService != "" && trace.RootOperation != "" {
		root += " — "
	}
	root += trace.RootOperation
	if root != "" {
		fmt.Fprintf(w, "%s %s (%s)\n", t.Heading("Root: "), root, trace.Duration)
	}
	if report.Summary != "" {
		fmt.Fprintf(w, "%s %s\n", t.Heading("Summary:"), report.Summary)
	}
	fmt.Fprintln(w, t.Heading("Spans:"))
	rows := report.Spans
	if len(rows) == 0 {
		rows = toolotel.WalkSpans(trace)
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "  "+t.Muted("(no spans)"))
	} else {
		for _, row := range rows {
			span := row.Span
			label := span.Operation
			if span.Service != "" {
				label = span.Service + ": " + label
			}
			status := ""
			if span.Status != "" {
				status = " [" + span.Status + "]"
			}
			fmt.Fprintf(
				w,
				"  %s└─ %s (%s)%s\n",
				strings.Repeat("  ", row.Depth),
				label,
				span.Duration,
				status,
			)
		}
	}
	if len(report.Bottlenecks) == 0 {
		return
	}
	fmt.Fprintln(w, t.Heading("Bottlenecks:"))
	for _, item := range report.Bottlenecks {
		fmt.Fprintf(w, "  - %s\n", t.Warn(item.Message))
	}
}

// PrintDashboardResult prints Grafana matches or one dashboard panel summary.
func PrintDashboardResult(w io.Writer, result toolgrafana.ShowResult) {
	t := themeFor(w)
	if result.Dashboard != nil {
		dashboard := result.Dashboard
		fmt.Fprintf(
			w,
			"%s %s\n",
			t.Heading("Dashboard:"),
			t.Accent(dashboard.Title),
		)
		fmt.Fprintf(w, "%s %s\n", t.Heading("UID:      "), dashboard.UID)
		if dashboard.URL != "" {
			fmt.Fprintf(w, "%s %s\n", t.Heading("URL:      "), dashboard.URL)
		}
		if len(dashboard.Panels) == 0 {
			fmt.Fprintln(w, t.Muted("No panels found."))
			return
		}
		fmt.Fprintln(w, t.Heading("Panels:"))
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, t.tabHeading("ID\tTITLE\tTYPE\tDATASOURCE"))
		for _, panel := range dashboard.Panels {
			fmt.Fprintf(
				tw,
				"%d\t%s\t%s\t%s\n",
				panel.ID,
				panel.Title,
				panel.Type,
				grafanaDatasourceLabel(panel.Datasource),
			)
		}
		_ = tw.Flush()
		return
	}

	if len(result.Dashboards) == 0 {
		if result.Query == "" {
			fmt.Fprintln(w, t.Muted("No Grafana dashboards found."))
		} else {
			fmt.Fprintf(
				w,
				"%s\n",
				t.Muted(fmt.Sprintf("No Grafana dashboards found for %q.", result.Query)),
			)
		}
		return
	}
	fmt.Fprintln(w, t.Heading("Grafana dashboards:"))
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, t.tabHeading("TITLE\tFOLDER\tUID\tURL"))
	for _, dashboard := range result.Dashboards {
		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\n",
			dashboard.Title,
			dashboard.FolderTitle,
			dashboard.UID,
			dashboard.URL,
		)
	}
	_ = tw.Flush()
}

func grafanaDatasourceLabel(source toolgrafana.Datasource) string {
	switch {
	case source.Name != "":
		return source.Name
	case source.UID != "":
		return source.UID
	default:
		return source.Type
	}
}

func formatPerformanceValue(value float64, unit string) string {
	switch unit {
	case "bytes":
		const (
			kib = 1024
			mib = 1024 * kib
			gib = 1024 * mib
		)
		switch {
		case value >= gib:
			return fmt.Sprintf("%.2f GiB", value/gib)
		case value >= mib:
			return fmt.Sprintf("%.2f MiB", value/mib)
		case value >= kib:
			return fmt.Sprintf("%.2f KiB", value/kib)
		default:
			return fmt.Sprintf("%.0f bytes", value)
		}
	case "seconds":
		return fmt.Sprintf("%.3fs", value)
	case "cores":
		return fmt.Sprintf("%.3f cores", value)
	case "replicas":
		return fmt.Sprintf("%.0f", value)
	default:
		return fmt.Sprintf("%.3f %s", value, unit)
	}
}

// PrintApplied confirms successful execution.
func PrintApplied(w io.Writer, plan planner.ExecutionPlan) {
	t := themeFor(w)
	fmt.Fprintf(w, "%s %s\n", t.Success("✓ Applied:"), plan.Summary)
}

// PrintQueryResult prints a read-only list/get table.
func PrintQueryResult(w io.Writer, res cluster.Result) {
	t := themeFor(w)
	if len(res.Rows) == 0 {
		fmt.Fprintf(w, "%s\n", t.Muted(fmt.Sprintf("No %s found.", res.Kind)))
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, t.tabHeading(strings.Join(res.Headers, "\t")))
	for _, row := range res.Rows {
		fmt.Fprintln(tw, strings.Join(queryRowCells(res.Headers, row), "\t"))
	}
	_ = tw.Flush()
	if res.Truncated || res.Continue != "" {
		fmt.Fprintf(w, "%s\n", t.Muted("(truncated — more results available)"))
	}
}

func queryRowCells(headers []string, row cluster.Row) []string {
	if len(headers) == 0 {
		cols := []string{row.Namespace, row.Name, row.Ready, row.Status}
		if row.Extra != "" {
			cols = append(cols, strings.Split(row.Extra, "\t")...)
		}
		return cols
	}
	extras := []string{}
	if row.Extra != "" {
		extras = strings.Split(row.Extra, "\t")
	}
	ei := 0
	cols := make([]string, 0, len(headers))
	for _, h := range headers {
		switch strings.ToUpper(h) {
		case "NAMESPACE":
			cols = append(cols, row.Namespace)
		case "NAME":
			cols = append(cols, row.Name)
		case "READY":
			cols = append(cols, row.Ready)
		case "STATUS", "TYPE", "CLUSTER-IP", "UP-TO-DATE":
			if strings.EqualFold(h, "TYPE") {
				cols = append(cols, row.Ready)
			} else if strings.EqualFold(h, "STATUS") {
				cols = append(cols, row.Status)
			} else if strings.EqualFold(h, "CLUSTER-IP") || strings.EqualFold(h, "UP-TO-DATE") {
				cols = append(cols, row.Status)
			} else {
				cols = append(cols, row.Status)
			}
		case "AGE":
			if len(extras) == 1 {
				cols = append(cols, extras[0])
			} else if ei < len(extras) {
				cols = append(cols, extras[ei])
				ei++
			} else {
				cols = append(cols, row.Extra)
			}
		default:
			if ei < len(extras) {
				cols = append(cols, extras[ei])
				ei++
			} else {
				cols = append(cols, "")
			}
		}
	}
	return cols
}

// PrintExplain prints an investigation report.
func PrintExplain(w io.Writer, rep cluster.ExplainReport) {
	t := themeFor(w)
	fmt.Fprintf(w, "%s %s\n", t.Heading("Target: "), t.Accent(fmt.Sprintf("%s/%s", rep.Kind, rep.Target))+fmt.Sprintf(" -n %s", rep.Namespace))
	fmt.Fprintf(w, "%s %s\n", t.Heading("Status: "), rep.Status)
	fmt.Fprintf(w, "%s %s\n", t.Heading("Summary:"), rep.Summary)
	if len(rep.Chain) > 0 {
		fmt.Fprintln(w, t.Heading("Investigation chain:"))
		for _, step := range rep.Chain {
			fmt.Fprintf(w, "  - %s/%s — %s\n", step.Level, step.Name, t.Muted(step.Detail))
		}
	}
	if len(rep.Findings) > 0 {
		fmt.Fprintln(w, t.Heading("Findings:"))
		for _, f := range rep.Findings {
			fmt.Fprintf(w, "  - [%s] %s: %s\n", t.Severity(f.Severity), t.Accent(f.Code), f.Message)
		}
	}
	if len(rep.Events) > 0 {
		fmt.Fprintln(w, t.Heading("Recent events:"))
		for _, ev := range rep.Events {
			fmt.Fprintf(w, "  - %s\n", t.Muted(ev))
		}
	}
	if strings.TrimSpace(rep.LogTail) != "" {
		header := fmt.Sprintf("Log tail: Pod/%s -n %s", rep.LogPod, rep.Namespace)
		if rep.LogContainer != "" {
			header += " container=" + rep.LogContainer
		}
		fmt.Fprintln(w, t.Heading(header))
		fmt.Fprintln(w, t.Muted(strings.TrimRight(rep.LogTail, "\n")))
	}
}

// PrintLogs prints a pod log tail.
func PrintLogs(w io.Writer, res cluster.LogsResult) {
	t := themeFor(w)
	header := fmt.Sprintf("Logs: Pod/%s -n %s", res.Pod, res.Namespace)
	if res.Container != "" {
		header += " container=" + res.Container
	}
	header += fmt.Sprintf(" (last %d lines)", res.Tail)
	fmt.Fprintln(w, t.Heading(header))
	body := strings.TrimRight(res.Body, "\n")
	if body == "" {
		fmt.Fprintln(w, t.Muted("(no log output)"))
		return
	}
	fmt.Fprintln(w, body)
}

// PrintDescribe prints a compact describe report.
func PrintDescribe(w io.Writer, rep cluster.DescribeReport) {
	t := themeFor(w)
	fmt.Fprintf(w, "%s\n", t.Heading(fmt.Sprintf("%s/%s -n %s", rep.Kind, rep.Name, rep.Namespace)))
	fmt.Fprintf(w, "%s %s\n", t.Heading("Status: "), rep.Status)
	for _, line := range rep.Lines {
		fmt.Fprintln(w, line)
	}
}

// PrintSuggestions prints explain follow-up prompts / fix ideas.
func PrintSuggestions(w io.Writer, suggestions []suggest.Suggestion) {
	if len(suggestions) == 0 {
		return
	}
	t := themeFor(w)
	fmt.Fprintln(w, t.Heading("Suggestions:"))
	for _, s := range suggestions {
		fmt.Fprintf(w, "  - [%s] %s\n", t.Accent(s.Code), s.Title)
		if s.Summary != "" {
			fmt.Fprintf(w, "      %s\n", t.Muted(s.Summary))
		}
		if hint := suggest.FormatPromptHint(s); hint != "" {
			fmt.Fprintf(w, "      %s %s\n", t.Accent("Try:"), hint)
		}
	}
}
