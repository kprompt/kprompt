package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
)

func TestMutationWithoutApproveNonInteractiveSkips(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("api", "default", 1))
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Approve:   false,
		Namespace: "default",
		Prompt:    "scale api to 3",
	}, &out, Deps{
		Provider: llm.ScaleStub("api", "default", 3),
		Client:   client,
		// Non-interactive: no Confirm, force non-TTY via Confirm unset and StdinIsTerminal false in CI.
		// Inject Confirm=nil and IsTerminal=false.
		IsTerminal: boolPtr(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	dep, err := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 1 {
		t.Fatalf("should not apply without approval, replicas=%v", dep.Spec.Replicas)
	}
	if !bytes.Contains(out.Bytes(), []byte("--approve")) {
		t.Fatalf("expected --approve hint, got %s", out.String())
	}
}

func TestMutationInteractiveYesApplies(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("api", "default", 1))
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Approve:   false,
		Namespace: "default",
		Prompt:    "scale api to 5",
	}, &out, Deps{
		Provider: llm.ScaleStub("api", "default", 5),
		Client:   client,
		Confirm: func(w io.Writer) (bool, error) {
			fmt.Fprintln(w, "(test confirm yes)")
			return true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	dep, err := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 5 {
		t.Fatalf("expected 5 replicas, got %v", dep.Spec.Replicas)
	}
}

func TestMutationInteractiveNoAborts(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("api", "default", 2))
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Approve: false,
		Prompt:  "scale api to 9",
	}, &out, Deps{
		Provider: llm.ScaleStub("api", "default", 9),
		Client:   client,
		Confirm:  func(io.Writer) (bool, error) { return false, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	dep, _ := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if *dep.Spec.Replicas != 2 {
		t.Fatalf("aborted apply should keep replicas=2, got %v", *dep.Spec.Replicas)
	}
	if !bytes.Contains(out.Bytes(), []byte("Aborted")) {
		t.Fatalf("expected Aborted, got %s", out.String())
	}
}

func TestMutationApproveFlagSkipsPrompt(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("api", "default", 1))
	called := false
	err := RunWith(context.Background(), config.Resolved{
		Approve: true,
		Prompt:  "scale api to 4",
	}, io.Discard, Deps{
		Provider: llm.ScaleStub("api", "default", 4),
		Client:   client,
		Confirm: func(io.Writer) (bool, error) {
			called = true
			return false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("--approve should not call Confirm")
	}
	dep, _ := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if *dep.Spec.Replicas != 4 {
		t.Fatalf("replicas=%v", *dep.Spec.Replicas)
	}
}

func deployment(name, ns string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}
}

func boolPtr(v bool) *bool { return &v }
