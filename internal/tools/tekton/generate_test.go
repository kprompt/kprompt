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

func TestGeneratePipelineRunQuotesInjectionShapedRepo(t *testing.T) {
	repo := "https://github.com/acme/app$(touch /tmp/pwn).git"
	manifest, _, err := GeneratePipelineRun(PipelineRequest{
		Name: "ci-app", Namespace: "default", Repo: repo, Task: "ci",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(manifest, `echo "repo=`) {
		t.Fatalf("repo echo should be single-quoted in script:\n%s", manifest)
	}
	if !strings.Contains(manifest, "echo 'repo=https://github.com/acme/app$(touch /tmp/pwn).git'") {
		t.Fatalf("manifest=%s", manifest)
	}
	if !strings.Contains(manifest, "git clone --depth 1 'https://github.com/acme/app$(touch /tmp/pwn).git' /workspace/src") {
		t.Fatalf("manifest=%s", manifest)
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{name: "empty", input: "", want: "''"},
		{name: "spaces", input: "hello world", want: "'hello world'"},
		{name: "single quote", input: "it's", want: `'it'"'"'s'`},
		{name: "metacharacters", input: "$(touch /tmp/pwned); *", want: "'$(touch /tmp/pwned); *'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shellQuote(tt.input); got != tt.want {
				t.Fatalf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
