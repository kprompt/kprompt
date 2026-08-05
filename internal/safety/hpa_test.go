package safety

import (
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
)

func TestCheckHPAPromptAllowsCreate(t *testing.T) {
	got := CheckHPAPrompt("add HPA for redis")
	if got.Denied {
		t.Fatalf("%+v", got)
	}
}

func TestEvaluateHPAPlan(t *testing.T) {
	plan := planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindHPA},
		Actions: []planner.Action{{
			Op: planner.OpCreate,
			Object: planner.ObjectRef{
				Kind: "HorizontalPodAutoscaler",
				Name: "redis-hpa",
			},
		}},
	}
	got := EvaluatePlan(plan)
	if got.Denied || got.Risk != RiskMedium {
		t.Fatalf("%+v", got)
	}
}
