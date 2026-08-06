package planner

import (
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildDeployRedis(t *testing.T) {
	in := intent.Intent{
		Kind: intent.KindDeploy,
		Target: intent.Target{
			Name:      "redis",
			Namespace: "demo",
		},
		Params: map[string]any{},
	}
	plan, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 2 {
		t.Fatalf("expected Deployment+Service, got %d actions", len(plan.Actions))
	}
	if plan.Actions[0].Object.Kind != "Deployment" {
		t.Fatalf("first action=%s", plan.Actions[0].Object.Kind)
	}
	if plan.Actions[1].Object.Kind != "Service" {
		t.Fatalf("second action=%s", plan.Actions[1].Object.Kind)
	}
	if plan.Actions[0].Manifest == "" {
		t.Fatal("missing deployment manifest")
	}
	if !plan.RequiresApproval {
		t.Fatal("deploy should require approval")
	}
}

func TestBuildDeployRequiresImage(t *testing.T) {
	in := intent.Intent{
		Kind:   intent.KindDeploy,
		Target: intent.Target{Name: "myapp"},
		Params: map[string]any{},
	}
	_, err := Build(in)
	if err == nil {
		t.Fatal("expected error for unknown app without image")
	}
}

func TestBuildDeployWithExplicitImage(t *testing.T) {
	in := intent.Intent{
		Kind:   intent.KindDeploy,
		Target: intent.Target{Name: "myapp", Namespace: "ns"},
		Params: map[string]any{
			"image":    "ghcr.io/example/app:1",
			"replicas": float64(2),
		},
	}
	plan, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("expected only Deployment without port, got %d", len(plan.Actions))
	}
}

