package executor

import (
	"testing"

	"github.com/kprompt/kprompt/internal/planner"
)

func TestIsCrossplanePlan(t *testing.T) {
	plan := planner.ExecutionPlan{
		Actions: []planner.Action{{
			Op:      planner.OpClaimCreate,
			Backend: "crossplane",
		}},
	}
	if !IsCrossplanePlan(plan) {
		t.Fatal("expected crossplane plan")
	}
	mixed := planner.ExecutionPlan{
		Actions: []planner.Action{
			{Backend: "crossplane", Op: planner.OpClaimCreate},
			{Backend: "kubernetes", Op: planner.OpScale},
		},
	}
	if IsCrossplanePlan(mixed) {
		t.Fatal("mixed should not be crossplane-only")
	}
}
