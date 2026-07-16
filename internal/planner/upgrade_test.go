package planner

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestBuildUpgradeNginx(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	plan, err := Build(intent.Intent{
		Kind: intent.KindUpgrade,
		Target: intent.Target{
			Name:      "nginx",
			Namespace: "demo",
		},
		Params: map[string]any{"version": "15.3.2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 3 {
		t.Fatalf("expected 3 helm actions, got %d", len(plan.Actions))
	}
	if plan.Actions[2].Op != OpHelmUpgrade {
		t.Fatalf("last op=%s", plan.Actions[2].Op)
	}
	cmd := strings.Join(plan.Actions[2].Command, " ")
	if !strings.Contains(cmd, "--version 15.3.2") {
		t.Fatalf("cmd=%s", cmd)
	}
	if plan.Actions[2].Diff == "" {
		t.Fatal("expected version diff on upgrade action")
	}
}

func TestBuildUpgradeRequiresVersion(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	_, err := Build(intent.Intent{
		Kind:   intent.KindUpgrade,
		Target: intent.Target{Name: "nginx"},
	})
	if err == nil {
		t.Fatal("expected error without version")
	}
}
