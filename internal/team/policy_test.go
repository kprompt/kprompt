package team

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPolicyParseErrorFailsClosed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))

	path, err := PolicyPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("max_risk: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, ok, err := LoadPolicy()
	if err == nil {
		t.Fatal("expected parse error")
	}
	if ok {
		t.Fatal("corrupted policy cache must fail closed")
	}
}

func TestLoadPolicyIgnoresUnknownLoosenKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))

	path, err := PolicyPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("org_id: acme\nversion: 7\nmax_risk: medium\nallow_wipe: true\nclear_hard_deny: true\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, ok, err := LoadPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected cached policy")
	}
	if got.OrgID != "acme" || got.Version != 7 || got.MaxRisk != "medium" {
		t.Fatalf("unexpected parsed policy: %+v", got)
	}
}
