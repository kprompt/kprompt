package xdg

import (
	"path/filepath"
	"testing"
)

func TestConfigHomeXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/custom-xdg")
	if got := ConfigHome(); got != "/tmp/custom-xdg" {
		t.Fatalf("ConfigHome = %q, want /tmp/custom-xdg", got)
	}
}

func TestConfigHomeDefault(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/tester")
	got := ConfigHome()
	want := filepath.Join("/home/tester", ".config")
	if got != want {
		t.Fatalf("ConfigHome = %q, want %q", got, want)
	}
}

func TestKpromptDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := KpromptDir("memory"); got != filepath.Join("/xdg", "kprompt", "memory") {
		t.Fatalf("KpromptDir = %q", got)
	}
	if got := KpromptDir(); got != filepath.Join("/xdg", "kprompt") {
		t.Fatalf("KpromptDir() = %q", got)
	}
}
