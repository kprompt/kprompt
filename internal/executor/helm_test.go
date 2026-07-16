package executor

import (
	"testing"

	"github.com/kprompt/kprompt/internal/planner"
)

func TestIsHelmPlan(t *testing.T) {
	plan := planner.ExecutionPlan{
		Actions: []planner.Action{{
			Backend: "helm",
			Op:      planner.OpHelmRepo,
			Command: []string{"helm", "repo", "add", "bitnami", "https://example"},
		}},
	}
	if !IsHelmPlan(plan) {
		t.Fatal("expected helm plan")
	}
	mixed := planner.ExecutionPlan{
		Actions: []planner.Action{
			{Backend: "helm", Op: planner.OpHelmInstall},
			{Backend: "kubernetes", Op: planner.OpCreate},
		},
	}
	if IsHelmPlan(mixed) {
		t.Fatal("expected mixed plan to be non-helm")
	}
}
