package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAPIKeyForPrefersEnvOverPulled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))
	t.Setenv("KPROMPT_OPENAI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	ResetPulledSecretsCache()

	path := filepath.Join(dir, ".kprompt", "provider-secrets.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("openai: sk-from-pull\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ResetPulledSecretsCache()
	if got := APIKeyFor("openai"); got != "sk-from-pull" {
		t.Fatalf("pulled key: got %q", got)
	}

	t.Setenv("KPROMPT_OPENAI_API_KEY", "sk-from-env")
	ResetPulledSecretsCache()
	if got := APIKeyFor("openai"); got != "sk-from-env" {
		t.Fatalf("env should win: got %q", got)
	}
}
