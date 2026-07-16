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
)

// PrintDenied writes a hard-deny message.
func PrintDenied(w io.Writer, msg string) {
	fmt.Fprintln(w, msg)
}

// PrintPlan prints a human-readable execution plan.
func PrintPlan(w io.Writer, plan planner.ExecutionPlan, risk safety.Result) {
	fmt.Fprintf(w, "Intent: %s\n", plan.Intent.Kind)
	if plan.Summary != "" {
		fmt.Fprintf(w, "Plan:   %s\n", plan.Summary)
	}
	fmt.Fprintf(w, "Risk:   %s\n", risk.Risk)
	if len(plan.Actions) > 0 {
		fmt.Fprintln(w, "Actions:")
		for i, a := range plan.Actions {
			var line string
			if len(a.Command) > 0 {
				line = fmt.Sprintf("  %d. $ %s", i+1, strings.Join(a.Command, " "))
			} else {
				line = fmt.Sprintf("  %d. %s %s/%s", i+1, a.Op, a.Object.Kind, a.Object.Name)
				if a.Object.Namespace != "" {
					line += " -n " + a.Object.Namespace
				}
				if a.Replicas != nil && a.Op == planner.OpScale {
					line += fmt.Sprintf(" → %d replicas", *a.Replicas)
				}
			}
			fmt.Fprintln(w, line)
			if a.Diff != "" && (len(a.Command) == 0 || a.Diff != strings.Join(a.Command, " ")) {
				fmt.Fprintln(w, "     Diff:")
				for _, dl := range strings.Split(a.Diff, "\n") {
					if dl == "" {
						continue
					}
					fmt.Fprintf(w, "       %s\n", dl)
				}
			}
			if strings.TrimSpace(a.Manifest) != "" {
				fmt.Fprintln(w, "     Preview:")
				for _, line := range strings.Split(strings.TrimRight(a.Manifest, "\n"), "\n") {
					fmt.Fprintf(w, "       %s\n", line)
				}
			}
		}
	}
	if plan.RequiresApproval {
		fmt.Fprintln(w, "Next: confirm interactively on a TTY, or re-run with --approve.")
	}
}

// PrintWorkflowReady confirms a generated workflow manifest without submitting it (T-030).
func PrintWorkflowReady(w io.Writer, plan planner.ExecutionPlan) {
	fmt.Fprintf(w, "✓ Workflow manifest ready: %s\n", plan.Summary)
	fmt.Fprintln(w, "Submit to the cluster is planned for a follow-up release (T-030).")
}

// PrintApplied confirms successful execution.
func PrintApplied(w io.Writer, plan planner.ExecutionPlan) {
	fmt.Fprintf(w, "✓ Applied: %s\n", plan.Summary)
}

// PrintQueryResult prints a read-only list/get table.
func PrintQueryResult(w io.Writer, res cluster.Result) {
	if len(res.Rows) == 0 {
		fmt.Fprintf(w, "No %s found.\n", res.Kind)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(res.Headers, "\t"))
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
	fmt.Fprintf(w, "Target:  %s/%s -n %s\n", rep.Kind, rep.Target, rep.Namespace)
	fmt.Fprintf(w, "Status:  %s\n", rep.Status)
	fmt.Fprintf(w, "Summary: %s\n", rep.Summary)
	if len(rep.Chain) > 0 {
		fmt.Fprintln(w, "Investigation chain:")
		for _, step := range rep.Chain {
			fmt.Fprintf(w, "  - %s/%s — %s\n", step.Level, step.Name, step.Detail)
		}
	}
	if len(rep.Findings) > 0 {
		fmt.Fprintln(w, "Findings:")
		for _, f := range rep.Findings {
			fmt.Fprintf(w, "  - [%s] %s: %s\n", f.Severity, f.Code, f.Message)
		}
	}
	if len(rep.Events) > 0 {
		fmt.Fprintln(w, "Recent events:")
		for _, ev := range rep.Events {
			fmt.Fprintf(w, "  - %s\n", ev)
		}
	}
	if strings.TrimSpace(rep.LogTail) != "" {
		header := fmt.Sprintf("Log tail: Pod/%s -n %s", rep.LogPod, rep.Namespace)
		if rep.LogContainer != "" {
			header += " container=" + rep.LogContainer
		}
		fmt.Fprintln(w, header)
		fmt.Fprintln(w, strings.TrimRight(rep.LogTail, "\n"))
	}
}

// PrintLogs prints a pod log tail.
func PrintLogs(w io.Writer, res cluster.LogsResult) {
	header := fmt.Sprintf("Logs: Pod/%s -n %s", res.Pod, res.Namespace)
	if res.Container != "" {
		header += " container=" + res.Container
	}
	header += fmt.Sprintf(" (last %d lines)", res.Tail)
	fmt.Fprintln(w, header)
	body := strings.TrimRight(res.Body, "\n")
	if body == "" {
		fmt.Fprintln(w, "(no log output)")
		return
	}
	fmt.Fprintln(w, body)
}

// PrintDescribe prints a compact describe report.
func PrintDescribe(w io.Writer, rep cluster.DescribeReport) {
	fmt.Fprintf(w, "%s/%s -n %s\n", rep.Kind, rep.Name, rep.Namespace)
	fmt.Fprintf(w, "Status:  %s\n", rep.Status)
	for _, line := range rep.Lines {
		fmt.Fprintln(w, line)
	}
}

// PrintSuggestions prints explain follow-up prompts / fix ideas.
func PrintSuggestions(w io.Writer, suggestions []suggest.Suggestion) {
	if len(suggestions) == 0 {
		return
	}
	fmt.Fprintln(w, "Suggestions:")
	for _, s := range suggestions {
		fmt.Fprintf(w, "  - [%s] %s\n", s.Code, s.Title)
		if s.Summary != "" {
			fmt.Fprintf(w, "      %s\n", s.Summary)
		}
		if hint := suggest.FormatPromptHint(s); hint != "" {
			fmt.Fprintf(w, "      Try: %s\n", hint)
		}
	}
}
