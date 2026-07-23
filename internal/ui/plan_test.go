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

func TestPrintBlastRadius(t *testing.T) {
	var buf bytes.Buffer
	PrintPlan(&buf, planner.ExecutionPlan{
		Intent:  intent.Intent{Kind: intent.KindScale},
		Summary: "Scale Deployment/api",
		Actions: []planner.Action{{
			Op:     planner.OpScale,
			Object: planner.ObjectRef{Kind: "Deployment", Name: "api", Namespace: "demo"},
		}},
		RequiresApproval: true,
		BlastRadius: &planner.BlastRadius{
			Namespaces: []string{"demo"},
			Targets: []planner.BlastTarget{{
				Op: "scale", Kind: "Deployment", Name: "api", Namespace: "demo",
				Labels:  map[string]string{"app": "api"},
				Related: []planner.BlastRelated{{Kind: "HorizontalPodAutoscaler", Name: "api-hpa", Relation: "scales"}},
			}},
		},
	}, safety.Result{Risk: safety.RiskMedium})
	out := buf.String()
	if !strings.Contains(out, "Blast radius:") || !strings.Contains(out, "namespaces: demo") || !strings.Contains(out, "scales: HorizontalPodAutoscaler/api-hpa") {
		t.Fatalf("output=\n%s", out)
	}
}
