package mcp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/history"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/pipeline"
)

func TestMain(m *testing.M) {
	history.Disable = true
	os.Exit(m.Run())
}

// callToolWithDeps runs a single tools/call through a server seeded with the
// given pipeline deps (stub LLM + fake client) and returns the text content.
func callToolWithDeps(t *testing.T, deps pipeline.Deps, name string, args map[string]any) (string, bool) {
	t.Helper()
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]any{"name": name, "arguments": args},
	}
	line, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var out capturingWriter
	srv := newServerWithDeps(strings.NewReader(string(line)+"\n"), &out, "test", deps)
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resp response
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.b.String())), &resp); err != nil {
		t.Fatalf("unmarshal response: %v (raw=%q)", err, out.b.String())
	}
	if resp.Error != nil {
		t.Fatalf("transport error: %+v", resp.Error)
	}
	result, _ := resp.Result.(map[string]any)
	isErr, _ := result["isError"].(bool)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("empty content: %v", result)
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	return text, isErr
}

// End-to-end: kprompt.read compiles a read prompt with a stub LLM against a fake
// cluster and returns a read-only PlanResult — applied, not mutating.
func TestReadToolEndToEndReturnsPlanResult(t *testing.T) {
	t.Setenv("KPROMPT_HOME", t.TempDir())
	deps := pipeline.Deps{
		Provider: &llm.Stub{Structured: []byte(`{"kind":"get","target":{"kind":"Deployment"},"confidence":1}`)},
		Client: fake.NewSimpleClientset(&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		}),
	}
	text, isErr := callToolWithDeps(t, deps, "kprompt.read", map[string]any{
		"prompt":    "list deployments",
		"namespace": "default",
	})
	if isErr {
		t.Fatalf("read tool reported error: %s", text)
	}
	if !strings.Contains(text, `"kind":"PlanResult"`) {
		t.Fatalf("expected a PlanResult document, got: %s", text)
	}
	if strings.Contains(text, `"requiresApproval":true`) {
		t.Fatalf("read must not require approval: %s", text)
	}
	if !strings.Contains(text, "web") {
		t.Fatalf("expected the fake deployment name in output: %s", text)
	}
}

// End-to-end: a mutation via kprompt.plan returns a PlanResult that requires
// approval and is never applied — the fake cluster stays untouched.
func TestPlanToolEndToEndNeverApplies(t *testing.T) {
	t.Setenv("KPROMPT_HOME", t.TempDir())
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(1)},
	})
	deps := pipeline.Deps{
		Provider: llm.ScaleStub("api", "default", 5),
		Client:   client,
	}
	text, isErr := callToolWithDeps(t, deps, "kprompt.plan", map[string]any{
		"prompt":    "scale api to 5",
		"namespace": "default",
	})
	if isErr {
		t.Fatalf("plan tool reported error: %s", text)
	}
	if !strings.Contains(text, `"requiresApproval":true`) {
		t.Fatalf("expected a mutating PlanResult: %s", text)
	}
	if strings.Contains(text, `"applied":true`) {
		t.Fatalf("plan must never apply: %s", text)
	}
	dep, err := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 1 {
		t.Fatalf("cluster was mutated over MCP: replicas=%v", dep.Spec.Replicas)
	}
}

func TestSRETools(t *testing.T) {
	tests := []struct {
		sreTool string
	}{
		{sreTool: "kprompt.investigate"},
		{sreTool: "kprompt.why"},
		{sreTool: "kprompt.impact"},
		{sreTool: "kprompt.timeline"},
	}
	for _, tc := range tests {
		t.Run(tc.sreTool, func(t *testing.T) {
			t.Setenv("KPROMPT_HOME", t.TempDir())
			ns := "payments"
			labels := map[string]string{"app": "api"}

			client := fake.NewSimpleClientset(
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: ns},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{MatchLabels: labels},
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: ns, Labels: labels},
				},
			)
			deps := pipeline.Deps{
				Provider: &llm.Stub{Structured: []byte(`{"kind":"get",
				"target":{"kind":"Deployment", "Name":"api", "Namespace":"payments"},"confidence":1}`)},
				Client: client,
			}
			text, isErr := callToolWithDeps(t, deps, tc.sreTool, map[string]any{
				"prompt": "why is api crashlooping",
				"target": "deployment",
				"name":   "api",
			})
			if isErr {
				t.Fatalf("%s tool reported error: %s", tc.sreTool, text)
			}
			if !strings.Contains(text, `"kind":"PlanResult"`) {
				t.Fatalf("for %s tool, expected a PlanResult document, got: %s", tc.sreTool, text)
			}
			if strings.Contains(text, `"requiresApproval":true`) {
				t.Fatalf("%s tool must not require approval: %s", tc.sreTool, text)
			}
			if !strings.Contains(text, "api") {
				t.Fatalf("for %s tool, expected the fake deployment name in output: %s", tc.sreTool, text)
			}
		})
	}
}

func TestSREToolsMissingTarget(t *testing.T) {
	tests := []struct {
		sreTool string
	}{
		{sreTool: "kprompt.investigate"},
		{sreTool: "kprompt.why"},
		{sreTool: "kprompt.impact"},
		{sreTool: "kprompt.timeline"},
	}
	for _, tc := range tests {
		t.Run(tc.sreTool, func(t *testing.T) {
			t.Setenv("KPROMPT_HOME", t.TempDir())
			ns := "payments"
			labels := map[string]string{"app": "api"}
			client := fake.NewSimpleClientset(
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: ns},
					Spec: appsv1.DeploymentSpec{
						Selector: &metav1.LabelSelector{MatchLabels: labels},
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "api-1", Namespace: ns, Labels: labels},
				},
			)
			deps := pipeline.Deps{
				Provider: &llm.Stub{Structured: []byte(`{"kind":"get",
				"target":{"kind":"Deployment", "Name":"api", "Namespace":"payments"},"confidence":1}`)},
				Client: client,
			}
			// Missing target
			text, isErr := callToolWithDeps(t, deps, tc.sreTool, map[string]any{
				"prompt": "why is api crashlooping",
				"name":   "api",
			})
			if !isErr {
				t.Fatalf("for %s tool, expected error: %s", tc.sreTool, text)
			}
			if !strings.Contains(text, `target is required`) {
				t.Fatalf("for %s tool, expected error of target is required: %s", "kprompt.why", text)
			}
		})
	}
}

func int32Ptr(i int32) *int32 { return &i }
