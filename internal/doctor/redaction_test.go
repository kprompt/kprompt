package doctor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/team"
	"github.com/kprompt/kprompt/internal/tools"
)

func TestRunRedactsPulledSecretValues(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))
	t.Setenv("KPROMPT_OPENAI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	config.ResetPulledSecretsCache()
	t.Cleanup(config.ResetPulledSecretsCache)

	if _, err := config.SetField("provider", "openai"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".kprompt", "provider-secrets.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("openai: sk-from-pull\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config.ResetPulledSecretsCache()

	rep, err := Run(context.Background(), Options{
		Detect: func(context.Context, tools.DetectOptions) (*tools.Registry, error) {
			return tools.NewRegistry([]tools.Result{
				{ID: tools.IDKubernetes, Name: "Kubernetes", Status: tools.StatusAvailable, Detail: "ok"},
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

	var buf bytes.Buffer
	if err := FormatText(&buf, rep); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "sk-from-pull") {
		t.Fatalf("secret leaked into doctor output:\n%s", out)
	}
	if !strings.Contains(out, "provider key(s) cached") {
		t.Fatalf("expected cached secret summary, got:\n%s", out)
	}
}
