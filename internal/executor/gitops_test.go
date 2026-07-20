package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/planner"
)

func TestIsGitOpsSyncPlan(t *testing.T) {
	plan := planner.ExecutionPlan{
		Actions: []planner.Action{{
			Op:      planner.OpGitOpsSync,
			Backend: "gitops",
		}},
	}
	if !IsGitOpsSyncPlan(plan) {
		t.Fatal("expected gitops sync plan")
	}
	status := planner.ExecutionPlan{
		Actions: []planner.Action{{
			Op:      planner.OpGitOpsStatus,
			Backend: "gitops",
		}},
	}
	if IsGitOpsSyncPlan(status) {
		t.Fatal("status should not be sync plan")
	}
	mixed := planner.ExecutionPlan{
		Actions: []planner.Action{
			{Backend: "gitops", Op: planner.OpGitOpsSync},
			{Backend: "kubernetes", Op: planner.OpScale},
		},
	}
	if IsGitOpsSyncPlan(mixed) {
		t.Fatal("mixed should not be gitops-only")
	}
}

func TestRunnerApplyRejectsGitOpsSync(t *testing.T) {
	r := &Runner{}
	err := r.Apply(context.Background(), planner.ExecutionPlan{
		Actions: []planner.Action{{
			Op:      planner.OpGitOpsSync,
			Backend: "gitops",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "ApplyGitOpsSync") {
		t.Fatalf("expected redirect to ApplyGitOpsSync, got %v", err)
	}
}
