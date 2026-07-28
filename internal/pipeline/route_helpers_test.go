package pipeline

import (
	"testing"

	"github.com/kprompt/kprompt/internal/output"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
)

func TestRouteRouteNeedsApproval(t *testing.T) {
	if routeNeedsApproval(nil) {
		t.Fatal("nil plans should not require approval")
	}
	if routeNeedsApproval([]planner.ExecutionPlan{{}, {}}) {
		t.Fatal("read-only plans should not require approval")
	}
	if !routeNeedsApproval([]planner.ExecutionPlan{{}, {RequiresApproval: true}}) {
		t.Fatal("expected approval when any step mutates")
	}
}

func TestRouteFirstMutatingStep(t *testing.T) {
	if got := firstMutatingStep(nil); got != 1 {
		t.Fatalf("got %d want 1", got)
	}
	if got := firstMutatingStep([]planner.ExecutionPlan{{}, {RequiresApproval: true}}); got != 2 {
		t.Fatalf("got %d want 2", got)
	}
	if got := firstMutatingStep([]planner.ExecutionPlan{{RequiresApproval: true}, {}}); got != 1 {
		t.Fatalf("got %d want 1", got)
	}
}

func TestRouteAggregateRouteRisk(t *testing.T) {
	t.Run("empty risks defaults low", func(t *testing.T) {
		got := aggregateRouteRisk(nil)
		if got.Risk != safety.RiskLow || got.Denied || got.Message != "" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("picks highest risk", func(t *testing.T) {
		got := aggregateRouteRisk([]safety.Result{
			{Risk: safety.RiskLow},
			{Risk: safety.RiskMedium},
			{Risk: safety.RiskHigh},
			{Risk: safety.RiskDenied, Denied: true, Message: "denied by policy"},
		})
		if got.Risk != safety.RiskDenied || !got.Denied || got.Message != "denied by policy" {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("fills default medium message", func(t *testing.T) {
		got := aggregateRouteRisk([]safety.Result{{Risk: safety.RiskMedium}})
		if got.Message != "Mutation requires approval" {
			t.Fatalf("got message %q", got.Message)
		}
	})

	t.Run("fills default high message", func(t *testing.T) {
		got := aggregateRouteRisk([]safety.Result{{Risk: safety.RiskHigh}})
		if got.Message != "High-risk mutation requires approval" {
			t.Fatalf("got message %q", got.Message)
		}
	})
}

func TestRouteRouteStopReason(t *testing.T) {
	tests := []struct {
		name string
		in   output.PlanResult
		want string
	}{
		{
			name: "denied",
			in: output.PlanResult{
				Applied: false,
				Risk:    output.RiskPayload{Denied: true},
				Plan:    output.PlanPayload{RequiresApproval: true},
			},
			want: "step denied by safety policy",
		},
		{
			name: "not approved",
			in: output.PlanResult{
				Applied: false,
				Plan:    output.PlanPayload{RequiresApproval: true},
			},
			want: "step was not approved",
		},
		{
			name: "applied",
			in: output.PlanResult{
				Applied: true,
			},
			want: "",
		},
		{
			name: "not complete",
			in: output.PlanResult{
				Applied: false,
			},
			want: "step did not complete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := routeStopReason(tt.in); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}
