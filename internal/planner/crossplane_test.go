package planner

import (
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildCrossplane(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind: intent.KindCrossplane,
		Target: intent.Target{
			Name:      "app-db",
			Namespace: "default",
			Kind:      "PostgreSQLInstance",
		},
		Params: map[string]any{
			"resource":  "postgres",
			"provider":  "aws",
			"storageGB": 20,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Op != OpClaimCreate || plan.Actions[0].Backend != "crossplane" {
		t.Fatalf("%+v", plan)
	}
	if !plan.RequiresApproval {
		t.Fatal("expected approval")
	}
	if !strings.Contains(plan.Actions[0].Manifest, "PostgreSQLInstance") {
		t.Fatalf("manifest=%s", plan.Actions[0].Manifest)
	}
}
