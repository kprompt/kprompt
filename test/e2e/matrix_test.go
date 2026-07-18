//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/pipeline"
	toolgrafana "github.com/kprompt/kprompt/internal/tools/grafana"
	toolotel "github.com/kprompt/kprompt/internal/tools/otel"
	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
)

// TestIntegrationMatrix is the T-046 coverage index: one stub-LLM smoke path
// per integration family. Kind-backed families skip when fixtures are missing.
func TestIntegrationMatrix(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	t.Run("kubernetes_get", func(t *testing.T) {
		requireKind(t)
		ensureKindCluster(t, ctx)
		kubeconfig := exportKubeconfig(t, ctx)
		t.Setenv("KUBECONFIG", kubeconfig)
		client := clientFromKubeconfig(t, kubeconfig)
		ensureNamespace(t, ctx, client)
		ensureDeployment(t, ctx, client, 1)
		deps := pipelineKubeDeps(t, kubeconfig)
		deps.Provider = llm.GetStub("Deployment", "", ns, "")
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Namespace: ns, Approve: false, Prompt: "list deployments",
		}, &out, deps)
		if err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		if !strings.Contains(out.String(), deployName) {
			t.Fatalf("expected %s:\n%s", deployName, out.String())
		}
	})

	t.Run("helm_install_plan", func(t *testing.T) {
		if _, err := exec.LookPath("helm"); err != nil {
			t.Skip("helm not installed")
		}
		requireKind(t)
		ensureKindCluster(t, ctx)
		kubeconfig := exportKubeconfig(t, ctx)
		t.Setenv("KUBECONFIG", kubeconfig)
		deps := pipelineKubeDeps(t, kubeconfig)
		deps.Provider = llm.InstallStub("redis", ns, 1)
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Namespace: ns, Approve: false, Prompt: "install redis",
		}, &out, deps)
		if err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		text := out.String()
		if !strings.Contains(strings.ToLower(text), "helm") && !strings.Contains(strings.ToLower(text), "install") {
			t.Fatalf("expected helm install plan:\n%s", text)
		}
		if strings.Contains(text, "✓ Applied") {
			t.Fatal("plan-only run must not apply without --approve")
		}
	})

	t.Run("argo_workflow_plan", func(t *testing.T) {
		requireKind(t)
		ensureKindCluster(t, ctx)
		kubeconfig := exportKubeconfig(t, ctx)
		t.Setenv("KUBECONFIG", kubeconfig)
		deps := pipelineKubeDeps(t, kubeconfig)
		deps.Provider = &llm.Stub{Structured: []byte(
			`{"kind":"workflow","target":{"name":"train-yolov11","namespace":"kprompt-e2e"},"params":{"task":"train","model":"yolov11"},"confidence":1}`,
		)}
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Namespace: ns, Approve: false, Prompt: "train a yolov11 model",
		}, &out, deps)
		if err != nil {
			// Missing Workflow CRD is an acceptable skip for clusters without Argo.
			if strings.Contains(err.Error(), "Workflow") || strings.Contains(err.Error(), "argo") ||
				strings.Contains(err.Error(), "not installed") || strings.Contains(err.Error(), "CRD") {
				t.Skipf("argo workflows not available: %v", err)
			}
			t.Fatalf("%v\n%s", err, out.String())
		}
		text := out.String()
		if !strings.Contains(strings.ToLower(text), "workflow") {
			t.Fatalf("expected workflow plan:\n%s", text)
		}
	})

	t.Run("prometheus_performance", func(t *testing.T) {
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Namespace: "prod", Prompt: "why is my api slow",
		}, &out, pipeline.Deps{
			Provider: llm.PerformanceStub("api", "prod", "15m"),
			Prometheus: matrixPromQuerier(func(context.Context, string, time.Time) (toolprometheus.Result, error) {
				return toolprometheus.Result{
					Type: "vector",
					Series: []toolprometheus.Series{{
						Samples: []toolprometheus.Sample{{Timestamp: 1, Value: "0.5"}},
					}},
				}, nil
			}),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Performance:") {
			t.Fatalf("output=%s", out.String())
		}
	})

	t.Run("otel_trace_walk", func(t *testing.T) {
		start := time.Unix(1, 0)
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Prompt: "trace payment request",
		}, &out, pipeline.Deps{
			Provider: &llm.Stub{Structured: []byte(
				`{"kind":"trace","target":{"name":"payment","kind":"Service"},"confidence":1}`,
			)},
			OTel: matrixTraceQuerier(func(_ context.Context, req toolotel.SearchRequest) ([]toolotel.Trace, error) {
				return []toolotel.Trace{{
					TraceID: "trace-1", RootService: "payment", RootOperation: "POST /charge",
					StartTime: start, Duration: 4 * time.Second,
					Spans: []toolotel.Span{
						{SpanID: "root", Service: "payment", Operation: "POST /charge", StartTime: start, Duration: 4 * time.Second},
						{SpanID: "db", ParentSpanID: "root", Service: "postgres", Operation: "SELECT", StartTime: start, Duration: 3 * time.Second},
					},
				}}, nil
			}),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Trace:") {
			t.Fatalf("output=%s", out.String())
		}
	})

	t.Run("grafana_dashboard", func(t *testing.T) {
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Prompt: "show payments dashboard",
		}, &out, pipeline.Deps{
			Provider: &llm.Stub{Structured: []byte(
				`{"kind":"dashboard","target":{"name":"payments","kind":"Dashboard"},"confidence":1}`,
			)},
			Grafana: &matrixGrafanaStub{
				dashboards: []toolgrafana.DashboardSummary{{UID: "payments", Title: "Payments"}},
				dashboard: toolgrafana.Dashboard{
					UID: "payments", Title: "Payments", URL: "https://grafana.example/d/payments",
					Panels: []toolgrafana.Panel{{ID: 1, Title: "Rate", Type: "timeseries"}},
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Payments") {
			t.Fatalf("output=%s", out.String())
		}
	})

	t.Run("generic_kubernetes_read", func(t *testing.T) {
		requireKind(t)
		ensureKindCluster(t, ctx)
		kubeconfig := exportKubeconfig(t, ctx)
		t.Setenv("KUBECONFIG", kubeconfig)
		deps := pipelineKubeDeps(t, kubeconfig)
		deps.Provider = llm.GetStub("Node", "", "", "")
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Approve: false, Prompt: "list nodes",
		}, &out, deps)
		if err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		if !strings.Contains(out.String(), "NAME") {
			t.Fatalf("expected node table:\n%s", out.String())
		}
	})
}

type matrixPromQuerier func(context.Context, string, time.Time) (toolprometheus.Result, error)

func (f matrixPromQuerier) Query(ctx context.Context, q string, at time.Time) (toolprometheus.Result, error) {
	return f(ctx, q, at)
}

type matrixTraceQuerier func(context.Context, toolotel.SearchRequest) ([]toolotel.Trace, error)

func (f matrixTraceQuerier) SearchTraces(ctx context.Context, req toolotel.SearchRequest) ([]toolotel.Trace, error) {
	return f(ctx, req)
}

type matrixGrafanaStub struct {
	dashboards []toolgrafana.DashboardSummary
	dashboard  toolgrafana.Dashboard
}

func (g *matrixGrafanaStub) ListDashboards(context.Context, toolgrafana.SearchRequest) ([]toolgrafana.DashboardSummary, error) {
	return g.dashboards, nil
}
func (g *matrixGrafanaStub) GetDashboard(context.Context, string) (toolgrafana.Dashboard, error) {
	return g.dashboard, nil
}
