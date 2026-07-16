package executor

import (
	"testing"

	"github.com/kprompt/kprompt/internal/planner"
)

func TestIsArgoWorkflowPlan(t *testing.T) {
	plan := planner.ExecutionPlan{
		Actions: []planner.Action{{
			Backend: "argo",
			Op:      planner.OpWorkflowCreate,
		}},
	}
	if !IsArgoWorkflowPlan(plan) {
		t.Fatal("expected argo workflow plan")
	}
	mixed := planner.ExecutionPlan{
		Actions: []planner.Action{
			{Backend: "argo", Op: planner.OpWorkflowCreate},
			{Backend: "kubernetes", Op: planner.OpCreate},
		},
	}
	if IsArgoWorkflowPlan(mixed) {
		t.Fatal("expected mixed plan to be false")
	}
}
