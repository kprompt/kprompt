package executor

import (
	"testing"

	"github.com/kprompt/kprompt/internal/planner"
)

func TestIsTektonPlan(t *testing.T) {
	plan := planner.ExecutionPlan{
		Actions: []planner.Action{{
			Op:      planner.OpPipelineRunCreate,
			Backend: "tekton",
		}},
	}
	if !IsTektonPlan(plan) {
		t.Fatal("expected tekton plan")
	}
	mixed := planner.ExecutionPlan{
		Actions: []planner.Action{
			{Backend: "tekton", Op: planner.OpPipelineRunCreate},
			{Backend: "kubernetes", Op: planner.OpScale},
		},
	}
	if IsTektonPlan(mixed) {
		t.Fatal("mixed should not be tekton-only")
	}
}
