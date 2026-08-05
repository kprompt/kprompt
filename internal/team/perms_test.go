package team

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveCredentialsUses0600(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))

	if err := SaveCredentials(Credentials{
		APIURL:   "https://api.kprompt.ai",
		APIToken: "team-token-123",
	}); err != nil {
		t.Fatal(err)
	}

	path, err := CredentialsPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("credentials perms = %04o, want 0600", got)
	}
}

func TestSaveProviderSecretsUses0600(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))

	if err := SaveProviderSecrets(map[string]string{
		"openai": "sk-from-pull",
	}); err != nil {
		t.Fatal(err)
	}

	path, err := ProviderSecretsPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("provider secrets perms = %04o, want 0600", got)
	}
}
