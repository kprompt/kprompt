package argo

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestStatusFromObject(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      "train-yolov11",
			"namespace": "ml",
		},
		"status": map[string]any{
			"phase":     "Running",
			"message":   "workflow running",
			"startedAt": "2026-01-01T00:00:00Z",
		},
	}}
	st := StatusFromObject(obj)
	if st.Name != "train-yolov11" || st.Namespace != "ml" || st.Phase != "Running" {
		t.Fatalf("status=%+v", st)
	}
	if st.Label() == "" {
		t.Fatal("expected label")
	}
}

func TestIsTerminalPhase(t *testing.T) {
	if !IsTerminalPhase("Succeeded") || IsTerminalPhase("Running") {
		t.Fatal("terminal phase mismatch")
	}
}

func TestSubmitWorkflow(t *testing.T) {
	manifest := `apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  name: train-yolov11
  namespace: ml
spec:
  entrypoint: train
  templates:
  - name: train
    container:
      image: busybox
      command: [sh, -c]
      args: ["echo hi"]
`
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	st, err := SubmitWithClient(context.Background(), dc, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if st.Name != "train-yolov11" || st.Namespace != "ml" {
		t.Fatalf("status=%+v", st)
	}
}

func TestGetStatusWorkflow(t *testing.T) {
	obj := workflowObject("demo", "default", "Succeeded")
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)

	st, err := GetStatusWithClient(context.Background(), dc, "default", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if st.Phase != "Succeeded" {
		t.Fatalf("phase=%s", st.Phase)
	}
}

func TestDecodeWorkflowManifestRejectsWrongKind(t *testing.T) {
	_, _, _, err := decodeWorkflowManifest(`apiVersion: v1
kind: Pod
metadata:
  name: x
`)
	if err == nil || !strings.Contains(err.Error(), "Workflow") {
		t.Fatalf("err=%v", err)
	}
}

func TestWaitWithClientSucceeded(t *testing.T) {
	obj := workflowObject("demo", "default", "Succeeded")
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
	st, err := WaitWithClient(context.Background(), dc, "default", "demo", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.Phase != "Succeeded" {
		t.Fatalf("phase=%s", st.Phase)
	}
}

func workflowObject(name, ns, phase string) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Workflow",
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
		},
		"spec": map[string]any{
			"entrypoint": "main",
		},
	}
	if phase != "" {
		obj["status"] = map[string]any{"phase": phase}
	}
	return &unstructured.Unstructured{Object: obj}
}
