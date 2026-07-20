package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/history"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/output"
	toolgrafana "github.com/kprompt/kprompt/internal/tools/grafana"
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
	start := time.Unix(1, 0)
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
				StartTime:     start,
				Duration:      4 * time.Second,
				Spans: []toolotel.Span{
					{
						SpanID:    "root",
						Service:   "payment",
						Operation: "POST /charge",
						StartTime: start,
						Duration:  4 * time.Second,
					},
					{
						SpanID:       "db",
						ParentSpanID: "root",
						Service:      "postgres",
						Operation:    "SELECT users",
						StartTime:    start.Add(200 * time.Millisecond),
						Duration:     3200 * time.Millisecond,
					},
				},
			}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Trace: trace-1")) ||
		!bytes.Contains(out.Bytes(), []byte("payment: POST /charge")) ||
		!bytes.Contains(out.Bytes(), []byte("Bottlenecks:")) ||
		!bytes.Contains(out.Bytes(), []byte("postgres")) {
		t.Fatalf("output=%s", out.String())
	}
}

func TestDashboardRunsReadOnlyWithoutKubernetesClient(t *testing.T) {
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Prompt: "show payments dashboard",
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"dashboard","target":{"name":"payments","kind":"Dashboard"},"confidence":1}`,
		)},
		Grafana: &grafanaQuerierStub{
			dashboards: []toolgrafana.DashboardSummary{{
				UID:   "payments",
				Title: "Payments Overview",
			}},
			dashboard: toolgrafana.Dashboard{
				UID:   "payments",
				Title: "Payments Overview",
				URL:   "https://grafana.example/d/payments",
				Panels: []toolgrafana.Panel{{
					ID:    1,
					Title: "Request rate",
					Type:  "timeseries",
					Datasource: toolgrafana.Datasource{
						UID: "prom-main",
					},
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Dashboard: Payments Overview")) ||
		!bytes.Contains(out.Bytes(), []byte("https://grafana.example/d/payments")) ||
		!bytes.Contains(out.Bytes(), []byte("Request rate")) {
		t.Fatalf("output=%s", out.String())
	}
}

func TestOptimizeRunsReadOnlyInventory(t *testing.T) {
	replicas := int32(2)
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "api",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
						}},
					},
				},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 2},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "api-hpa", Namespace: "default"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind: "Deployment",
					Name: "api",
				},
				MaxReplicas: 2,
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{
				CurrentReplicas: 2,
				DesiredReplicas: 2,
			},
		},
	)
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Prompt: "optimize my cluster",
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"optimize","target":{"kind":"Cluster"},"params":{"scope":"cluster"},"confidence":1}`,
		)},
		Client: client,
		Prometheus: performanceQuerierFunc(
			func(_ context.Context, promQL string, _ time.Time) (toolprometheus.Result, error) {
				switch {
				case strings.Contains(promQL, "container_cpu_usage_seconds_total"):
					return toolprometheus.Result{
						Type: "vector",
						Series: []toolprometheus.Series{{
							Samples: []toolprometheus.Sample{{Timestamp: 1, Value: "0.01"}},
						}},
					}, nil
				case strings.Contains(promQL, `resource="cpu"`) && strings.Contains(promQL, "requests"):
					return toolprometheus.Result{
						Type: "vector",
						Series: []toolprometheus.Series{{
							Samples: []toolprometheus.Sample{{Timestamp: 1, Value: "1"}},
						}},
					}, nil
				case strings.Contains(promQL, `resource="cpu"`) && strings.Contains(promQL, "limits"):
					return toolprometheus.Result{
						Type: "vector",
						Series: []toolprometheus.Series{{
							Samples: []toolprometheus.Sample{{Timestamp: 1, Value: "2"}},
						}},
					}, nil
				case strings.Contains(promQL, "quantile_over_time") || strings.Contains(promQL, "container_memory_working_set_bytes"):
					return toolprometheus.Result{
						Type: "vector",
						Series: []toolprometheus.Series{{
							Samples: []toolprometheus.Sample{{Timestamp: 1, Value: "10000000"}},
						}},
					}, nil
				case strings.Contains(promQL, `resource="memory"`) && strings.Contains(promQL, "requests"):
					return toolprometheus.Result{
						Type: "vector",
						Series: []toolprometheus.Series{{
							Samples: []toolprometheus.Sample{{Timestamp: 1, Value: "1000000000"}},
						}},
					}, nil
				case strings.Contains(promQL, `resource="memory"`) && strings.Contains(promQL, "limits"):
					return toolprometheus.Result{
						Type: "vector",
						Series: []toolprometheus.Series{{
							Samples: []toolprometheus.Sample{{Timestamp: 1, Value: "2000000000"}},
						}},
					}, nil
				default:
					return toolprometheus.Result{}, nil
				}
			},
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Optimize:")) ||
		!bytes.Contains(out.Bytes(), []byte("Inventory:")) ||
		!bytes.Contains(out.Bytes(), []byte("api")) ||
		!bytes.Contains(out.Bytes(), []byte("Idle:")) ||
		!bytes.Contains(out.Bytes(), []byte("CPU of request")) ||
		!bytes.Contains(out.Bytes(), []byte("Rightsizing:")) ||
		!bytes.Contains(out.Bytes(), []byte("HPA:")) ||
		!bytes.Contains(out.Bytes(), []byte("at max")) ||
		!bytes.Contains(out.Bytes(), []byte("Optional fix")) {
		t.Fatalf("output=%s", out.String())
	}
	if bytes.Contains(out.Bytes(), []byte("inventory: pending")) {
		t.Fatalf("inventory should be ready, output=%s", out.String())
	}
}

