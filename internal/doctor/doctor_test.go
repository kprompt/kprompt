package doctor

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/team"
	"github.com/kprompt/kprompt/internal/tools"
)

func TestRunLLMKeyRequired(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))
	t.Setenv("KPROMPT_OPENAI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	rep, err := Run(context.Background(), Options{
		Detect: func(context.Context, tools.DetectOptions) (*tools.Registry, error) {
			return tools.NewRegistry([]tools.Result{
				{ID: tools.IDKubernetes, Name: "Kubernetes", Status: tools.StatusAvailable, Detail: "context: kind"},
				{ID: tools.IDHelm, Name: "Helm", Status: tools.StatusUnavailable, Detail: "not on PATH"},
			}), nil
		},
		Me: func(context.Context, string, string) (team.MeResponse, error) {
			t.Fatal("me should not be called")
			return team.MeResponse{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatal("expected fail without API key")
	}
	llm := find(rep, "llm")
	if llm.Status != Fail || !strings.Contains(llm.Hint, "KPROMPT_OPENAI_API_KEY") {
		t.Fatalf("llm=%+v", llm)
	}
	kube := find(rep, "kubernetes")
	if kube.Status != Pass {
		t.Fatalf("kube=%+v", kube)
	}
	helm := find(rep, "helm")
	if helm.Status != Warn {
		t.Fatalf("helm=%+v", helm)
	}
	teamChk := find(rep, "team")
	if teamChk.Status != Skip {
		t.Fatalf("team=%+v", teamChk)
	}

	var buf bytes.Buffer
	if err := FormatText(&buf, rep); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(strings.ToLower(out), "sk-") {
		t.Fatalf("secret leaked:\n%s", out)
	}
	if !strings.Contains(out, "Overall: FAIL") {
		t.Fatalf("out=%s", out)
	}
}

func TestRunOKWithOllama(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))
	t.Setenv("KPROMPT_OPENAI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	if _, err := config.SetField("provider", "ollama"); err != nil {
		t.Fatal(err)
	}

	rep, err := Run(context.Background(), Options{
		Detect: func(context.Context, tools.DetectOptions) (*tools.Registry, error) {
			return tools.NewRegistry([]tools.Result{
				{ID: tools.IDKubernetes, Name: "Kubernetes", Status: tools.StatusAvailable, Detail: "ok"},
				{ID: tools.IDHelm, Name: "Helm", Status: tools.StatusAvailable, Detail: "v3"},
			}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("expected ok: %+v", rep.Checks)
	}
	if find(rep, "llm").Status != Pass {
		t.Fatalf("llm=%+v", find(rep, "llm"))
	}
}

func find(rep Report, id string) Check {
	for _, c := range rep.Checks {
		if c.ID == id {
			return c
		}
	}
	return Check{}
}
