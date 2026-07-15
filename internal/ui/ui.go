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
			if a.Replicas != nil {
				line += fmt.Sprintf(" → %d replicas", *a.Replicas)
			}
			fmt.Fprintln(w, line)
			if a.Diff != "" {
				fmt.Fprintf(w, "     %s\n", a.Diff)
			}
		}
	}
	if plan.RequiresApproval {
		fmt.Fprintln(w, strings.TrimSpace(`
Next: re-run with --approve to apply, or review and cancel.`))
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