func TestOptimizeApproveFlagDoesNotAutoApplyFix(t *testing.T) {
	replicas := int32(2)
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "api",
						Image: "api:1",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					}},
				},
			},
		},
	})
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Prompt:  "optimize my cluster",
		Approve: true, // must NOT auto-apply optimize follow-up patches
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"optimize","target":{"kind":"Cluster"},"params":{"scope":"cluster"},"confidence":1}`,
		)},
		Client:     client,
		IsTerminal: boolPtr(false),
		Prometheus: performanceQuerierFunc(
			func(_ context.Context, promQL string, _ time.Time) (toolprometheus.Result, error) {
				switch {
				case strings.Contains(promQL, "container_cpu_usage_seconds_total"):
					return toolprometheus.Result{
						Type: "vector",
						Series: []toolprometheus.Series{{
							Samples: []toolprometheus.Sample{{Timestamp: 1, Value: "0.01"}},
						}},
					}, nil
				case strings.Contains(promQL, `resource="cpu"`) && strings.Contains(promQL, "requests"):
					return toolprometheus.Result{
						Type: "vector",
						Series: []toolprometheus.Series{{
							Samples: []toolprometheus.Sample{{Timestamp: 1, Value: "1"}},
						}},
					}, nil
				case strings.Contains(promQL, "quantile_over_time") || strings.Contains(promQL, "container_memory_working_set_bytes"):
					return toolprometheus.Result{
						Type: "vector",
						Series: []toolprometheus.Series{{
							Samples: []toolprometheus.Sample{{Timestamp: 1, Value: "10000000"}},
						}},
					}, nil
				case strings.Contains(promQL, `resource="memory"`) && strings.Contains(promQL, "requests"):
					return toolprometheus.Result{
						Type: "vector",
						Series: []toolprometheus.Series{{
							Samples: []toolprometheus.Sample{{Timestamp: 1, Value: "1000000000"}},
						}},
					}, nil
				default:
					return toolprometheus.Result{}, nil
				}
			},
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	dep, err := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	mem := dep.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceMemory]
	if mem.Cmp(resource.MustParse("1Gi")) != 0 {
		t.Fatalf("optimize --approve must not patch resources, got %s", mem.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("does not auto-apply")) {
		t.Fatalf("expected no-auto-apply notice, got %s", out.String())
	}
}

func TestGraphRunsReadOnly(t *testing.T) {
	port := int32(8080)
	client := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "api"}},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "db"}},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-xyz",
				Namespace: "default",
				Labels:    map[string]string{discoveryv1.LabelServiceName: "api"},
			},
			Ports: []discoveryv1.EndpointPort{{Port: &port}},
			Endpoints: []discoveryv1.Endpoint{{
				TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "api-0", Namespace: "default"},
			}},
		},
	)
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Prompt: "show service dependency graph",
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"graph","target":{"kind":"ServiceGraph"},"params":{"scope":"cluster","includeNetworkPolicy":false},"confidence":1}`,
		)},
		Client: client,
		OTel: traceQuerierFunc(func(
			_ context.Context,
			req toolotel.SearchRequest,
		) ([]toolotel.Trace, error) {
			if req.Service != "api" {
				return nil, nil
			}
			return []toolotel.Trace{{
				TraceID: "abc",
				Spans: []toolotel.Span{
					{SpanID: "1", Service: "api", Operation: "GET /"},
					{SpanID: "2", ParentSpanID: "1", Service: "db", Operation: "query"},
				},
			}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Service graph:")) ||
		!bytes.Contains(out.Bytes(), []byte("Edges:")) ||
		!bytes.Contains(out.Bytes(), []byte("routes")) ||
		!bytes.Contains(out.Bytes(), []byte("otel/calls")) {
		t.Fatalf("output=%s", out.String())
	}
}

func TestDashboardJSONOutput(t *testing.T) {
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Prompt: "show dashboards",
		Output: "json",
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"dashboard","target":{"kind":"Dashboard"},"confidence":1}`,
		)},
		Grafana: &grafanaQuerierStub{
			dashboards: []toolgrafana.DashboardSummary{{
				UID:   "payments",
				Title: "Payments Overview",
				URL:   "https://grafana.example/d/payments",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out.Bytes()) ||
		!bytes.Contains(out.Bytes(), []byte(`"type":"dashboard"`)) ||
		!bytes.Contains(out.Bytes(), []byte(`"uid":"payments"`)) {
		t.Fatalf("output=%s", out.String())
	}
}

type grafanaQuerierStub struct {
	dashboards []toolgrafana.DashboardSummary
	dashboard  toolgrafana.Dashboard
}

func (f *grafanaQuerierStub) ListDashboards(
	context.Context,
	toolgrafana.SearchRequest,
) ([]toolgrafana.DashboardSummary, error) {
	return f.dashboards, nil
}

func (f *grafanaQuerierStub) GetDashboard(
	context.Context,
	string,
) (toolgrafana.Dashboard, error) {
	return f.dashboard, nil
}

func TestMultiToolRouteRunsSequentialReadOnlySteps(t *testing.T) {
	provider := &sequenceProvider{structured: []json.RawMessage{
		json.RawMessage(
			`{"kind":"performance","target":{"name":"api","kind":"Deployment"},"params":{"window":"15m"}}`,
		),
		json.RawMessage(
			`{"kind":"trace","target":{"name":"payment","kind":"Service"},"params":{"window":"1h"}}`,
		),
	}}
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Namespace: "prod",
		Prompt:    "why is api slow then trace payment request",
	}, &out, Deps{
		Provider: provider,
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
		OTel: traceQuerierFunc(func(
			context.Context,
			toolotel.SearchRequest,
		) ([]toolotel.Trace, error) {
			return []toolotel.Trace{{
				TraceID:       "trace-1",
				RootService:   "payment",
				RootOperation: "POST /charge",
				Duration:      100 * time.Millisecond,
				Spans: []toolotel.Span{{
					SpanID:    "root",
					Service:   "payment",
					Operation: "POST /charge",
					Duration:  100 * time.Millisecond,
				}},
			}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.index != 2 ||
		!bytes.Contains(out.Bytes(), []byte("Route: 2 sequential steps")) ||
		!bytes.Contains(out.Bytes(), []byte("Performance:")) ||
		!bytes.Contains(out.Bytes(), []byte("Trace: trace-1")) {
		t.Fatalf("calls=%d output=%s", provider.index, out.String())
	}
}

func TestMultiToolRouteJSONIsSingleDocument(t *testing.T) {
	provider := &sequenceProvider{structured: []json.RawMessage{
		json.RawMessage(
			`{"kind":"dashboard","target":{"kind":"Dashboard"}}`,
		),
		json.RawMessage(
			`{"kind":"trace","target":{"name":"payment","kind":"Service"}}`,
		),
	}}
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Prompt: "show dashboards; trace payment request",
		Output: "json",
	}, &out, Deps{
		Provider: provider,
		Grafana: &grafanaQuerierStub{
			dashboards: []toolgrafana.DashboardSummary{{
				UID:   "payments",
				Title: "Payments",
			}},
		},
		OTel: traceQuerierFunc(func(
			context.Context,
			toolotel.SearchRequest,
		) ([]toolotel.Trace, error) {
			return []toolotel.Trace{{
				TraceID: "trace-1",
				Spans:   []toolotel.Span{},
			}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	var result output.RouteResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid route JSON: %v\n%s", err, out.String())
	}
	if result.Kind != output.KindRouteResult ||
		!result.Applied ||
		len(result.Steps) != 2 {
		t.Fatalf("result=%+v", result)
	}
	if result.Steps[0].Plan.Actions[0].Backend != "grafana" ||
		result.Steps[1].Plan.Actions[0].Backend != "opentelemetry" {
		t.Fatalf("route backends=%+v", result.Steps)
	}
}

func TestMultiToolRouteStopsAfterUnapprovedMutation(t *testing.T) {
	provider := &sequenceProvider{structured: []json.RawMessage{
		json.RawMessage(
			`{"kind":"scale","target":{"name":"api","kind":"Deployment"},"params":{"replicas":3}}`,
		),
		json.RawMessage(
			`{"kind":"dashboard","target":{"kind":"Dashboard"}}`,
		),
	}}
	client := fake.NewSimpleClientset(deployment("api", "default", 1))
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Prompt: "scale api to 3 then show dashboards",
	}, &out, Deps{
		Provider:   provider,
		Client:     client,
		IsTerminal: boolPtr(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.index != 2 ||
		!bytes.Contains(out.Bytes(), []byte("Aggregate plan:")) ||
		!bytes.Contains(out.Bytes(), []byte("route was not approved")) {
		t.Fatalf("calls=%d output=%s", provider.index, out.String())
	}
	dep, getErr := client.AppsV1().Deployments("default").Get(
		context.Background(),
		"api",
		metav1.GetOptions{},
	)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 1 {
		t.Fatalf("unapproved route must not mutate, replicas=%v", dep.Spec.Replicas)
	}
}

func TestMultiToolRouteSingleApprovalCoversMutatingChain(t *testing.T) {
	provider := &sequenceProvider{structured: []json.RawMessage{
		json.RawMessage(
			`{"kind":"performance","target":{"name":"api","kind":"Deployment"},"params":{"window":"15m"}}`,
		),
		json.RawMessage(
			`{"kind":"scale","target":{"name":"api","kind":"Deployment"},"params":{"replicas":4}}`,
		),
	}}
	client := fake.NewSimpleClientset(deployment("api", "default", 1))
	confirms := 0
	err := RunWith(context.Background(), config.Resolved{
		Prompt: "why is api slow then scale api to 4",
	}, io.Discard, Deps{
		Provider: provider,
		Client:   client,
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
		Confirm: func(io.Writer) (bool, error) {
			confirms++
			return true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if confirms != 1 {
		t.Fatalf("expected one aggregate approval, got %d", confirms)
	}
	dep, getErr := client.AppsV1().Deployments("default").Get(
		context.Background(),
		"api",
		metav1.GetOptions{},
	)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 4 {
		t.Fatalf("replicas=%v", dep.Spec.Replicas)
	}
}

func TestMultiToolRouteJSONIncludesAggregateApproval(t *testing.T) {
	provider := &sequenceProvider{structured: []json.RawMessage{
		json.RawMessage(
			`{"kind":"scale","target":{"name":"api","kind":"Deployment"},"params":{"replicas":2}}`,
		),
		json.RawMessage(
			`{"kind":"dashboard","target":{"kind":"Dashboard"}}`,
		),
	}}
	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Approve: true,
		Prompt:  "scale api to 2 then show dashboards",
		Output:  "json",
	}, &out, Deps{
		Provider: provider,
		Client:   fake.NewSimpleClientset(deployment("api", "default", 1)),
		Grafana: &grafanaQuerierStub{
			dashboards: []toolgrafana.DashboardSummary{{UID: "d1", Title: "D"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var result output.RouteResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid route JSON: %v\n%s", err, out.String())
	}
	if !result.RequiresApproval || result.Risk.Level == "" || !result.Applied || len(result.Steps) != 2 {
		t.Fatalf("result=%+v", result)
	}
}

func TestMultiToolRoutePreflightsAllStepsBeforeMutation(t *testing.T) {
	provider := &sequenceProvider{structured: []json.RawMessage{
		json.RawMessage(
			`{"kind":"scale","target":{"name":"api","kind":"Deployment"},"params":{"replicas":3}}`,
		),
		json.RawMessage(`{"kind":"unknown","target":{}}`),
	}}
	client := fake.NewSimpleClientset(deployment("api", "default", 1))
	err := RunWith(context.Background(), config.Resolved{
		Approve: true,
		Prompt:  "scale api to 3 then investigate something unsupported",
	}, io.Discard, Deps{
		Provider: provider,
		Client:   client,
	})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("route step 2")) {
		t.Fatalf("err=%v", err)
	}
	dep, getErr := client.AppsV1().Deployments("default").Get(
		context.Background(),
		"api",
		metav1.GetOptions{},
	)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 1 {
		t.Fatalf("preflight failure must not mutate, replicas=%v", dep.Spec.Replicas)
	}
}

type sequenceProvider struct {
	structured []json.RawMessage
	index      int
}

func (p *sequenceProvider) Name() string { return "sequence" }

func (p *sequenceProvider) Complete(
	context.Context,
	llm.CompletionRequest,
) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{}, nil
}

func (p *sequenceProvider) CompleteStructured(
	_ context.Context,
	_ llm.CompletionRequest,
	_ json.RawMessage,
) (json.RawMessage, error) {
	if p.index >= len(p.structured) {
		return nil, fmt.Errorf("unexpected structured completion %d", p.index+1)
	}
	result := p.structured[p.index]
	p.index++
	return result, nil
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
