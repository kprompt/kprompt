package planner

import (
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildWorkflowTrainYOLOv11(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind: intent.KindWorkflow,
		Target: intent.Target{
			Name:      "train-yolov11",
			Namespace: "ml",
		},
		Params: map[string]any{
			"task":  "train",
			"model": "yolov11",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("actions=%d", len(plan.Actions))
	}
	a := plan.Actions[0]
	if a.Op != OpWorkflowCreate || a.Backend != "argo" {
		t.Fatalf("op=%s backend=%s", a.Op, a.Backend)
	}
	if !strings.Contains(a.Manifest, "kind: Workflow") {
		t.Fatalf("manifest=%s", a.Manifest)
	}
	if !plan.RequiresApproval {
		t.Fatal("workflow should require approval")
	}
}
