package executor

import "github.com/kprompt/kprompt/internal/planner"

// IsArgoWorkflowPlan reports whether the plan only contains Argo workflow create actions.
func IsArgoWorkflowPlan(plan planner.ExecutionPlan) bool {
	if len(plan.Actions) == 0 {
		return false
	}
	for _, a := range plan.Actions {
		if a.Backend != "argo" || a.Op != planner.OpWorkflowCreate {
			return false
		}
	}
	return true
}
