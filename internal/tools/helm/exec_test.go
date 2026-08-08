package helm

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunCaptureRejectsNonHelmEntryPoint(t *testing.T) {
	_, err := RunCapture(context.Background(), []string{"helm install redis bitnami/redis"})
	if err == nil || !strings.Contains(err.Error(), "invalid helm command") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunCaptureDoesNotUseShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping shell script test on Windows")
	}
	tmp := t.TempDir()
	dummyHelm := filepath.Join(tmp, "helm")
	script := `#!/bin/sh
for arg in "$@"; do
  echo "ARG: $arg"
done
`
	if err := os.WriteFile(dummyHelm, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp+":"+os.Getenv("PATH"))

	// Run with an argument containing shell metacharacters
	out, err := RunCapture(context.Background(), []string{"helm", "install", "foo; echo PWNED"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\nPWNED") || out == "PWNED" {
		t.Fatalf("shell injection succeeded! output: %q", out)
	}
	if !strings.Contains(out, "ARG: foo; echo PWNED") {
		t.Fatalf("expected literal argument to be passed, got: %q", out)
	}
}
