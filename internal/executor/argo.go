package executor

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/tools/argo"
)

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

// ApplyArgo submits Argo workflow plans and returns the latest workflow status.
func ApplyArgo(ctx context.Context, cfg *rest.Config, plan planner.ExecutionPlan) (argo.WorkflowStatus, error) {
	if cfg == nil {
		return argo.WorkflowStatus{}, fmt.Errorf("argo apply: rest config is nil")
	}
	var last argo.WorkflowStatus
	for _, a := range plan.Actions {
		switch a.Op {
		case planner.OpWorkflowCreate:
			st, err := argo.Submit(ctx, cfg, a.Manifest)
			if err != nil {
				return last, err
			}
			last = st
		default:
			return last, fmt.Errorf("executor: unsupported argo op %q", a.Op)
		}
	}
	return last, nil
}

// WorkflowTargets returns workflow objects from a plan for wait/status helpers.
func WorkflowTargets(plan planner.ExecutionPlan) []planner.ObjectRef {
	if !IsArgoWorkflowPlan(plan) {
		return nil
	}
	out := make([]planner.ObjectRef, 0, len(plan.Actions))
	for _, a := range plan.Actions {
		if a.Object.Name == "" {
			continue
		}
		out = append(out, a.Object)
	}
	return out
}
