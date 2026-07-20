package planner

import (
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildIstio(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind: intent.KindIstio,
		Target: intent.Target{
			Name:      "payments",
			Namespace: "prod",
			Kind:      "VirtualService",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Op != OpIstioTraffic || plan.Actions[0].Backend != "istio" {
		t.Fatalf("%+v", plan)
	}
	if plan.RequiresApproval {
		t.Fatal("istio traffic is read-only")
	}
}
