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
