package autopilot

import (
	"testing"

	"github.com/kprompt/kprompt/internal/agent/ctxbuild"
	"github.com/kprompt/kprompt/internal/agent/patterns"
	"github.com/kprompt/kprompt/internal/incident"
)

func TestAttachProposalToAlertCases(t *testing.T) {
	// nil inputs: no panic.
	AttachProposalToAlert(nil, nil)

	// denied proposals are skipped.
	a := &incident.AgentAlert{Namespace: "payments"}
	AttachProposalToAlert(a, &Proposal{Decision: DecisionDenied, ID: "ap-x"})
	if a.ProposalID != "" {
		t.Fatalf("denied proposal must not stamp alert: %+v", a)
	}

	// namespace falls back to the alert namespace when the proposal omits it.
	AttachProposalToAlert(a, &Proposal{ID: "ap-1", ActionID: ActionRestartDeployment, Risk: "low"})
	if a.ProposalID != "ap-1" || a.ProposalHint == "" {
		t.Fatalf("expected hint stamped: %+v", a)
	}
}

func TestWriteLearnOutcomeActionBranches(t *testing.T) {
	// empty outcome -> no-op, no error.
	if _, err := WriteLearnOutcomeAction(nil, ctxbuild.AgentContext{}, "", ""); err != nil {
		t.Fatalf("nil lib: %v", err)
	}
	lib := patterns.New(patterns.FileStore{Dir: t.TempDir()})
	if _, err := WriteLearnOutcomeAction(lib, ctxbuild.AgentContext{}, "", ""); err != nil {
		t.Fatalf("empty outcome: %v", err)
	}
	// empty namespace -> error.
	if _, err := WriteLearnOutcomeAction(lib, ctxbuild.AgentContext{}, patterns.OutcomeApplyFailed, ""); err == nil {
		t.Fatal("expected empty-namespace error")
	}
	// actionID path records against the store.
	ctx := ctxbuild.AgentContext{
		Namespace: "payments",
		Incident: incident.Incident{
			Summary:         "CrashLoopBackOff",
			PrimaryResource: &incident.ResourceRef{Kind: "Deployment", Name: "api"},
		},
	}
	if _, err := WriteLearnOutcomeAction(lib, ctx, patterns.OutcomeApplyFailed, ActionRestartDeployment); err != nil {
		t.Fatalf("record with action: %v", err)
	}
	// namespace fallback from incident.
	ctx2 := ctxbuild.AgentContext{Incident: incident.Incident{Namespace: "payments", Summary: "x"}}
	if _, err := WriteLearnOutcome(lib, ctx2, patterns.OutcomeApplySuccess); err != nil {
		t.Fatalf("record with incident ns: %v", err)
	}
}

func TestReasonForAction(t *testing.T) {
	cases := map[string]string{
		ActionRollbackFailedRollout: "ProgressDeadlineExceeded",
		ActionRestartDeployment:     "CrashLoopBackOff",
		ActionScaleDeployment:       "ScalingReplicaSet",
		ActionEvictPod:              "Evicted",
		"unknownAction":             "Other",
	}
	for action, want := range cases {
		if got := reasonForAction(action); got != want {
			t.Errorf("reasonForAction(%q)=%q want %q", action, got, want)
		}
	}
}

func TestVerifyPlanForBranches(t *testing.T) {
	reps := int32(2)
	if _, err := verifyPlanFor(Proposal{ActionID: ActionScaleDeployment, Namespace: "ns", TargetName: "api", Replicas: &reps}); err != nil {
		t.Fatalf("scale verify plan: %v", err)
	}
	if _, err := verifyPlanFor(Proposal{ActionID: "unsupported", Namespace: "ns", TargetName: "api"}); err == nil {
		t.Fatal("expected error for unsupported verify plan")
	}
}
