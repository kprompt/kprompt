package optimize

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
)

func TestSuggestRightsizingLowersMemoryRequest(t *testing.T) {
	q := idleQuerierFunc(func(_ context.Context, promQL string, _ time.Time) (toolprometheus.Result, error) {
		switch {
		case strings.Contains(promQL, "container_cpu_usage_seconds_total"):
			return vector("0.05"), nil
		case strings.Contains(promQL, `resource="cpu"`) && strings.Contains(promQL, "requests"):
			return vector("1"), nil
		case strings.Contains(promQL, `resource="cpu"`) && strings.Contains(promQL, "limits"):
			return vector("2"), nil
		case strings.Contains(promQL, "quantile_over_time"):
			// ~128Mi working set vs 512Mi request → lower
			return vector(fmt.Sprintf("%d", 128*1024*1024)), nil
		case strings.Contains(promQL, `resource="memory"`) && strings.Contains(promQL, "requests"):
			return vector(fmt.Sprintf("%d", 512*1024*1024)), nil
		case strings.Contains(promQL, `resource="memory"`) && strings.Contains(promQL, "limits"):
			return vector(fmt.Sprintf("%d", 1024*1024*1024)), nil
		default:
			return toolprometheus.Result{}, fmt.Errorf("unexpected %s", promQL)
		}
	})

	rs := SuggestRightsizing(context.Background(), q, []Workload{{
		Kind: WorkloadDeployment, Namespace: "prod", Name: "api",
		CPURequest: "1", MemoryRequest: "512Mi",
		CPULimit: "2", MemoryLimit: "1Gi",
	}}, time.Hour)
	if rs.Skipped || len(rs.Deltas) == 0 {
		t.Fatalf("%+v", rs)
	}

	var memReq *RightsizingDelta
	for i := range rs.Deltas {
		d := &rs.Deltas[i]
		if d.Resource == "memory" && d.Field == "request" {
			memReq = d
		}
	}
	if memReq == nil || memReq.Direction != "lower" {
		t.Fatalf("deltas=%+v", rs.Deltas)
	}
	if !strings.Contains(memReq.Message, "512Mi→") {
		t.Fatalf("message=%s", memReq.Message)
	}

	rep := BuildScaffold(Request{})
	ApplyRightsizing(&rep, rs)
	if rep.Sections.Rightsizing.Status != SectionReady {
		t.Fatalf("%+v", rep.Sections.Rightsizing)
	}
	if len(rep.Rightsizing) == 0 || len(rep.Suggestions) == 0 {
		t.Fatalf("report=%+v", rep)
	}
	foundHint := false
	for _, s := range rep.Suggestions {
		if s.ActionHint == "patch-resources" {
			foundHint = true
		}
	}
	if !foundHint {
		t.Fatalf("suggestions=%+v", rep.Suggestions)
	}
}

func TestSuggestRightsizingSkipsWithoutQuerier(t *testing.T) {
	rs := SuggestRightsizing(context.Background(), nil, []Workload{{Name: "api"}}, time.Hour)
	if !rs.Skipped {
		t.Fatalf("%+v", rs)
	}
	rep := BuildScaffold(Request{})
	ApplyRightsizing(&rep, rs)
	if rep.Sections.Rightsizing.Status != SectionSkipped {
		t.Fatalf("%+v", rep.Sections.Rightsizing)
	}
}

func TestSuggestRightsizingNoDeltaWhenClose(t *testing.T) {
	q := idleQuerierFunc(func(_ context.Context, promQL string, _ time.Time) (toolprometheus.Result, error) {
		switch {
		case strings.Contains(promQL, "container_cpu_usage_seconds_total"):
			return vector("0.7"), nil // matches 1 core @ 70% target
		case strings.Contains(promQL, `resource="cpu"`) && strings.Contains(promQL, "requests"):
			return vector("1"), nil
		case strings.Contains(promQL, "quantile_over_time"):
			return vector(fmt.Sprintf("%d", 400*1024*1024)), nil // ~512Mi with headroom
		case strings.Contains(promQL, `resource="memory"`) && strings.Contains(promQL, "requests"):
			return vector(fmt.Sprintf("%d", 512*1024*1024)), nil
		default:
			return toolprometheus.Result{}, nil
		}
	})
	rs := SuggestRightsizing(context.Background(), q, []Workload{{
		Kind: WorkloadDeployment, Namespace: "prod", Name: "api",
		CPURequest: "1", MemoryRequest: "512Mi",
	}}, time.Hour)
	if rs.Skipped {
		t.Fatal("should not skip")
	}
	if len(rs.Deltas) != 0 {
		t.Fatalf("expected no deltas, got %+v", rs.Deltas)
	}
}

func TestNiceMemoryAndCPU(t *testing.T) {
	if got := niceMemory(100 * 1024 * 1024); got != "128Mi" {
		t.Fatalf("mem=%s", got)
	}
	if got := niceCPU(0.123); got != "130m" {
		t.Fatalf("cpu=%s", got)
	}
}
