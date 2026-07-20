package executor

import (
	"context"
	"fmt"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/tools/tekton"
)

// IsTektonPlan reports whether the plan only contains Tekton PipelineRun create actions.
func IsTektonPlan(plan planner.ExecutionPlan) bool {
	if len(plan.Actions) == 0 {
		return false
	}
	for _, a := range plan.Actions {
		if a.Backend != "tekton" || a.Op != planner.OpPipelineRunCreate {
			return false
		}
	}
	return true
}

// ApplyTekton submits Tekton PipelineRun plans and returns the latest status.
func ApplyTekton(ctx context.Context, cfg *rest.Config, plan planner.ExecutionPlan) (tekton.PipelineRunStatus, error) {
	if cfg == nil {
		return tekton.PipelineRunStatus{}, fmt.Errorf("tekton apply: rest config is nil")
	}
	var last tekton.PipelineRunStatus
	for _, a := range plan.Actions {
		switch a.Op {
		case planner.OpPipelineRunCreate:
			st, err := tekton.Submit(ctx, cfg, a.Manifest)
			if err != nil {
				return last, err
			}
			last = st
		default:
			return last, fmt.Errorf("executor: unsupported tekton op %q", a.Op)
		}
	}
	return last, nil
}
