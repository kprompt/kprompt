package tekton

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGeneratePipelineRun(t *testing.T) {
	manifest, summary, err := GeneratePipelineRun(PipelineRequest{
		Name: "ci-demo", Namespace: "default", Repo: "https://github.com/example/app.git", Task: "ci",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(manifest, "kind: PipelineRun") || !strings.Contains(manifest, "tekton.dev/v1") {
		t.Fatalf("manifest=%s", manifest)
	}
	if !strings.Contains(manifest, "git clone") || !strings.Contains(summary, "example/app") {
		t.Fatalf("summary=%s manifest=%s", summary, manifest)
	}
}

func TestDefaultPipelineRunName(t *testing.T) {
	if got := DefaultPipelineRunName("ci", "https://github.com/acme/widget.git"); got != "ci-widget" {
		t.Fatalf("got=%s", got)
	}
}

func TestStatusFromObjectSucceeded(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "run1", "namespace": "ns"},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Succeeded", "status": "True", "message": "done"},
			},
		},
	}}
	st := StatusFromObject(obj)
	if st.Phase != "Succeeded" || st.Name != "run1" {
		t.Fatalf("%+v", st)
	}
}

func TestInferRepoFromPrompt(t *testing.T) {
	if got := InferRepoFromPrompt("create a CI pipeline for https://github.com/acme/app"); !strings.Contains(got, "github.com/acme/app") {
		t.Fatalf("got=%q", got)
	}
}
