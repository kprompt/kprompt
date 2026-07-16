package planner

import (
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildGetWorkflowRequiresName(t *testing.T) {
	_, err := Build(intent.Intent{
		Kind:   intent.KindGet,
		Target: intent.Target{Kind: "Workflow", Namespace: "ml"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildGetWorkflow(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind: intent.KindGet,
		Target: intent.Target{
			Kind:      "Workflow",
			Name:      "train-yolov11",
			Namespace: "ml",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Actions[0].Object.Kind != "Workflow" {
		t.Fatalf("kind=%s", plan.Actions[0].Object.Kind)
	}
}
