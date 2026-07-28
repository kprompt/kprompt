package gitopspr

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
)

func TestLoadSettingsEnvWins(t *testing.T) {
	t.Setenv(EnvMode, "pr")
	t.Setenv(EnvRepo, "acme/infra")
	t.Setenv(EnvPath, "apps/demo")
	s := LoadSettings(config.File{GitOps: config.GitOpsFile{Mode: "apply", Repo: "other/repo"}})
	if !s.Enabled() || s.Repo != "acme/infra" || s.Path != "apps/demo" {
		t.Fatalf("%+v", s)
	}
}

func TestValidateConnected(t *testing.T) {
	s := Settings{Mode: ModePR}
	if err := s.ValidateConnected(); err == nil || !strings.Contains(err.Error(), "SCM repo") {
		t.Fatalf("err=%v", err)
	}
	s.Repo = "bad"
	if err := s.ValidateConnected(); err == nil {
		t.Fatal("expected invalid repo")
	}
	s.Repo = "acme/infra"
	if err := s.ValidateConnected(); err != nil {
		t.Fatal(err)
	}
}

func TestFilesFromPlanDeploy(t *testing.T) {
	plan := planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindDeploy},
		Actions: []planner.Action{{
			Op:       planner.OpCreate,
			Backend:  "kubernetes",
			Object:   planner.ObjectRef{Kind: "Deployment", Name: "api", Namespace: "demo"},
			Manifest: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n",
		}},
	}
	files, err := FilesFromPlan(context.Background(), plan, "clusters/dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "clusters/dev/deployment-api.yaml" {
		t.Fatalf("%+v", files)
	}
}

func TestFilesFromPlanRejectsScale(t *testing.T) {
	plan := planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindScale},
		Actions: []planner.Action{{
			Op: planner.OpScale,
			Object: planner.ObjectRef{Kind: "Deployment", Name: "api"},
		}},
	}
	_, err := FilesFromPlan(context.Background(), plan, "kprompt")
	if err == nil || !strings.Contains(err.Error(), "supports deploy") {
		t.Fatalf("err=%v", err)
	}
}

func TestOpenFromPlanMemRunner(t *testing.T) {
	mem := &MemRunner{}
	plan := planner.ExecutionPlan{
		Intent:  intent.Intent{Kind: intent.KindDeploy},
		Summary: "Deploy api",
		Actions: []planner.Action{{
			Op:       planner.OpCreate,
			Object:   planner.ObjectRef{Kind: "Deployment", Name: "api"},
			Manifest: "kind: Deployment\n",
		}},
	}
	res, err := OpenFromPlan(context.Background(), plan, Options{
		Settings: Settings{Mode: ModePR, Repo: "acme/infra", Path: "apps", BaseBranch: "main"},
		Prompt:   "deploy api",
		Now:      time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC),
		Runner:   mem,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.URL == "" || res.Type != "gitops-pr" {
		t.Fatalf("%+v", res)
	}
	if mem.LastTarget.Repo != "acme/infra" || !strings.HasPrefix(mem.LastTarget.Branch, "kprompt/deploy-") {
		t.Fatalf("%+v", mem.LastTarget)
	}
	if !strings.Contains(mem.LastBody, "instead of applying") {
		t.Fatalf("body=%q", mem.LastBody)
	}
	if strings.Contains(mem.LastBody, "T-072") {
		t.Fatalf("body contains internal ticket ID: %q", mem.LastBody)
	}
}

func TestOpenFromPlanRequiresRepo(t *testing.T) {
	_, err := OpenFromPlan(context.Background(), planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindDeploy},
		Actions: []planner.Action{{
			Op: planner.OpCreate, Manifest: "kind: Deployment\n",
			Object: planner.ObjectRef{Kind: "Deployment", Name: "x"},
		}},
	}, Options{Settings: Settings{Mode: ModePR}})
	if err == nil || !strings.Contains(err.Error(), "SCM repo") {
		t.Fatalf("err=%v", err)
	}
}
