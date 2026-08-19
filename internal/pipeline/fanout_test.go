package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/output"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
)

func TestMultiContextRefusesSilentApprove(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("api", "default", 1))
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Approve:   true,
		Namespace: "default",
		Prompt:    "scale api to 3",
		Contexts:  []string{"ctx-a", "ctx-b"},
		Output:    "json",
	}, &out, Deps{
		Provider: llm.ScaleStub("api", "default", 3),
		Client:   client,
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc output.PlanResult
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if !doc.Risk.Denied || !strings.Contains(doc.Risk.Message, "--approve") {
		t.Fatalf("%+v", doc.Risk)
	}
	dep, _ := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 1 {
		t.Fatal("must not mutate")
	}
}

func TestMultiContextApproveEachContext(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("api", "default", 1))
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		ApproveEachContext: true,
		Namespace:          "default",
		Prompt:             "scale api to 3",
		Contexts:           []string{"ctx-a", "ctx-b"},
		Output:             "json",
	}, &out, Deps{
		Provider: llm.ScaleStub("api", "default", 3),
		Client:   client,
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc output.MultiContextResult
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(doc.Steps) != 2 {
		t.Fatalf("steps=%d", len(doc.Steps))
	}
	for _, step := range doc.Steps {
		if !step.Applied {
			t.Fatalf("step not applied: %+v", step)
		}
		if step.ClusterContext == "" {
			t.Fatal("missing cluster_context")
		}
		if len(step.Plan.Actions) == 0 || step.Plan.Actions[0].ClusterContext == "" {
			t.Fatalf("action missing cluster_context: %+v", step.Plan.Actions)
		}
	}
	dep, _ := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 3 {
		t.Fatalf("replicas=%v", dep.Spec.Replicas)
	}
}

func TestMultiContextMutatePerContextConfirm(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("api", "default", 1))
	confirms := 0
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Namespace: "default",
		Prompt:    "scale api to 3",
		Contexts:  []string{"ctx-a", "ctx-b"},
		Output:    "json",
	}, &out, Deps{
		Provider: llm.ScaleStub("api", "default", 3),
		Client:   client,
		Confirm: func(io.Writer) (bool, error) {
			confirms++
			return confirms == 1, nil // approve only first context
		},
		IsTerminal: boolPtr(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if confirms != 2 {
		t.Fatalf("confirms=%d", confirms)
	}
	var doc output.MultiContextResult
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if doc.Applied {
		t.Fatal("expected overall applied=false when one skipped")
	}
	if !doc.Steps[0].Applied || doc.Steps[1].Applied {
		t.Fatalf("steps applied=%v %v", doc.Steps[0].Applied, doc.Steps[1].Applied)
	}
}

func TestMultiContextGetFanOut(t *testing.T) {
	reps := int32(1)
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: &reps},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 1, Replicas: 1},
	})
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Namespace: "default",
		Prompt:    "list deployments",
		Contexts:  []string{"kind-a", "kind-b"},
		Output:    "json",
	}, &out, Deps{
		Provider: llm.GetStub("Deployment", "", "default", ""),
		Client:   client,
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc output.MultiContextResult
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if doc.Kind != output.KindMultiContextResult {
		t.Fatalf("kind=%s", doc.Kind)
	}
	if len(doc.Steps) != 2 {
		t.Fatalf("steps=%d", len(doc.Steps))
	}
	if doc.Steps[0].Plan.Context != "kind-a" || doc.Steps[1].Plan.Context != "kind-b" {
		t.Fatalf("contexts=%q %q", doc.Steps[0].Plan.Context, doc.Steps[1].Plan.Context)
	}
	if !doc.Applied {
		t.Fatal("expected applied")
	}
}

