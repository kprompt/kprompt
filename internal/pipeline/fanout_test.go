package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/output"
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
