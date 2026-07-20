package executor

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/tools/crossplane"
)

// IsCrossplanePlan reports whether the plan only contains Crossplane claim create actions.
func IsCrossplanePlan(plan planner.ExecutionPlan) bool {
	if len(plan.Actions) == 0 {
		return false
	}
	for _, a := range plan.Actions {
		if a.Backend != "crossplane" || a.Op != planner.OpClaimCreate {
			return false
		}
	}
	return true
}

// ApplyCrossplane submits Crossplane claim plans and returns the latest status.
func ApplyCrossplane(ctx context.Context, cfg *rest.Config, plan planner.ExecutionPlan) (crossplane.ClaimStatus, error) {
	if cfg == nil {
		return crossplane.ClaimStatus{}, fmt.Errorf("crossplane apply: rest config is nil")
	}
	var last crossplane.ClaimStatus
	for _, a := range plan.Actions {
		switch a.Op {
		case planner.OpClaimCreate:
			st, err := crossplane.Submit(ctx, cfg, a.Manifest)
			if err != nil {
				return last, err
			}
			last = st
		default:
			return last, fmt.Errorf("executor: unsupported crossplane op %q", a.Op)
		}
	}
	return last, nil
}
