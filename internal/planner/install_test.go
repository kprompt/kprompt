package planner

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildInstallRedis(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	plan, err := Build(intent.Intent{
		Kind:   intent.KindInstall,
		Target: intent.Target{Name: "redis", Namespace: "demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 2 {
		t.Fatalf("expected 2 helm actions, got %d", len(plan.Actions))
	}
	if plan.Actions[0].Op != OpHelmRepo || plan.Actions[1].Op != OpHelmInstall {
		t.Fatalf("ops=%s %s", plan.Actions[0].Op, plan.Actions[1].Op)
	}
	cmd := strings.Join(plan.Actions[1].Command, " ")
	if !strings.Contains(cmd, "bitnami/redis") {
		t.Fatalf("cmd=%s", cmd)
	}
	if !plan.RequiresApproval {
		t.Fatal("install should require approval")
	}
}

func TestBuildInstallWithoutHelm(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := Build(intent.Intent{
		Kind:   intent.KindInstall,
		Target: intent.Target{Name: "redis"},
	})
	if err == nil {
		t.Fatal("expected error when helm missing")
	}
}
