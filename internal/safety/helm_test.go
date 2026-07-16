package safety

import (
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
)

func TestCheckHelmPromptDeniesWipeUninstall(t *testing.T) {
	cases := []string{
		`helm uninstall --all`,
		`uninstall all helm releases`,
		`purge all releases in the cluster`,
	}
	for _, c := range cases {
		r := CheckHelmPrompt(c)
		if !r.Denied {
			t.Fatalf("expected deny for %q", c)
		}
	}
}

func TestCheckHelmPromptAllowsNamedUninstallPhrase(t *testing.T) {
	r := CheckHelmPrompt(`helm uninstall redis`)
	if r.Denied {
		t.Fatal("named helm uninstall phrase should not be hard-denied yet")
	}
}

func TestEvaluateHelmPlanDeniesUninstallAllCommand(t *testing.T) {
	r := EvaluatePlan(planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindInstall},
		Actions: []planner.Action{{
			Backend: "helm",
			Command: []string{"helm", "uninstall", "redis", "--all"},
		}},
	})
	if !r.Denied {
		t.Fatal("expected deny for helm uninstall --all in plan")
	}
}
