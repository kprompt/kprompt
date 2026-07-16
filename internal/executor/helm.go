package executor

import (
	"context"
	"fmt"

	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/tools/helm"
)

// ApplyHelm runs helm-backed plan actions.
func ApplyHelm(ctx context.Context, plan planner.ExecutionPlan) error {
	for _, a := range plan.Actions {
		switch a.Op {
		case planner.OpHelmRepo, planner.OpHelmInstall:
			if len(a.Command) == 0 {
				return fmt.Errorf("helm action missing command")
			}
			if err := helm.Run(ctx, a.Command); err != nil {
				return fmt.Errorf("%s: %w", a.Op, err)
			}
		default:
			return fmt.Errorf("executor: unsupported helm op %q", a.Op)
		}
	}
	return nil
}

// IsHelmPlan reports whether every mutating action is helm-backed.
func IsHelmPlan(plan planner.ExecutionPlan) bool {
	if len(plan.Actions) == 0 {
		return false
	}
	for _, a := range plan.Actions {
		if a.Backend != "helm" {
			return false
		}
	}
	return true
}
