package ui

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
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
			line := fmt.Sprintf("  %d. %s %s/%s", i+1, a.Op, a.Object.Kind, a.Object.Name)
			if a.Object.Namespace != "" {
				line += " -n " + a.Object.Namespace
			}
			if a.Replicas != nil && a.Op == planner.OpScale {
				line += fmt.Sprintf(" → %d replicas", *a.Replicas)
			}
			fmt.Fprintln(w, line)
			if a.Diff != "" {
				fmt.Fprintln(w, "     Diff:")
				for _, dl := range strings.Split(a.Diff, "\n") {
					if dl == "" {
						continue
					}
					fmt.Fprintf(w, "       %s\n", dl)
				}
			}
		}
	}
	if plan.RequiresApproval {
		fmt.Fprintln(w, "Next: confirm interactively on a TTY, or re-run with --approve.")
	}
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

// PrintExplain prints an explain-lite report.
func PrintExplain(w io.Writer, rep cluster.ExplainReport) {
	fmt.Fprintf(w, "Target:  %s/%s -n %s\n", rep.Kind, rep.Target, rep.Namespace)
	fmt.Fprintf(w, "Status:  %s\n", rep.Status)
	fmt.Fprintf(w, "Summary: %s\n", rep.Summary)
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
