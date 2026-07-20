package planner

import (
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildKEDA(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind: intent.KindKEDA,
		Target: intent.Target{
			Name:      "api",
			Namespace: "default",
			Kind:      "ScaledObject",
		},
		Params: map[string]any{
			"trigger":     "redis",
			"minReplicas": 0,
			"maxReplicas": 5,
			"queue":       "jobs",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Op != OpScaledObjectCreate || plan.Actions[0].Backend != "keda" {
		t.Fatalf("%+v", plan)
	}
	if !plan.RequiresApproval {
		t.Fatal("expected approval")
	}
	if !strings.Contains(plan.Actions[0].Manifest, "ScaledObject") || !strings.Contains(plan.Actions[0].Manifest, "keda.sh") {
		t.Fatalf("manifest=%s", plan.Actions[0].Manifest)
	}
	if !strings.Contains(plan.Actions[0].Manifest, "type: redis") {
		t.Fatalf("manifest=%s", plan.Actions[0].Manifest)
	}
}
