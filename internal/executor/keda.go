package executor

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/tools/keda"
)

// IsKEDAPlan reports whether the plan only contains KEDA ScaledObject create actions.
func IsKEDAPlan(plan planner.ExecutionPlan) bool {
	if len(plan.Actions) == 0 {
		return false
	}
	for _, a := range plan.Actions {
		if a.Backend != "keda" || a.Op != planner.OpScaledObjectCreate {
			return false
		}
	}
	return true
}

// ApplyKEDA submits KEDA ScaledObject plans and returns the latest status.
func ApplyKEDA(ctx context.Context, cfg *rest.Config, plan planner.ExecutionPlan) (keda.ScaledObjectStatus, error) {
	if cfg == nil {
		return keda.ScaledObjectStatus{}, fmt.Errorf("keda apply: rest config is nil")
	}
	var last keda.ScaledObjectStatus
	for _, a := range plan.Actions {
		switch a.Op {
		case planner.OpScaledObjectCreate:
			st, err := keda.Submit(ctx, cfg, a.Manifest)
			if err != nil {
				return last, err
			}
			last = st
		default:
			return last, fmt.Errorf("executor: unsupported keda op %q", a.Op)
		}
	}
	return last, nil
}