func TestMultiContextOptimizeFanOut(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("api", "default", 2))
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Prompt:   "optimize my cluster",
		Contexts: []string{"ctx-a", "ctx-b"},
		Output:   "json",
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"optimize","target":{"kind":"Cluster"},"params":{"scope":"cluster"},"confidence":1}`,
		)},
		Client: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc output.MultiContextResult
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if doc.FleetSummary == nil {
		t.Fatal("expected fleetSummary")
	}
	if len(doc.FleetSummary.ContextsOK) != 2 {
		t.Fatalf("ok=%v failed=%v", doc.FleetSummary.ContextsOK, doc.FleetSummary.ContextsFailed)
	}
	if doc.FleetSummary.FindingCount < 1 {
		t.Fatalf("findings=%d", doc.FleetSummary.FindingCount)
	}
	for _, f := range doc.FleetSummary.Findings {
		if f.ClusterContext == "" {
			t.Fatalf("finding missing cluster_context: %+v", f)
		}
	}
}

func TestSupportsReadFanOut(t *testing.T) {
	ok := []intent.Kind{
		intent.KindGet, intent.KindExplain, intent.KindInvestigate, intent.KindWhy,
		intent.KindTimeline, intent.KindImpact, intent.KindAudit, intent.KindCleanup,
		intent.KindSearch, intent.KindScore, intent.KindArchitecture, intent.KindLearn, intent.KindDrift, intent.KindLogs, intent.KindDescribe, intent.KindOptimize,
		intent.KindRoast, intent.KindGraph, intent.KindIstio, intent.KindGitOps,
	}
	for _, k := range ok {
		if !supportsReadFanOut(k) {
			t.Fatalf("%s should support fan-out", k)
		}
	}
	if supportsReadFanOut(intent.KindScale) {
		t.Fatal("scale is mutate, not read fan-out")
	}
	for _, k := range []intent.Kind{intent.KindPerformance, intent.KindTrace, intent.KindDashboard} {
		if supportsReadFanOut(k) {
			t.Fatalf("%s reads a configured endpoint, not the context — must not fan out", k)
		}
	}
}

func TestReadFanOutKindListMatchesAllowlist(t *testing.T) {
	list := readFanOutKindList()
	for _, k := range readFanOutKinds {
		if !strings.Contains(list, string(k)) {
			t.Fatalf("deny message %q omits allowlisted kind %s", list, k)
		}
	}
}

func TestMultiContextGraphFanOut(t *testing.T) {
	client := fake.NewSimpleClientset(
		deployment("api", "default", 1),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "api"}},
		},
	)
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Namespace: "default",
		Prompt:    "show service graph",
		Contexts:  []string{"kind-a", "kind-b"},
		Output:    "json",
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"graph","target":{"kind":"ServiceGraph","namespace":"default"},"confidence":1}`,
		)},
		Client: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc output.MultiContextResult
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(doc.Steps) != 2 {
		t.Fatalf("steps=%d\n%s", len(doc.Steps), out.String())
	}
	if !doc.Applied {
		t.Fatalf("expected applied: %+v", doc)
	}
	for i, want := range []string{"kind-a", "kind-b"} {
		step := doc.Steps[i]
		if step.Risk.Denied {
			t.Fatalf("%s denied: %s", want, step.Risk.Message)
		}
		if step.ClusterContext != want || step.Plan.Context != want {
			t.Fatalf("step %d context=%q/%q want %q", i, step.ClusterContext, step.Plan.Context, want)
		}
		var payload struct {
			Type  string `json:"type"`
			Nodes []any  `json:"nodes"`
		}
		if err := json.Unmarshal(step.Result, &payload); err != nil {
			t.Fatalf("step %d result: %v (%s)", i, err, step.Result)
		}
		if payload.Type == "" || len(payload.Nodes) == 0 {
			t.Fatalf("step %d missing graph payload: %s", i, step.Result)
		}
	}
}

