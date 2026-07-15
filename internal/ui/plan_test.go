package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
)

func TestPrintPlanMultilineDiff(t *testing.T) {
	rep := int32(3)
	var buf bytes.Buffer
	PrintPlan(&buf, planner.ExecutionPlan{
		Intent:  intent.Intent{Kind: intent.KindScale},
		Summary: "Scale Deployment/api",
		Actions: []planner.Action{{
			Op:       planner.OpScale,
			Object:   planner.ObjectRef{Kind: "Deployment", Name: "api", Namespace: "default"},
			Replicas: &rep,
			Diff:     "replicas: 1 → 3",
		}},
		RequiresApproval: true,
	}, safety.Result{Risk: safety.RiskMedium})
	out := buf.String()
	if !strings.Contains(out, "Diff:") || !strings.Contains(out, "replicas: 1 → 3") {
		t.Fatalf("output=\n%s", out)
	}
}
