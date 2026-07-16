package prometheus

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type queryFunc func(context.Context, string, time.Time) (Result, error)

func (f queryFunc) Query(ctx context.Context, query string, at time.Time) (Result, error) {
	return f(ctx, query, at)
}

func TestExplainPerformanceHighLatencyAndHPASuggestion(t *testing.T) {
	querier := queryFunc(func(_ context.Context, query string, _ time.Time) (Result, error) {
		switch {
		case strings.Contains(query, "container_cpu_usage_seconds_total"):
			return vectorValue("0.9"), nil
		case strings.Contains(query, `resource="cpu"`):
			return vectorValue("1"), nil
		case strings.Contains(query, "container_memory_working_set_bytes"):
			return vectorValue("800"), nil
		case strings.Contains(query, `resource="memory"`):
			return vectorValue("1000"), nil
		case strings.Contains(query, "histogram_quantile"):
			return vectorValue("1.25"), nil
		case strings.Contains(query, "kube_deployment_status_replicas"):
			return vectorValue("2"), nil
		case strings.Contains(query, "status_current_replicas"):
			return vectorValue("2"), nil
		case strings.Contains(query, "status_desired_replicas"):
			return vectorValue("4"), nil
		case strings.Contains(query, "spec_max_replicas"):
			return vectorValue("6"), nil
		default:
			return Result{}, errors.New("unexpected query")
		}
	})

	report, err := ExplainPerformance(context.Background(), querier, PerformanceRequest{
		Workload:  "api",
		Namespace: "prod",
		Window:    15 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.Summary, "latency") {
		t.Fatalf("summary=%q", report.Summary)
	}
	if report.Suggestion == nil || report.Suggestion.Suggested != 4 {
		t.Fatalf("suggestion=%+v", report.Suggestion)
	}
	if len(report.Metrics) != 9 {
		t.Fatalf("metrics=%d", len(report.Metrics))
	}
}

func TestExplainPerformanceReturnsPartialReport(t *testing.T) {
	querier := queryFunc(func(_ context.Context, query string, _ time.Time) (Result, error) {
		if strings.Contains(query, "container_cpu_usage_seconds_total") {
			return vectorValue("0.2"), nil
		}
		return Result{}, errors.New("metric unavailable")
	})
	report, err := ExplainPerformance(context.Background(), querier, PerformanceRequest{
		Workload: "api",
		Window:   5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Namespace != "default" {
		t.Fatalf("namespace=%q", report.Namespace)
	}
}

func TestExplainPerformanceFailsWhenAllQueriesFail(t *testing.T) {
	querier := queryFunc(func(context.Context, string, time.Time) (Result, error) {
		return Result{}, errors.New("prometheus unavailable")
	})
	_, err := ExplainPerformance(context.Background(), querier, PerformanceRequest{
		Workload: "api",
		Window:   5 * time.Minute,
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPerformanceQueriesEscapeTarget(t *testing.T) {
	specs := performanceQueries(PerformanceRequest{
		Workload:  `api.*"}`,
		Namespace: `prod"}`,
		Window:    5 * time.Minute,
	})
	for _, spec := range specs {
		if strings.Contains(spec.query, `namespace="prod"}"`) {
			t.Fatalf("unsafe query=%s", spec.query)
		}
	}
}

func TestFirstValue(t *testing.T) {
	value, ok, err := FirstValue(vectorValue("3.5"))
	if err != nil || !ok || value != 3.5 {
		t.Fatalf("value=%v ok=%v err=%v", value, ok, err)
	}
}

func vectorValue(value string) Result {
	return Result{
		Type: "vector",
		Series: []Series{{
			Metric: map[string]string{},
			Samples: []Sample{{
				Timestamp: 1,
				Value:     value,
			}},
		}},
	}
}
