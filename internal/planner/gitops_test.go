package planner

import (
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildGitOpsStatus(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind: intent.KindGitOps,
		Params: map[string]any{
			"action": "status",
			"engine": "auto",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Op != OpGitOpsStatus || plan.Actions[0].Backend != "gitops" {
		t.Fatalf("%+v", plan)
	}
	if plan.RequiresApproval {
		t.Fatal("status should not require approval")
	}
}

func TestBuildGitOpsSyncRequiresNameAndEngine(t *testing.T) {
	_, err := Build(intent.Intent{
		Kind: intent.KindGitOps,
		Params: map[string]any{
			"action": "sync",
			"engine": "auto",
		},
		Target: intent.Target{Name: "apps"},
	})
	if err == nil {
		t.Fatal("expected error for engine=auto")
	}

	_, err = Build(intent.Intent{
		Kind: intent.KindGitOps,
		Params: map[string]any{
			"action": "sync",
			"engine": "flux",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing name")
	}

	plan, err := Build(intent.Intent{
		Kind: intent.KindGitOps,
		Target: intent.Target{
			Name:      "apps",
			Namespace: "flux-system",
		},
		Params: map[string]any{
			"action": "sync",
			"engine": "flux",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Op != OpGitOpsSync || !plan.RequiresApproval {
		t.Fatalf("%+v", plan)
	}
	if plan.Actions[0].Object.Kind != "Kustomization" {
		t.Fatalf("%+v", plan.Actions[0].Object)
	}
}
