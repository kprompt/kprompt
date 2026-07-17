package planner

import (
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildTracePlan(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindTrace,
		Target: intent.Target{Name: "payment", Kind: "Service"},
		Params: map[string]any{"window": "30m"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval || len(plan.Actions) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Actions[0].Op != OpTraceQuery ||
		plan.Actions[0].Backend != "opentelemetry" {
		t.Fatalf("action=%+v", plan.Actions[0])
	}
	if !strings.Contains(plan.Actions[0].Diff, "30m") {
		t.Fatalf("diff=%q", plan.Actions[0].Diff)
	}
}

func TestBuildTraceRejectsLongWindow(t *testing.T) {
	_, err := Build(intent.Intent{
		Kind:   intent.KindTrace,
		Target: intent.Target{Name: "payment"},
		Params: map[string]any{"window": "25h"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}
