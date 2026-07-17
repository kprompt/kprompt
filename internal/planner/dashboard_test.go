package planner

import (
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildDashboardListPlan(t *testing.T) {
	plan, err := Build(intent.Intent{Kind: intent.KindDashboard})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval || len(plan.Actions) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Actions[0].Op != OpGrafanaQuery ||
		plan.Actions[0].Backend != "grafana" ||
		plan.Actions[0].Object.Kind != "Dashboard" {
		t.Fatalf("action=%+v", plan.Actions[0])
	}
}

func TestBuildDashboardUIDPlan(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindDashboard,
		Params: map[string]any{"uid": "payments"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Actions[0].Object.Name != "payments" {
		t.Fatalf("action=%+v", plan.Actions[0])
	}
}
