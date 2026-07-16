package planner

import (
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildPerformancePlan(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind: intent.KindPerformance,
		Target: intent.Target{
			Name:      "api",
			Namespace: "prod",
		},
		Params: map[string]any{"window": "30m"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval {
		t.Fatal("performance query must be read-only")
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("actions=%d", len(plan.Actions))
	}
	action := plan.Actions[0]
	if action.Op != OpPromQuery || action.Backend != "prometheus" {
		t.Fatalf("action=%+v", action)
	}
	if !strings.Contains(plan.Summary, "30m") {
		t.Fatalf("summary=%q", plan.Summary)
	}
}

func TestBuildPerformanceRejectsLargeWindow(t *testing.T) {
	_, err := Build(intent.Intent{
		Kind:   intent.KindPerformance,
		Target: intent.Target{Name: "api"},
		Params: map[string]any{"window": "48h"},
	})
	if err == nil {
		t.Fatal("expected window error")
	}
}
