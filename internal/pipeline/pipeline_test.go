package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/history"
	"github.com/kprompt/kprompt/internal/llm"
	toolotel "github.com/kprompt/kprompt/internal/tools/otel"
	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
)

func TestMain(m *testing.M) {
	history.Disable = true
	os.Exit(m.Run())
}

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

func TestPerformanceRunsReadOnlyWithoutKubernetesClient(t *testing.T) {
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Namespace: "prod",
		Prompt:    "why is my api slow",
	}, &out, Deps{
		Provider: llm.PerformanceStub("api", "prod", "15m"),
		Prometheus: performanceQuerierFunc(
			func(context.Context, string, time.Time) (toolprometheus.Result, error) {
				return toolprometheus.Result{
					Type: "vector",
					Series: []toolprometheus.Series{{
						Samples: []toolprometheus.Sample{{Timestamp: 1, Value: "0.5"}},
					}},
				}, nil
			},
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Performance: Deployment/api")) {
		t.Fatalf("output=%s", out.String())
	}
}

func TestTraceRunsReadOnlyWithoutKubernetesClient(t *testing.T) {
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Prompt: "trace payment request",
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"trace","target":{"name":"payment","kind":"Service"},"confidence":1}`,
		)},
		OTel: traceQuerierFunc(func(
			_ context.Context,
			req toolotel.SearchRequest,
		) ([]toolotel.Trace, error) {
			if req.Service != "payment" {
				t.Fatalf("service=%q", req.Service)
			}
			return []toolotel.Trace{{
				TraceID:       "trace-1",
				RootService:   "payment",
				RootOperation: "POST /charge",
				Duration:      120 * time.Millisecond,
				Spans: []toolotel.Span{{
					SpanID:    "root",
					Service:   "payment",
					Operation: "POST /charge",
					Duration:  120 * time.Millisecond,
				}},
			}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Trace: trace-1")) ||
		!bytes.Contains(out.Bytes(), []byte("payment: POST /charge")) {
		t.Fatalf("output=%s", out.String())
	}
}

type traceQuerierFunc func(
	context.Context,
	toolotel.SearchRequest,
) ([]toolotel.Trace, error)

func (f traceQuerierFunc) SearchTraces(
	ctx context.Context,
	req toolotel.SearchRequest,
) ([]toolotel.Trace, error) {
	return f(ctx, req)
}

type performanceQuerierFunc func(
	context.Context,
	string,
	time.Time,
) (toolprometheus.Result, error)

func (f performanceQuerierFunc) Query(
	ctx context.Context,
	query string,
	at time.Time,
) (toolprometheus.Result, error) {
	return f(ctx, query, at)
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
	if !bytes.Contains(out.Bytes(), []byte("replicas: 1 → 5")) {
		t.Fatalf("expected live scale diff, got:\n%s", out.String())
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

func TestJSONOutputScalePlan(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("api", "default", 1))
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Approve:   false,
		Namespace: "default",
		Output:    "json",
		Prompt:    "scale api to 3",
	}, &out, Deps{
		Provider:   llm.ScaleStub("api", "default", 3),
		Client:     client,
		IsTerminal: boolPtr(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out.Bytes()) {
		t.Fatalf("invalid json: %s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte(`"schemaVersion":"1"`)) {
		t.Fatalf("missing schemaVersion: %s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte(`"intent":"scale"`)) {
		t.Fatalf("missing intent: %s", out.String())
	}
	if bytes.Contains(out.Bytes(), []byte("Intent:")) {
		t.Fatal("human plan leaked to stdout in json mode")
	}
}

func TestScopeHeuristicSetsNamespaceFromPrompt(t *testing.T) {
	client := fake.NewSimpleClientset()
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Namespace: "default",
		Prompt:    `list deployments in staging`,
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(`{"kind":"get","target":{"kind":"Deployment"},"confidence":1}`)},
		Client:   client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("-n staging")) && !bytes.Contains(out.Bytes(), []byte("in staging")) {
		// Plan summary should mention staging namespace
		if !bytes.Contains(out.Bytes(), []byte("staging")) {
			t.Fatalf("expected staging in plan output:\n%s", out.String())
		}
	}
}

func TestExplainOOMSuggestsPatchWithApprove(t *testing.T) {
	limit := resource.MustParse("64Mi")
	labels := map[string]string{"app": "api"}
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(1),
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "app",
							Image: "app:1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceMemory: limit},
							},
						}},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api-0", Namespace: "default", Labels: labels},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "app:1"}},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "app",
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137},
					},
				}},
			},
		},
	)
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Approve:   true,
		Namespace: "default",
		Prompt:    "explain why api is crashing",
	}, &out, Deps{
		Provider: llm.ExplainStub("api", "default", "Deployment"),
		Client:   client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("OOMKilled")) {
		t.Fatalf("expected OOM finding:\n%s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Suggested fix")) {
		t.Fatalf("expected suggested fix:\n%s", out.String())
	}
	dep, err := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := dep.Spec.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceMemory]
	if got.Cmp(resource.MustParse("128Mi")) != 0 {
		t.Fatalf("memory limit after patch = %s", got.String())
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

func int32Ptr(v int32) *int32 { return &v }

func boolPtr(v bool) *bool { return &v }
