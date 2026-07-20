package planner

import (
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildTekton(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind: intent.KindTekton,
		Target: intent.Target{
			Name:      "ci-app",
			Namespace: "default",
			Kind:      "PipelineRun",
		},
		Params: map[string]any{
			"task":     "ci",
			"repo_url": "https://github.com/acme/app.git",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Op != OpPipelineRunCreate || plan.Actions[0].Backend != "tekton" {
		t.Fatalf("%+v", plan)
	}
	if !plan.RequiresApproval {
		t.Fatal("expected approval")
	}
	if !strings.Contains(plan.Actions[0].Manifest, "PipelineRun") {
		t.Fatalf("manifest=%s", plan.Actions[0].Manifest)
	}
}
