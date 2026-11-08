package autopilot

import (
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/agent/ctxbuild"
	"github.com/kprompt/kprompt/internal/agent/patterns"
	"github.com/kprompt/kprompt/internal/incident"
)

func TestDetectCandidatesOverlap(t *testing.T) {
	ctx := ctxbuild.AgentContext{
		Namespace: "payments",
		Incident: incident.Incident{
			Summary:         "CrashLoopBackOff and ProgressDeadlineExceeded on api rollout",
			PrimaryResource: &incident.ResourceRef{Kind: "Deployment", Name: "api"},
		},
		Deployment: &ctxbuild.DeploymentSnapshot{Name: "api", DesiredReplicas: 2, ReadyReplicas: 0},
		Target:     &incident.ResourceRef{Kind: "Deployment", Name: "api"},
	}
	cands := detectCandidates(ctx)
	if len(cands) < 2 {
		t.Fatalf("expected rollback+restart candidates, got %+v", cands)
	}
	if cands[0].Action != ActionRollbackFailedRollout {
		t.Fatalf("base rank should prefer rollback: %+v", cands)
	}
}

func TestDetectCandidatesPrefersRollbackOnRevisionImagePull(t *testing.T) {
	// IC-001 magic-moment shape: broken rollout image before ProgressDeadlineExceeded.
	ctx := ctxbuild.AgentContext{
		Namespace: "payments",
		Incident: incident.Incident{
			Summary:         "Image pull failure",
			RootCause:       "Image name/tag invalid; recent change-cause: kprompt-examples: revision 2 (broken image)",
			PrimaryResource: &incident.ResourceRef{Kind: "Deployment", Name: "checkout"},
		},
		Deployment: &ctxbuild.DeploymentSnapshot{Name: "checkout", DesiredReplicas: 2, ReadyReplicas: 2},
		Target:     &incident.ResourceRef{Kind: "Deployment", Name: "checkout"},
	}
	cands := detectCandidates(ctx)
	if len(cands) < 1 {
		t.Fatalf("expected candidates, got none")
	}
	if cands[0].Action != ActionRollbackFailedRollout {
		t.Fatalf("want rollbackFailedRollout first for bad revision image pull, got %+v", cands)
	}
}

func TestRankCandidatesPrefersLastAction(t *testing.T) {
	cands := []candidate{
		{Action: ActionRollbackFailedRollout, Base: 40},
		{Action: ActionRestartDeployment, Base: 30},
	}
	match := patterns.Pattern{
		Count: 5, Weight: 1, Confirmed: 3, LastActionID: ActionRestartDeployment,
	}
	ranked := rankCandidates(cands, match, true)
	if ranked[0].Action != ActionRestartDeployment {
		t.Fatalf("Learn should prefer last successful restart: %+v", ranked)
	}
}

func TestRankCandidatesFPDemotesLastAction(t *testing.T) {
	cands := []candidate{
		{Action: ActionRollbackFailedRollout, Base: 40},
		{Action: ActionRestartDeployment, Base: 30},
	}
	match := patterns.Pattern{
		Count: 5, Weight: 0.3, Confirmed: 0, FalsePositives: 3, LastActionID: ActionRestartDeployment,
	}
	ranked := rankCandidates(cands, match, true)
	if ranked[0].Action != ActionRollbackFailedRollout {
		t.Fatalf("FP/low weight should fall back to base rank: %+v", ranked)
	}
}

func TestProposeLearnBiasActionConfidence(t *testing.T) {
	lib := patterns.New(patterns.NewMemStore())
	ctx := ctxbuild.AgentContext{
		Namespace: "payments",
		Incident: incident.Incident{
			ID:              "inc-1",
			Summary:         "CrashLoopBackOff on api",
			Evidence:        []incident.EvidenceRef{{Type: incident.EvidenceEvent, Reason: "BackOff", Message: "Back-off"}},
			PrimaryResource: &incident.ResourceRef{Kind: "Deployment", Name: "api"},
		},
		Deployment: &ctxbuild.DeploymentSnapshot{Name: "api", DesiredReplicas: 2, ReadyReplicas: 1},
		Target:     &incident.ResourceRef{Kind: "Deployment", Name: "api"},
	}
	for i := 0; i < 2; i++ {
		if _, err := lib.Record("payments", ctx, "high", "crash", "bad", "restart"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := lib.RecordOutcomeAction("payments", ctx, patterns.OutcomeApplySuccess, ActionRestartDeployment); err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Policy: DefaultPolicy(), Audit: &MemAudit{}, Patterns: lib}
	p, err := eng.ProposeFromContext(ctx, 0.9)
	if err != nil || p == nil {
		t.Fatalf("%v %+v", err, p)
	}
	if p.ActionID != ActionRestartDeployment {
		t.Fatalf("action=%s", p.ActionID)
	}
	if p.LearnNote == "" || !strings.Contains(p.LearnNote, "Learn bias") {
		t.Fatalf("expected LearnNote, got %+v", p)
	}
	if p.ActionConfidence == 0 {
		t.Fatal("expected biased ActionConfidence")
	}
}
