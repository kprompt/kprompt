package autopilot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/graph"
	"github.com/kprompt/kprompt/internal/incident"
)

func TestFileAuditAppend(t *testing.T) {
	dir := t.TempDir()
	a := FileAudit{Dir: dir}
	if err := a.Append(AuditEntry{Proposal: Proposal{ID: "ap-1"}}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "autopilot-audit.jsonl")); err != nil {
		t.Fatalf("expected audit file: %v", err)
	}
}

func TestDefaultAuditDirEnv(t *testing.T) {
	t.Setenv("KPROMPT_AUTOPILOT_AUDIT_DIR", "/tmp/kprompt-audit-test")
	if got := DefaultAuditDir(); got != "/tmp/kprompt-audit-test" {
		t.Fatalf("DefaultAuditDir=%q", got)
	}
	// Without the env override it must return a non-empty default path.
	t.Setenv("KPROMPT_AUTOPILOT_AUDIT_DIR", "")
	if got := DefaultAuditDir(); got == "" {
		t.Fatal("expected non-empty default audit dir")
	}
}

func TestIncidentConfidence(t *testing.T) {
	if got := IncidentConfidence(incident.Incident{Confidence: 0.42}, 0.9); got != 0.42 {
		t.Fatalf("got %v", got)
	}
	if got := IncidentConfidence(incident.Incident{}, 0.7); got != 0.7 {
		t.Fatalf("fallback got %v", got)
	}
}

func TestAttachGraphImpact(t *testing.T) {
	// No graph -> no-op.
	eng := &Engine{}
	prop := Proposal{Namespace: "payments", TargetKind: "Deployment", TargetName: "api"}
	eng.attachGraphImpact(&prop)
	if prop.ExpectedImpact != "" {
		t.Fatalf("expected empty impact without graph, got %q", prop.ExpectedImpact)
	}

	rep := graph.Report{
		Nodes: []graph.Node{
			{ID: "svc-api", Kind: graph.NodeService, Name: "api", Namespace: "payments"},
			{ID: "ext-db", Kind: graph.NodeExternalHost, Name: "db.example.com"},
		},
		Edges: []graph.Edge{
			{From: "svc-api", To: "ext-db", Type: graph.EdgeDependsOn, Source: graph.SourceKubernetes},
		},
	}
	eng2 := &Engine{Graph: &rep}
	prop2 := Proposal{Namespace: "payments", TargetKind: "Deployment", TargetName: "api"}
	eng2.attachGraphImpact(&prop2)
	if prop2.ExpectedImpact == "" {
		t.Fatal("expected impact note from depends_on edge")
	}
	// Second call appends to an existing ExpectedImpact.
	eng2.attachGraphImpact(&prop2)
}

func TestApplyProposalScaleExecutor(t *testing.T) {
	eng := &Engine{Policy: autoPolicy(), Audit: &MemAudit{}}
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "ns"},
	})
	reps := int32(3)
	prop := Proposal{
		ActionID: ActionScaleDeployment, Namespace: "ns", TargetName: "api",
		Confidence: 0.99, Replicas: &reps, Plan: PlanBody{Summary: "scale api to 3"},
	}
	out, _ := eng.ApplyProposal(context.Background(), client, prop)
	if out == nil {
		t.Fatal("expected proposal result for scale")
	}
}

func TestApplyProposalRollbackExecutor(t *testing.T) {
	eng := &Engine{Policy: autoPolicy(), Audit: &MemAudit{}}
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "ns"},
	})
	prop := Proposal{
		ActionID: ActionRollbackFailedRollout, Namespace: "ns", TargetName: "api",
		Confidence: 0.99, Plan: PlanBody{Summary: "rollback api"},
	}
	out, _ := eng.ApplyProposal(context.Background(), client, prop)
	if out == nil {
		t.Fatal("expected proposal result for rollback")
	}
}
