package planner

import (
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildOptimizeCluster(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindOptimize,
		Target: intent.Target{Kind: "Cluster"},
		Params: map[string]any{"scope": "cluster"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval {
		t.Fatal("optimize must be read-only")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Op != OpOptimize {
		t.Fatalf("actions=%v", plan.Actions)
	}
	if plan.Actions[0].Object.Namespace != "" {
		t.Fatalf("cluster scope ns=%q", plan.Actions[0].Object.Namespace)
	}
}

func TestBuildOptimizeNamespace(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindOptimize,
		Target: intent.Target{Namespace: "prod", Kind: "Cluster"},
		Params: map[string]any{"window": "2h"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Actions[0].Object.Namespace != "prod" {
		t.Fatalf("ns=%q", plan.Actions[0].Object.Namespace)
	}
	if w, _ := plan.Intent.Window(); w != "2h" {
		t.Fatalf("window=%q", w)
	}
}
