package planner

import (
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildGraphCluster(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindGraph,
		Target: intent.Target{Kind: "ServiceGraph"},
		Params: map[string]any{"scope": "cluster"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Op != OpGraph || plan.RequiresApproval {
		t.Fatalf("%+v", plan)
	}
	if plan.Actions[0].Object.Namespace != "" {
		t.Fatalf("ns=%q", plan.Actions[0].Object.Namespace)
	}
}

func TestBuildGraphNamespace(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindGraph,
		Target: intent.Target{Namespace: "prod", Kind: "ServiceGraph"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Actions[0].Object.Namespace != "prod" {
		t.Fatalf("%+v", plan)
	}
}