func TestBuildScaleStatefulSet(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind: intent.KindScale,
		Target: intent.Target{
			Kind:      "sts",
			Name:      "redis",
			Namespace: "demo",
		},
		Params: map[string]any{"replicas": float64(3)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresApproval {
		t.Fatal("scale should require approval")
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("expected one action, got %d", len(plan.Actions))
	}
	action := plan.Actions[0]
	if action.Op != OpScale {
		t.Fatalf("op=%s", action.Op)
	}
	if action.Object.Kind != "StatefulSet" {
		t.Fatalf("kind=%s", action.Object.Kind)
	}
	if action.Object.Name != "redis" || action.Object.Namespace != "demo" {
		t.Fatalf("object=%+v", action.Object)
	}
	if action.Replicas == nil || *action.Replicas != 3 {
		t.Fatalf("replicas=%v", action.Replicas)
	}
	if plan.Summary == "" || !strings.Contains(plan.Summary, "StatefulSet/redis") {
		t.Fatalf("summary=%q", plan.Summary)
	}
}

func TestBuildGetPods(t *testing.T) {
	in := intent.Intent{
		Kind: intent.KindGet,
		Target: intent.Target{
			Kind:      "pods",
			Namespace: "demo",
		},
		Params: map[string]any{"minMemory": "2Gi"},
	}
	plan, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval {
		t.Fatal("get must not require approval")
	}
	if plan.Actions[0].Object.Kind != "Pod" {
		t.Fatalf("kind=%s", plan.Actions[0].Object.Kind)
	}
}

func TestBuildGetGenericResources(t *testing.T) {
	cases := []struct {
		kind      string
		ns        string
		wantKind  string
		wantNS    string
		wantGroup string
	}{
		{"Node", "default", "Node", "", ""},
		{"ConfigMap", "demo", "ConfigMap", "demo", ""},
		{"Secret", "demo", "Secret", "demo", ""},
		{"deployments.apps", "prod", "Deployment", "prod", "apps"},
		{"widgets.example.com", "demo", "Widget", "demo", "example.com"},
	}
	for _, tc := range cases {
		plan, err := Build(intent.Intent{
			Kind:   intent.KindGet,
			Target: intent.Target{Kind: tc.kind, Namespace: tc.ns},
		})
		if err != nil {
			t.Fatalf("%s: %v", tc.kind, err)
		}
		if plan.RequiresApproval {
			t.Fatalf("%s: get must not require approval", tc.kind)
		}
		if plan.Actions[0].Object.Kind != tc.wantKind {
			t.Fatalf("%s: kind=%s want %s", tc.kind, plan.Actions[0].Object.Kind, tc.wantKind)
		}
		if plan.Actions[0].Object.Namespace != tc.wantNS {
			t.Fatalf("%s: ns=%q want %q", tc.kind, plan.Actions[0].Object.Namespace, tc.wantNS)
		}
		if tc.wantGroup != "" {
			g, _ := plan.Intent.StringParam("group")
			if g != tc.wantGroup {
				t.Fatalf("%s: group=%q", tc.kind, g)
			}
		}
		if plan.Actions[0].Backend != "kubernetes" {
			t.Fatalf("%s: backend=%s", tc.kind, plan.Actions[0].Backend)
		}
	}
}

func TestBuildGetRejectsClusterScopedNamespace(t *testing.T) {
	_, err := Build(intent.Intent{
		Kind:   intent.KindGet,
		Target: intent.Target{Kind: "Node", Namespace: "kube-system"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildExplainRequiresName(t *testing.T) {
	_, err := Build(intent.Intent{Kind: intent.KindExplain, Target: intent.Target{}})
	if err == nil {
		t.Fatal("expected error")
	}
	plan, err := Build(intent.Intent{
		Kind:   intent.KindExplain,
		Target: intent.Target{Name: "payment-api", Namespace: "prod", Kind: "Deployment"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval {
		t.Fatal("explain is read-only")
	}
}

func TestBuildInvestigate(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindInvestigate,
		Target: intent.Target{Name: "api", Namespace: "payments", Kind: "Deployment"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval {
		t.Fatal("investigate is read-only")
	}
	if !strings.Contains(plan.Summary, "Service→Endpoints") {
		t.Fatalf("summary: %s", plan.Summary)
	}
}

func TestBuildWhy(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindWhy,
		Target: intent.Target{Name: "ledger", Namespace: "payments", Kind: "Pod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval {
		t.Fatal("why is read-only")
	}
	if !strings.Contains(plan.Summary, "symptom → proximate → root") {
		t.Fatalf("summary: %s", plan.Summary)
	}
}

func TestBuildTimeline(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindTimeline,
		Target: intent.Target{Name: "api", Namespace: "payments", Kind: "Deployment"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval {
		t.Fatal("timeline is read-only")
	}
	if !strings.Contains(plan.Summary, "Events→ReplicaSets→HPA") {
		t.Fatalf("summary: %s", plan.Summary)
	}
}

func TestBuildImpact(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindImpact,
		Target: intent.Target{Name: "api", Namespace: "payments", Kind: "Service"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval || len(plan.Actions) != 1 {
		t.Fatalf("plan=%+v", plan)
	}
	if got := plan.Actions[0].Object; got.Kind != "Service" || got.Name != "api" || got.Namespace != "payments" {
		t.Fatalf("object=%+v", got)
	}
}

func TestBuildAudit(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindAudit,
		Target: intent.Target{Namespace: "payments", Kind: "Namespace"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval || len(plan.Actions) != 1 || plan.Actions[0].Op != OpAudit {
		t.Fatalf("plan=%+v", plan)
	}
	if got := plan.Actions[0].Object; got.Namespace != "payments" {
		t.Fatalf("object=%+v", got)
	}
	clusterPlan, err := Build(intent.Intent{
		Kind:   intent.KindAudit,
		Target: intent.Target{Kind: "Cluster"},
		Params: map[string]any{"scope": "cluster"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if clusterPlan.Actions[0].Object.Namespace != "" {
		t.Fatalf("cluster scope should clear namespace: %+v", clusterPlan.Actions[0].Object)
	}
}

func TestBuildCleanup(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindCleanup,
		Target: intent.Target{Namespace: "payments", Kind: "Namespace"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval || len(plan.Actions) != 1 || plan.Actions[0].Op != OpCleanup {
		t.Fatalf("plan=%+v", plan)
	}
	if got := plan.Actions[0].Object; got.Namespace != "payments" {
		t.Fatalf("object=%+v", got)
	}
	clusterPlan, err := Build(intent.Intent{
		Kind:   intent.KindCleanup,
		Target: intent.Target{Kind: "Cluster"},
		Params: map[string]any{"scope": "cluster"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if clusterPlan.Actions[0].Object.Namespace != "" {
		t.Fatalf("cluster scope should clear namespace: %+v", clusterPlan.Actions[0].Object)
	}
}

func TestBuildSearch(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindSearch,
		Target: intent.Target{Namespace: "payments", Kind: "Deployment"},
		Params: map[string]any{"query": "redis"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval || len(plan.Actions) != 1 || plan.Actions[0].Op != OpSearch {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Actions[0].Object.Namespace != "payments" || plan.Actions[0].Object.Kind != "Deployment" {
		t.Fatalf("object=%+v", plan.Actions[0].Object)
	}
	if _, err := Build(intent.Intent{Kind: intent.KindSearch, Target: intent.Target{Namespace: "payments"}}); err == nil {
		t.Fatal("expected error without query")
	}
}

func TestBuildScore(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindScore,
		Target: intent.Target{Namespace: "payments", Kind: "Namespace"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval || len(plan.Actions) != 1 || plan.Actions[0].Op != OpScore {
		t.Fatalf("plan=%+v", plan)
	}
	clusterPlan, err := Build(intent.Intent{
		Kind:   intent.KindScore,
		Target: intent.Target{Kind: "Cluster"},
		Params: map[string]any{"scope": "cluster"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if clusterPlan.Actions[0].Object.Namespace != "" {
		t.Fatalf("cluster scope should clear namespace: %+v", clusterPlan.Actions[0].Object)
	}
}

func TestBuildArchitecture(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindArchitecture,
		Target: intent.Target{Namespace: "payments", Kind: "Namespace"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval || len(plan.Actions) != 1 || plan.Actions[0].Op != OpArchitecture {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestBuildLearn(t *testing.T) {
	plan, err := Build(intent.Intent{Kind: intent.KindLearn})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval || len(plan.Actions) != 1 || plan.Actions[0].Op != OpLearn {
		t.Fatalf("plan=%+v", plan)
	}
	if plan.Actions[0].Object.Kind != "Cluster" {
		t.Fatalf("object=%+v", plan.Actions[0].Object)
	}
}

func TestBuildDrift(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindDrift,
		Target: intent.Target{Namespace: "flux-system", Kind: "Namespace"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RequiresApproval || len(plan.Actions) != 1 || plan.Actions[0].Op != OpDrift {
		t.Fatalf("plan=%+v", plan)
	}
	clusterPlan, err := Build(intent.Intent{
		Kind:   intent.KindDrift,
		Target: intent.Target{Kind: "Cluster"},
		Params: map[string]any{"scope": "cluster"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if clusterPlan.Actions[0].Object.Namespace != "" {
		t.Fatalf("cluster scope should clear namespace: %+v", clusterPlan.Actions[0].Object)
	}
}

func TestBuildRollback(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindRollback,
		Target: intent.Target{Name: "payment-api", Namespace: "prod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresApproval {
		t.Fatal("rollback should require approval")
	}
	if len(plan.Actions) != 1 || plan.Actions[0].Op != OpRollback {
		t.Fatalf("actions=%v", plan.Actions)
	}
	if plan.Actions[0].Revision != nil {
		t.Fatal("default rollback should not set revision")
	}

	plan, err = Build(intent.Intent{
		Kind:   intent.KindRollback,
		Target: intent.Target{Name: "payment-api", Namespace: "prod"},
		Params: map[string]any{"revision": float64(17)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Actions[0].Revision == nil || *plan.Actions[0].Revision != 17 {
		t.Fatalf("revision=%v", plan.Actions[0].Revision)
	}
}

func TestBuildRollbackRequiresName(t *testing.T) {
	_, err := Build(intent.Intent{Kind: intent.KindRollback, Target: intent.Target{}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildLogsAndDescribe(t *testing.T) {
	logs, err := Build(intent.Intent{
		Kind:   intent.KindLogs,
		Target: intent.Target{Name: "api", Namespace: "prod", Kind: "Deployment"},
		Params: map[string]any{"tail": float64(50), "container": "app"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if logs.RequiresApproval {
		t.Fatal("logs is read-only")
	}
	if !strings.Contains(logs.Summary, "50") {
		t.Fatalf("summary=%s", logs.Summary)
	}

	desc, err := Build(intent.Intent{
		Kind:   intent.KindDescribe,
		Target: intent.Target{Name: "api", Namespace: "prod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if desc.RequiresApproval {
		t.Fatal("describe is read-only")
	}
}

func TestBuildDelete(t *testing.T) {
	plan, err := Build(intent.Intent{
		Kind:   intent.KindDelete,
		Target: intent.Target{Name: "redis", Namespace: "demo", Kind: "Deployment"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresApproval {
		t.Fatal("delete requires approval")
	}
	if plan.Actions[0].Op != OpDelete {
		t.Fatalf("op=%s", plan.Actions[0].Op)
	}
}

func TestBuildDeleteRejectsUnscoped(t *testing.T) {
	_, err := Build(intent.Intent{
		Kind:   intent.KindDelete,
		Target: intent.Target{Name: "all", Kind: "Deployment"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = Build(intent.Intent{
		Kind:   intent.KindDelete,
		Target: intent.Target{Name: "prod", Kind: "Namespace"},
	})
	if err == nil {
		t.Fatal("expected error for Namespace")
	}
}

func TestBuildDeleteJobAndReplicaSet(t *testing.T) {
	// 1. Test Job deletion
	plan, err := Build(intent.Intent{
		Kind:   intent.KindDelete,
		Target: intent.Target{Name: "my-job", Namespace: "default", Kind: "Job"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresApproval {
		t.Fatal("Job delete requires approval")
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(plan.Actions))
	}
	a := plan.Actions[0]
	if a.Op != OpDelete || a.Object.Kind != "Job" || a.Object.APIVersion != "batch/v1" {
		t.Fatalf("unexpected action: %+v", a)
	}

	// 2. Test ReplicaSet deletion
	plan, err = Build(intent.Intent{
		Kind:   intent.KindDelete,
		Target: intent.Target{Name: "my-rs", Namespace: "default", Kind: "ReplicaSet"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.RequiresApproval {
		t.Fatal("ReplicaSet delete requires approval")
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(plan.Actions))
	}
	a = plan.Actions[0]
	if a.Op != OpDelete || a.Object.Kind != "ReplicaSet" || a.Object.APIVersion != "apps/v1" {
		t.Fatalf("unexpected action: %+v", a)
	}
}

func TestBuildDeleteUnsupportedKind(t *testing.T) {
	_, err := Build(intent.Intent{
		Kind:   intent.KindDelete,
		Target: intent.Target{Name: "my-config", Kind: "ConfigMap"},
	})
	if err == nil {
		t.Fatal("expected error for ConfigMap deletion")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildUnknownIntent(t *testing.T) {
	_, err := Build(intent.Intent{Kind: intent.KindUnknown, Target: intent.Target{Name: "redis"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown intent") {
		t.Fatalf("got %v", err)
	}
}
