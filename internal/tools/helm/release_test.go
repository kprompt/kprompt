package helm

import (
	"strings"
	"testing"
)

func TestUpgradeDiffLine(t *testing.T) {
	got := UpgradeDiffLine("nginx-14.0.0", "bitnami/nginx", "15.3.2")
	if !strings.Contains(got, "nginx-14.0.0") || !strings.Contains(got, "15.3.2") {
		t.Fatalf("diff=%s", got)
	}
}

func TestUpgradeDiffLineWithoutCurrent(t *testing.T) {
	got := UpgradeDiffLine("", "bitnami/nginx", "1.3")
	if !strings.Contains(got, "bitnami/nginx") {
		t.Fatalf("diff=%s", got)
	}
}
