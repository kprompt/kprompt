package planner

import (
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildDeployRedis(t *testing.T) {
	in := intent.Intent{
		Kind: intent.KindDeploy,
		Target: intent.Target{
			Name:      "redis",
			Namespace: "demo",
		},
		Params: map[string]any{},
	}
	plan, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 2 {
		t.Fatalf("expected Deployment+Service, got %d actions", len(plan.Actions))
	}
	if plan.Actions[0].Object.Kind != "Deployment" {
		t.Fatalf("first action=%s", plan.Actions[0].Object.Kind)
	}
	if plan.Actions[1].Object.Kind != "Service" {
		t.Fatalf("second action=%s", plan.Actions[1].Object.Kind)
	}
	if plan.Actions[0].Manifest == "" {
		t.Fatal("missing deployment manifest")
	}
	if !plan.RequiresApproval {
		t.Fatal("deploy should require approval")
	}
}

func TestBuildDeployRequiresImage(t *testing.T) {
	in := intent.Intent{
		Kind:   intent.KindDeploy,
		Target: intent.Target{Name: "myapp"},
		Params: map[string]any{},
	}
	_, err := Build(in)
	if err == nil {
		t.Fatal("expected error for unknown app without image")
	}
}

func TestBuildDeployWithExplicitImage(t *testing.T) {
	in := intent.Intent{
		Kind:   intent.KindDeploy,
		Target: intent.Target{Name: "myapp", Namespace: "ns"},
		Params: map[string]any{
			"image":    "ghcr.io/example/app:1",
			"replicas": float64(2),
		},
	}
	plan, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("expected only Deployment without port, got %d", len(plan.Actions))
	}
}

func TestBuildGetPods(t *testing.T) {
	in := intent.Intent{
		Kind: intent.KindGet,
		Target: intent.Target{
			Kind:      "pods",
			Namespace: "demo",
		},
		Params: map[string]any{"minMemory": "2Gi"},
	}
	plan, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval {
		t.Fatal("get must not require approval")
	}
	if plan.Actions[0].Object.Kind != "Pod" {
		t.Fatalf("kind=%s", plan.Actions[0].Object.Kind)
	}
}
