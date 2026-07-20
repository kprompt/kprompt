package executor

import (
	"testing"

	"github.com/kprompt/kprompt/internal/planner"
)

func TestIsKEDAPlan(t *testing.T) {
	plan := planner.ExecutionPlan{
		Actions: []planner.Action{{
			Op:      planner.OpScaledObjectCreate,
			Backend: "keda",
		}},
	}
	if !IsKEDAPlan(plan) {
		t.Fatal("expected keda plan")
	}
	mixed := planner.ExecutionPlan{
		Actions: []planner.Action{
			{Backend: "keda", Op: planner.OpScaledObjectCreate},
			{Backend: "kubernetes", Op: planner.OpScale},
		},
	}
	if IsKEDAPlan(mixed) {
		t.Fatal("mixed should not be keda-only")
	}
}
