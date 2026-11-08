package autopilot

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/incident"
)

func TestProposalLibraryPutGetList(t *testing.T) {
	lib := NewProposalLibrary(NewMemProposalStore())
	prop := Proposal{
		ID: "ap-1", Namespace: "payments", ActionID: ActionRestartDeployment,
		Decision: DecisionProposed, TargetName: "api", Confidence: 0.9,
	}
	if err := lib.Put(prop); err != nil {
		t.Fatal(err)
	}
	got, err := lib.Get("payments", "ap-1")
	if err != nil || got.ActionID != ActionRestartDeployment {
		t.Fatalf("%v %+v", err, got)
	}
	snap, err := lib.List("payments")
	if err != nil || len(snap.Proposals) != 1 {
		t.Fatalf("%v %+v", err, snap)
	}
	prop.Decision = DecisionApplied
	prop.Applied = true
	if err := lib.Put(prop); err != nil {
		t.Fatal(err)
	}
	got, err = lib.Get("payments", "ap-1")
	if err != nil || !got.Applied || got.Decision != DecisionApplied {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestProposalLibraryFindOpen(t *testing.T) {
	lib := NewProposalLibrary(NewMemProposalStore())
	prop := Proposal{
		ID: "ap-1", Namespace: "payments", IncidentID: "inc-9",
		ActionID: ActionRestartDeployment, Decision: DecisionProposed,
	}
	if err := lib.Put(prop); err != nil {
		t.Fatal(err)
	}
	got, ok := lib.FindOpen("payments", "inc-9", ActionRestartDeployment)
	if !ok || got.ID != "ap-1" {
		t.Fatalf("%v %+v", ok, got)
	}
	prop.Applied = true
	prop.Decision = DecisionApplied
	_ = lib.Put(prop)
	if _, ok := lib.FindOpen("payments", "inc-9", ActionRestartDeployment); ok {
		t.Fatal("applied proposal should not be open")
	}
}

func TestAttachProposalToAlert(t *testing.T) {
	alert := incident.AgentAlert{Namespace: "payments", IncidentID: "inc-1"}
	prop := &Proposal{
		ID: "ap-9", Namespace: "payments", ActionID: ActionRollbackFailedRollout,
		Decision: DecisionProposed, Risk: "medium",
		ExpectedImpact: "Traffic returns to the last successful ReplicaSet",
	}
	AttachProposalToAlert(&alert, prop)
	if alert.ProposalID != "ap-9" || alert.ProposalAction == "" || alert.ProposalHint == "" {
		t.Fatalf("%+v", alert)
	}
	if alert.ProposalImpact == "" || alert.ProposalRisk != "medium" {
		t.Fatalf("impact/risk not stamped: %+v", alert)
	}
	if !strings.Contains(alert.ProposalHint, "--approve") {
		t.Fatalf("hint=%q", alert.ProposalHint)
	}
	denied := &Proposal{ID: "x", Decision: DecisionDenied}
	alert2 := incident.AgentAlert{}
	AttachProposalToAlert(&alert2, denied)
	if alert2.ProposalID != "" {
		t.Fatal("denied should not attach")
	}
}

func TestProposalLibraryCap(t *testing.T) {
	lib := NewProposalLibrary(NewMemProposalStore())
	for i := 0; i < ProposalsMax+5; i++ {
		p := Proposal{ID: fmt.Sprintf("ap-%d", i), Namespace: "ns", ActionID: ActionEvictPod}
		if err := lib.Put(p); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := lib.List("ns")
	if err != nil || len(snap.Proposals) != ProposalsMax {
		t.Fatalf("cap: len=%d err=%v", len(snap.Proposals), err)
	}
}
