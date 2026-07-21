package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLookDashBinaryEnv(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "kprompt-dash")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KPROMPT_DASH_BIN", bin)
	got, err := lookDashBinary()
	if err != nil {
		t.Fatal(err)
	}
	if got != bin {
		t.Fatalf("got %q", got)
	}
}

func TestLookDashBinaryMissing(t *testing.T) {
	t.Setenv("KPROMPT_DASH_BIN", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("PATH", t.TempDir())
	_, err := lookDashBinary()
	if err == nil {
		t.Fatal("expected error")
	}
}
