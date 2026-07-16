package executor

import (
	"context"
	"testing"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/planner"
)

func TestIsArgoWorkflowPlan(t *testing.T) {
	plan := planner.ExecutionPlan{
		Actions: []planner.Action{{
			Backend: "argo",
			Op:      planner.OpWorkflowCreate,
		}},
	}
	if !IsArgoWorkflowPlan(plan) {
		t.Fatal("expected argo workflow plan")
	}
	mixed := planner.ExecutionPlan{
		Actions: []planner.Action{
			{Backend: "argo", Op: planner.OpWorkflowCreate},
			{Backend: "kubernetes", Op: planner.OpCreate},
		},
	}
	if IsArgoWorkflowPlan(mixed) {
		t.Fatal("expected mixed plan to be false")
	}
}

func TestWorkflowTargets(t *testing.T) {
	plan := planner.ExecutionPlan{
		Actions: []planner.Action{{
			Backend: "argo",
			Op:      planner.OpWorkflowCreate,
			Object:  planner.ObjectRef{Name: "train-yolov11", Namespace: "ml"},
		}},
	}
	targets := WorkflowTargets(plan)
	if len(targets) != 1 || targets[0].Name != "train-yolov11" {
		t.Fatalf("targets=%+v", targets)
	}
}

func TestApplyArgoRequiresConfig(t *testing.T) {
	_, err := ApplyArgo(context.Background(), nil, planner.ExecutionPlan{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestApplyArgoUnsupportedOp(t *testing.T) {
	_, err := ApplyArgo(context.Background(), &rest.Config{Host: "https://127.0.0.1:1"}, planner.ExecutionPlan{
		Actions: []planner.Action{{Backend: "argo", Op: "workflow-delete"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
