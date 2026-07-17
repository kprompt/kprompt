package ui

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
	"github.com/kprompt/kprompt/internal/suggest"
	"github.com/kprompt/kprompt/internal/tools/argo"
	toolotel "github.com/kprompt/kprompt/internal/tools/otel"
	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
)

// PrintDenied writes a hard-deny message.
func PrintDenied(w io.Writer, msg string) {
	t := themeFor(w)
	fmt.Fprintln(w, t.Danger(msg))
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

// PrintTrace prints a parent-before-child distributed span tree.
func PrintTrace(w io.Writer, trace toolotel.Trace) {
	t := themeFor(w)
	fmt.Fprintf(w, "%s %s\n", t.Heading("Trace:"), t.Accent(trace.TraceID))
	root := trace.RootService
	if trace.RootService != "" && trace.RootOperation != "" {
		root += " — "
	}
	root += trace.RootOperation
	if root != "" {
		fmt.Fprintf(w, "%s %s (%s)\n", t.Heading("Root: "), root, trace.Duration)
	}
	fmt.Fprintln(w, t.Heading("Spans:"))
	rows := toolotel.WalkSpans(trace)
	if len(rows) == 0 {
		fmt.Fprintln(w, "  "+t.Muted("(no spans)"))
		return
	}
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
		cols := []string{row.Namespace, row.Name, row.Ready, row.Status}
		if row.Extra != "" {
			cols = append(cols, strings.Split(row.Extra, "\t")...)
		}
		fmt.Fprintln(tw, strings.Join(cols, "\t"))
	}
	_ = tw.Flush()
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
