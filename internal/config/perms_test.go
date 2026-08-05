package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveFileUses0600AndRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))

	if err := SaveFile(File{
		Provider: "openai",
		Model:    "gpt-4o-mini",
	}); err != nil {
		t.Fatal(err)
	}

	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config perms = %04o, want 0600", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if strings.Contains(strings.ToLower(body), "api_key") || strings.Contains(body, "sk-") {
		t.Fatalf("secret leaked into config file:\n%s", body)
	}
}
