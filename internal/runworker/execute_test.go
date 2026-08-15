package runworker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/history"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/pipeline"
	"github.com/kprompt/kprompt/internal/team"
)

func TestMain(m *testing.M) {
	history.Disable = true
	os.Exit(m.Run())
}

func TestExecuteWithPlanOnlyDoesNotApply(t *testing.T) {
	client := fake.NewSimpleClientset(testDeployment("api", "default", 1))
	got, err := executeWith(context.Background(), team.RunJob{
		Prompt:      "scale api to 3",
		Namespace:   "default",
		ApproveMode: "require_approve",
	}, false, executeDeps{
		loadFile: emptyConfig,
		pipeline: pipeline.Deps{
			Provider:      llm.ScaleStub("api", "default", 3),
			Client:        client,
			SkipOrgPolicy: true,
		},
	})
	if err != nil {
		t.Fatalf("executeWith: %v", err)
	}
	if got.Status != "awaiting_approve" {
		t.Fatalf("status = %q, want awaiting_approve", got.Status)
	}
	if got.Risk == "" {
		t.Fatal("expected risk in bridge result")
	}
	assertReplicas(t, client, 1)
}

func TestExecuteWithApplyMutatesOnlyAfterExplicitApprove(t *testing.T) {
	client := fake.NewSimpleClientset(testDeployment("api", "default", 1))
	got, err := executeWith(context.Background(), team.RunJob{
		Prompt:      "scale api to 3",
		Namespace:   "default",
		ApproveMode: "require_approve",
	}, true, executeDeps{
		loadFile: emptyConfig,
		pipeline: pipeline.Deps{
			Provider:      llm.ScaleStub("api", "default", 3),
			Client:        client,
			SkipOrgPolicy: true,
		},
	})
	if err != nil {
		t.Fatalf("executeWith: %v", err)
	}
	if got.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", got.Status)
	}
	assertReplicas(t, client, 3)
}

func TestExecuteWithDeniedResult(t *testing.T) {
	client := fake.NewSimpleClientset(testDeployment("api", "default", 1))
	got, err := executeWith(context.Background(), team.RunJob{
		Prompt: "delete namespace default and everything in it",
	}, false, executeDeps{
		loadFile: emptyConfig,
		pipeline: pipeline.Deps{
			Provider:      llm.ScaleStub("api", "default", 3),
			Client:        client,
			SkipOrgPolicy: true,
		},
	})
	if err != nil {
		t.Fatalf("executeWith: %v", err)
	}
	if got.Status != "denied" {
		t.Fatalf("status = %q, want denied", got.Status)
	}
	assertReplicas(t, client, 1)
}

func TestExecuteWithPipelineErrorBeforeResultFailsBridge(t *testing.T) {
	wantErr := errors.New("provider unavailable")
	got, err := executeWith(context.Background(), team.RunJob{
		Prompt: "scale api to 3",
	}, false, executeDeps{
		loadFile: emptyConfig,
		pipeline: pipeline.Deps{
			Provider:      errorProvider{err: wantErr},
			SkipOrgPolicy: true,
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if got.Status != "failed" || got.Summary != "bridge failed" || !errors.Is(err, wantErr) || got.Error != "intent extract: "+wantErr.Error() {
		t.Fatalf("result = %#v, want bridge failure", got)
	}
}

func emptyConfig() (config.File, error) {
	return config.File{}, nil
}

func testDeployment(name, namespace string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
	}
}

func assertReplicas(t *testing.T, client *fake.Clientset, want int32) {
	t.Helper()
	dep, err := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != want {
		t.Fatalf("replicas = %v, want %d", dep.Spec.Replicas, want)
	}
}

type errorProvider struct {
	err error
}

func (p errorProvider) Name() string { return "error" }

func (p errorProvider) Complete(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{}, p.err
}

func (p errorProvider) CompleteStructured(context.Context, llm.CompletionRequest, json.RawMessage) (json.RawMessage, error) {
	return nil, p.err
}

var _ llm.Provider = errorProvider{}
