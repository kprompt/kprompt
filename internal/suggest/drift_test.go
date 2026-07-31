package suggest

import (
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/drift"
	"github.com/kprompt/kprompt/internal/incident"
	"github.com/kprompt/kprompt/internal/planner"
)

func TestFromDriftSyncPlans(t *testing.T) {
	inv := incident.NewInvestigation("check drift", "flux-system")
	inv.Findings = []incident.Finding{{
		Code:     drift.CodeOutOfSync,
		Severity: incident.SeverityHigh,
		Title:    "out of sync",
		Message:  "flux apps out of sync",
		Evidence: []incident.EvidenceRef{{
			Type:   incident.EvidenceGitOps,
			Source: "flux",
			Resource: &incident.ResourceRef{
				Kind: "Kustomization", Name: "apps", Namespace: "flux-system",
			},
		}},
	}}
	suggestions, err := FromDrift(inv)
	if err != nil {
		t.Fatal(err)
	}
	actionable := ActionablePlans(suggestions)
	if len(actionable) != 1 || actionable[0].Plan == nil {
		t.Fatalf("want 1 sync plan: %+v", suggestions)
	}
	if actionable[0].Plan.Actions[0].Op != planner.OpGitOpsSync {
		t.Fatalf("op=%s", actionable[0].Plan.Actions[0].Op)
	}
	if !actionable[0].Plan.RequiresApproval {
		t.Fatal("sync must require approval")
	}
	engine, _ := actionable[0].Plan.Intent.StringParam("engine")
	if engine != "flux" {
		t.Fatalf("engine=%s", engine)
	}
}

func TestFromDriftMissingIsGuidance(t *testing.T) {
	inv := incident.NewInvestigation("drift", "all")
	inv.Findings = []incident.Finding{{
		Code:     drift.CodeMissing,
		Severity: incident.SeverityInfo,
		Title:    "missing",
		Message:  "no gitops",
	}}
	suggestions, err := FromDrift(inv)
	if err != nil {
		t.Fatal(err)
	}
	if len(ActionablePlans(suggestions)) != 0 {
		t.Fatalf("missing must be guidance-only: %+v", suggestions)
	}
	if len(suggestions) == 0 {
		t.Fatal("expected guidance")
	}
	if !strings.Contains(suggestions[0].Summary, "docs/gitops-pr.md") && !strings.Contains(suggestions[0].Summary, "--gitops --gitops-repo") {
		t.Fatalf("guidance=%q", suggestions[0].Summary)
	}
	if strings.Contains(suggestions[0].Summary, "T-072") || strings.Contains(suggestions[0].Summary, "not shipped") {
		t.Fatalf("stale guidance=%q", suggestions[0].Summary)
	}
}
