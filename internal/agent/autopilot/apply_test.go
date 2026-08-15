package autopilot

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/planner"
)

func autoPolicy() Policy {
	p := Policy{
		Allow:         append([]string(nil), MVPAllowlist...),
		Mode:          ModePolicyAuto,
		Apply:         true,
		MinConfidence: DefaultMinConfidence,
	}
	p.Normalize()
	return p
}

func TestApplyProposalNilEngine(t *testing.T) {
	var e *Engine
	if _, err := e.ApplyProposal(context.Background(), nil, Proposal{}); err == nil {
		t.Fatal("expected nil engine error")
	}
}

func TestApplyProposalHardDeny(t *testing.T) {
	eng := &Engine{Policy: autoPolicy(), Audit: &MemAudit{}}
	prop := Proposal{ActionID: "wipeCluster", Namespace: "ns", TargetName: "x", Confidence: 0.99}
	out, err := eng.ApplyProposal(context.Background(), nil, prop)
	if err == nil || out.Decision != DecisionDenied {
		t.Fatalf("expected hard-deny, got %+v err=%v", out, err)
	}
}

func TestApplyProposalConfidenceFloor(t *testing.T) {
	eng := &Engine{Policy: autoPolicy(), Audit: &MemAudit{}}
	prop := Proposal{ActionID: ActionRestartDeployment, Namespace: "ns", TargetName: "api", Confidence: 0.1}
	out, err := eng.ApplyProposal(context.Background(), nil, prop)
	if err == nil || out.Decision != DecisionDenied {
		t.Fatalf("expected confidence deny, got %+v err=%v", out, err)
	}
}

func TestApplyProposalMissingTarget(t *testing.T) {
	eng := &Engine{Policy: autoPolicy(), Audit: &MemAudit{}}
	prop := Proposal{ActionID: ActionRestartDeployment, Namespace: "ns", TargetName: "", Confidence: 0.99}
	out, err := eng.ApplyProposal(context.Background(), nil, prop)
	if err == nil || out.Decision != DecisionDenied {
		t.Fatalf("expected missing target deny, got %+v err=%v", out, err)
	}
}

func TestApplyProposalNilClient(t *testing.T) {
	eng := &Engine{Policy: autoPolicy(), Audit: &MemAudit{}}
	prop := Proposal{ActionID: ActionRestartDeployment, Namespace: "ns", TargetName: "api", Confidence: 0.99}
	if _, err := eng.ApplyProposal(context.Background(), nil, prop); err == nil {
		t.Fatal("expected nil kube client error")
	}
}

func TestApplyProposalRestartMissingDeployment(t *testing.T) {
	eng := &Engine{Policy: autoPolicy(), Audit: &MemAudit{}}
	client := fake.NewSimpleClientset()
	prop := Proposal{ActionID: ActionRestartDeployment, Namespace: "ns", TargetName: "api", Confidence: 0.99}
	out, err := eng.ApplyProposal(context.Background(), client, prop)
	if err == nil || out.Decision != DecisionFailed {
		t.Fatalf("expected failed on missing deployment, got %+v err=%v", out, err)
	}
}

func TestApplyProposalRestartExisting(t *testing.T) {
	eng := &Engine{Policy: autoPolicy(), Audit: &MemAudit{}}
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "ns"},
	})
	prop := Proposal{ActionID: ActionRestartDeployment, Namespace: "ns", TargetName: "api", Confidence: 0.99}
	out, _ := eng.ApplyProposal(context.Background(), client, prop)
	if out == nil {
		t.Fatal("expected proposal result")
	}
	// The restart annotation must have been written regardless of verify outcome.
	dep, err := client.AppsV1().Deployments("ns").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	if dep.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatal("expected restartedAt annotation")
	}
}

func TestRestartDeployment(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ns"},
	})
	if err := restartDeployment(context.Background(), client, "ns", "web"); err != nil {
		t.Fatalf("restartDeployment: %v", err)
	}
	if err := restartDeployment(context.Background(), client, "ns", "missing"); err == nil {
		t.Fatal("expected error for missing deployment")
	}
}

func TestFinalizeAfterMutateSkippedMarksApplied(t *testing.T) {
	// nil client → AttachVerify Skipped → default branch of finalizeAfterMutate.
	prop := &Proposal{
		ActionID:   ActionRestartDeployment,
		Namespace:  "ns",
		TargetName: "api",
	}
	if err := finalizeAfterMutate(context.Background(), nil, prop); err != nil {
		t.Fatal(err)
	}
	if !prop.Applied || prop.Decision != DecisionApplied {
		t.Fatalf("expected applied under skipped verify, got %+v", prop)
	}
	if !strings.Contains(prop.Reason, "verify") {
		t.Fatalf("reason=%q", prop.Reason)
	}
}

func TestFinalizeAfterMutateFailed(t *testing.T) {
	prop := &Proposal{
		ActionID:   ActionRollbackFailedRollout,
		Namespace:  "ns",
		TargetName: "missing-api",
		TargetKind: "Deployment",
		Plan:       PlanBody{Summary: "rollback"},
	}
	client := fake.NewSimpleClientset()
	err := finalizeAfterMutate(context.Background(), client, prop)
	if err == nil {
		t.Fatal("expected verify failure for missing deployment")
	}
	if prop.Decision != DecisionFailed || prop.Applied {
		t.Fatalf("expected failed/cleared, got %+v", prop)
	}
}

func TestProposalToPlan(t *testing.T) {
	rep := int32(3)
	cases := []struct {
		name    string
		prop    Proposal
		wantOp  planner.Op
		wantErr bool
	}{
		{"rollback", Proposal{ActionID: ActionRollbackFailedRollout, Namespace: "ns", TargetName: "api"}, planner.OpRollback, false},
		{"scale", Proposal{ActionID: ActionScaleDeployment, Namespace: "ns", TargetName: "api", Replicas: &rep}, planner.OpScale, false},
		{"evict", Proposal{ActionID: ActionEvictPod, Namespace: "ns", TargetName: "pod-1"}, planner.OpDelete, false},
		{"scale-no-replicas", Proposal{ActionID: ActionScaleDeployment, Namespace: "ns", TargetName: "api"}, "", true},
		{"unsupported", Proposal{ActionID: "somethingElse", Namespace: "ns", TargetName: "api"}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := proposalToPlan(tc.prop)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(plan.Actions) != 1 || plan.Actions[0].Op != tc.wantOp {
				t.Fatalf("unexpected plan: %+v", plan)
			}
		})
	}
}
