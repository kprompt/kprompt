package safety

import (
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
)

func TestCheckGitOpsPromptDeniesWipe(t *testing.T) {
	r := CheckGitOpsPrompt("delete all argocd applications")
	if !r.Denied {
		t.Fatal("expected deny")
	}
}

func TestEvaluatePlanGitOpsStatusLow(t *testing.T) {
	r := EvaluatePlan(planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindGitOps},
		Actions: []planner.Action{{
			Op:      planner.OpGitOpsStatus,
			Backend: "gitops",
		}},
	})
	if r.Denied || r.Risk != RiskLow {
		t.Fatalf("%+v", r)
	}
}

func TestEvaluatePlanGitOpsSyncMedium(t *testing.T) {
	r := EvaluatePlan(planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindGitOps},
		Actions: []planner.Action{{
			Op:      planner.OpGitOpsSync,
			Backend: "gitops",
		}},
		RequiresApproval: true,
	})
	if r.Denied || r.Risk != RiskMedium {
		t.Fatalf("%+v", r)
	}
}
