package optimize

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
)

type idleQuerierFunc func(ctx context.Context, promQL string, at time.Time) (toolprometheus.Result, error)

func (f idleQuerierFunc) Query(ctx context.Context, promQL string, at time.Time) (toolprometheus.Result, error) {
	return f(ctx, promQL, at)
}

func TestDetectIdleMarksLowCPU(t *testing.T) {
	q := idleQuerierFunc(func(_ context.Context, promQL string, _ time.Time) (toolprometheus.Result, error) {
		switch {
		case strings.Contains(promQL, "container_cpu_usage_seconds_total"):
			return vector("0.03"), nil
		case strings.Contains(promQL, `resource="cpu"`):
			return vector("1"), nil
		case strings.Contains(promQL, "container_memory_working_set_bytes"):
			return vector("500000000"), nil
		case strings.Contains(promQL, `resource="memory"`):
			return vector("1000000000"), nil
		default:
			return toolprometheus.Result{}, fmt.Errorf("unexpected query %s", promQL)
		}
	})
	idle := DetectIdle(context.Background(), q, []Workload{{
		Kind: WorkloadDeployment, Namespace: "prod", Name: "api",
	}}, time.Hour)
	if idle.Skipped || idle.IdleCount != 1 || len(idle.Workloads) != 1 {
		t.Fatalf("%+v", idle)
	}
	if idle.Workloads[0].CPUOfRequestPct == nil || *idle.Workloads[0].CPUOfRequestPct < 2 || *idle.Workloads[0].CPUOfRequestPct > 4 {
		t.Fatalf("cpu pct=%v", idle.Workloads[0].CPUOfRequestPct)
	}
	if !strings.Contains(idle.Workloads[0].Message, "Deployment/api") ||
		!strings.Contains(idle.Workloads[0].Message, "CPU of request") {
		t.Fatalf("message=%s", idle.Workloads[0].Message)
	}

	rep := BuildScaffold(Request{})
	ApplyInventory(&rep, Inventory{Workloads: []Workload{{
		Kind: WorkloadDeployment, Namespace: "prod", Name: "api",
	}}, Namespaces: 1})
	ApplyIdle(&rep, idle)
	if rep.Sections.Idle.Status != SectionReady {
		t.Fatalf("section=%+v", rep.Sections.Idle)
	}
	if len(rep.Idle) != 1 || len(rep.Suggestions) == 0 {
		t.Fatalf("report idle=%+v suggestions=%+v", rep.Idle, rep.Suggestions)
	}
}

func TestDetectIdleSkipsWithoutQuerier(t *testing.T) {
	idle := DetectIdle(context.Background(), nil, []Workload{{Name: "api"}}, time.Hour)
	if !idle.Skipped {
		t.Fatalf("%+v", idle)
	}
	rep := BuildScaffold(Request{})
	ApplyIdle(&rep, idle)
	if rep.Sections.Idle.Status != SectionSkipped {
		t.Fatalf("%+v", rep.Sections.Idle)
	}
}

func TestDetectIdlePartialFailuresAreWarnings(t *testing.T) {
	q := idleQuerierFunc(func(_ context.Context, promQL string, _ time.Time) (toolprometheus.Result, error) {
		if strings.Contains(promQL, "container_cpu_usage_seconds_total") {
			return toolprometheus.Result{}, fmt.Errorf("timeout")
		}
		if strings.Contains(promQL, `resource="cpu"`) {
			return vector("1"), nil
		}
		if strings.Contains(promQL, "container_memory_working_set_bytes") {
			return vector("10000000"), nil
		}
		if strings.Contains(promQL, `resource="memory"`) {
			return vector("1000000000"), nil
		}
		return toolprometheus.Result{}, nil
	})
	idle := DetectIdle(context.Background(), q, []Workload{{
		Kind: WorkloadDeployment, Namespace: "default", Name: "web",
	}}, time.Hour)
	if idle.Skipped {
		t.Fatal("should not skip on partial failure")
	}
	if idle.IdleCount != 1 {
		t.Fatalf("expected memory-idle, got %+v", idle)
	}
	if len(idle.Warnings) == 0 {
		t.Fatal("expected cpu warning")
	}
}

func TestDetectIdleBusyWorkload(t *testing.T) {
	q := idleQuerierFunc(func(_ context.Context, promQL string, _ time.Time) (toolprometheus.Result, error) {
		switch {
		case strings.Contains(promQL, "container_cpu_usage_seconds_total"):
			return vector("0.8"), nil
		case strings.Contains(promQL, `resource="cpu"`):
			return vector("1"), nil
		case strings.Contains(promQL, "container_memory_working_set_bytes"):
			return vector("800000000"), nil
		case strings.Contains(promQL, `resource="memory"`):
			return vector("1000000000"), nil
		default:
			return toolprometheus.Result{}, nil
		}
	})
	idle := DetectIdle(context.Background(), q, []Workload{{
		Kind: WorkloadDeployment, Namespace: "prod", Name: "api",
	}}, time.Hour)
	if idle.IdleCount != 0 || len(idle.Workloads) != 0 {
		t.Fatalf("%+v", idle)
	}
}

func vector(value string) toolprometheus.Result {
	return toolprometheus.Result{
		Type: "vector",
		Series: []toolprometheus.Series{{
			Samples: []toolprometheus.Sample{{Timestamp: 1, Value: value}},
		}},
	}
}
