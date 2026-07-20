package safety

import (
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
)

func TestCheckCrossplanePromptDeniesWipe(t *testing.T) {
	r := CheckCrossplanePrompt("delete all crossplane claims")
	if !r.Denied {
		t.Fatal("expected deny")
	}
}

func TestCheckCrossplanePromptAllowsProvision(t *testing.T) {
	r := CheckCrossplanePrompt("provision a postgres database")
	if r.Denied {
		t.Fatal("should allow")
	}
}

func TestEvaluatePlanCrossplaneIsHighRisk(t *testing.T) {
	r := EvaluatePlan(planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindCrossplane},
		Actions: []planner.Action{{
			Op:      planner.OpClaimCreate,
			Backend: "crossplane",
		}},
		RequiresApproval: true,
	})
	if r.Denied || r.Risk != RiskHigh {
		t.Fatalf("%+v", r)
	}
}
