package argo

import (
	"strings"
	"testing"
)

func TestGenerateWorkflowYOLOv11(t *testing.T) {
	manifest, summary, err := GenerateWorkflow(WorkflowRequest{
		Name:      "train-yolov11",
		Namespace: "ml",
		Task:      "train",
		Model:     "yolov11",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"apiVersion: argoproj.io/v1alpha1",
		"kind: Workflow",
		"name: train-yolov11",
		"namespace: ml",
		"ultralytics/ultralytics:latest",
		"model=yolo11n.pt",
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifest)
		}
	}
	if !strings.Contains(summary, "train-yolov11") {
		t.Fatalf("summary=%q", summary)
	}
}

func TestInferModelFromPrompt(t *testing.T) {
	if got := InferModelFromPrompt("train a yolov11 model"); got != "yolov11" {
		t.Fatalf("got=%q", got)
	}
}

func TestGenerateWorkflowRequiresImageOrModel(t *testing.T) {
	_, _, err := GenerateWorkflow(WorkflowRequest{Name: "demo"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGenerateWorkflowRejectsShellLauncherCommand(t *testing.T) {
	_, _, err := GenerateWorkflow(WorkflowRequest{
		Name:    "train",
		Image:   "python:3.11-slim",
		Command: []string{"/bin/sh", "-c"},
		Args:    []string{"echo hi"},
	})
	if err == nil || !strings.Contains(err.Error(), "may not use shell launcher") {
		t.Fatalf("err=%v", err)
	}
}

func TestGenerateWorkflowUnknownModelDoesNotUseShell(t *testing.T) {
	manifest, _, err := GenerateWorkflow(WorkflowRequest{
		Name:  "train-custom",
		Model: "custom-model-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(manifest, "/bin/sh") || strings.Contains(manifest, " -c") {
		t.Fatalf("manifest should avoid shell launcher:\n%s", manifest)
	}
	if !strings.Contains(manifest, "command:") || !strings.Contains(manifest, "- echo") {
		t.Fatalf("manifest=%s", manifest)
	}
}