func TestMultiContextDeniesEndpointBackedRead(t *testing.T) {
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Namespace: "default",
		Prompt:    "why is api slow",
		Contexts:  []string{"kind-a", "kind-b"},
		Output:    "json",
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"performance","target":{"kind":"Deployment","name":"api","namespace":"default"},"confidence":1}`,
		)},
		Client: fake.NewSimpleClientset(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var doc output.PlanResult
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !doc.Risk.Denied {
		t.Fatalf("expected deny, got %+v", doc.Risk)
	}
	if !strings.Contains(doc.Risk.Message, "performance") || !strings.Contains(doc.Risk.Message, "graph") {
		t.Fatalf("deny message should name the rejected kind and the allowlist: %q", doc.Risk.Message)
	}
}

// TestMultiContextIstioAndGitOpsReadsAllowed exercises the fan-out dispatch
// layer directly (no real cluster needed): a read-only istio/gitops plan
// must clear the supportsReadFanOut gate and reach fanOutSteps instead of
// being denied as an unsupported kind (issue #159).
func TestMultiContextIstioAndGitOpsReadsAllowed(t *testing.T) {
	for _, in := range []intent.Intent{
		{Kind: intent.KindIstio, Target: intent.Target{Kind: "VirtualService", Namespace: "default"}},
		{Kind: intent.KindGitOps, Params: map[string]any{"action": "status"}, Target: intent.Target{Namespace: "default"}},
	} {
		plan, err := planner.Build(in)
		if err != nil {
			t.Fatalf("%s: build plan: %v", in.Kind, err)
		}
		if plan.RequiresApproval {
			t.Fatalf("%s: expected a read-only status/health plan", in.Kind)
		}
		if !isReadOnly(plan) {
			t.Fatalf("%s: expected isReadOnly", in.Kind)
		}
		if !supportsReadFanOut(plan.Intent.Kind) {
			t.Fatalf("%s: expected supportsReadFanOut after #159", in.Kind)
		}

		var out bytes.Buffer
		contexts := []string{"ctx-a", "ctx-b"}
		err = runMultiContextReads(context.Background(), config.Resolved{
			Prompt:   "n/a",
			Contexts: contexts,
			Output:   "json",
		}, &out, Deps{}, nil, plan, safety.Result{}, contexts)
		if err != nil {
			t.Fatalf("%s: %v", in.Kind, err)
		}

		var doc output.MultiContextResult
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatalf("%s: expected a MultiContextResult (not a single-kind denial): %v\n%s", in.Kind, err, out.String())
		}
		if doc.Kind != output.KindMultiContextResult {
			t.Fatalf("%s: kind=%s", in.Kind, doc.Kind)
		}
		if len(doc.Steps) != len(contexts) {
			t.Fatalf("%s: steps=%d", in.Kind, len(doc.Steps))
		}
		for _, step := range doc.Steps {
			if strings.Contains(step.Risk.Message, "multi-context fan-out supports") {
				t.Fatalf("%s: still denied as unsupported kind: %q", in.Kind, step.Risk.Message)
			}
		}
	}
}

// TestMultiContextGitOpsMutateRefusesSilentApprove asserts that a gitops
// sync/promote/rollback plan (RequiresApproval=true) never silently fans out
// under plain --approve, exactly like every other mutating kind (issue #159).
func TestMultiContextGitOpsMutateRefusesSilentApprove(t *testing.T) {
	plan, err := planner.Build(intent.Intent{
		Kind:   intent.KindGitOps,
		Params: map[string]any{"action": "sync", "engine": "argocd"},
		Target: intent.Target{Kind: "Application", Name: "payments", Namespace: "default"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresApproval {
		t.Fatal("expected gitops sync to require approval")
	}
	if isReadOnly(plan) {
		t.Fatal("gitops sync must not be treated as read-only")
	}

	var out bytes.Buffer
	err = runMultiContextFanOut(context.Background(), config.Resolved{
		Approve:  true,
		Prompt:   "sync argocd application payments",
		Contexts: []string{"ctx-a", "ctx-b"},
		Output:   "json",
	}, &out, Deps{}, nil, plan, safety.Result{}, []string{"ctx-a", "ctx-b"})
	if err != nil {
		t.Fatal(err)
	}
	var doc output.PlanResult
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !doc.Risk.Denied || !strings.Contains(doc.Risk.Message, "--approve") {
		t.Fatalf("expected silent-approve refusal, got %+v", doc.Risk)
	}
}
