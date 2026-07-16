package helm

import (
	"strings"
	"testing"
)

func TestPreviewInstallCommand(t *testing.T) {
	install := InstallCommand("redis", "bitnami/redis", "demo", "kind-test", 2)
	got, err := PreviewInstallCommand(install, "https://charts.bitnami.com/bitnami")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "helm template redis bitnami/redis") {
		t.Fatalf("cmd=%s", joined)
	}
	if strings.Contains(joined, "--create-namespace") {
		t.Fatalf("template should drop create-namespace: %s", joined)
	}
	if !strings.Contains(joined, "--repo https://charts.bitnami.com/bitnami") {
		t.Fatalf("cmd=%s", joined)
	}
}

func TestPreviewUpgradeCommand(t *testing.T) {
	upgrade := UpgradeCommand("nginx", "bitnami/nginx", "default", "", "15.3.2")
	got, err := PreviewUpgradeCommand(upgrade)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--dry-run=client") {
		t.Fatalf("cmd=%s", joined)
	}
}

func TestTruncatePreview(t *testing.T) {
	long := strings.Repeat("a", previewMaxBytes+10)
	got := TruncatePreview(long)
	if !strings.Contains(got, "truncated") {
		t.Fatal("expected truncation marker")
	}
}

func TestRepoURLFromCommand(t *testing.T) {
	cmd := RepoAddCommand("bitnami", "https://charts.bitnami.com/bitnami")
	if RepoURLFromCommand(cmd) != "https://charts.bitnami.com/bitnami" {
		t.Fatalf("url=%q", RepoURLFromCommand(cmd))
	}
}
