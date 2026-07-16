package helm

import (
	"strings"
	"testing"
)

func TestInstallCommandRedis(t *testing.T) {
	cmd := InstallCommand("redis", "bitnami/redis", "demo", "", 0)
	joined := strings.Join(cmd, " ")
	if !strings.Contains(joined, "helm install redis bitnami/redis") {
		t.Fatalf("cmd=%s", joined)
	}
	if !strings.Contains(joined, "-n demo") {
		t.Fatalf("cmd=%s", joined)
	}
}

func TestInstallCommandReplicas(t *testing.T) {
	cmd := InstallCommand("redis", "bitnami/redis", "default", "kind-test", 3)
	joined := strings.Join(cmd, " ")
	if !strings.Contains(joined, "replicaCount=3") || !strings.Contains(joined, "--kube-context kind-test") {
		t.Fatalf("cmd=%s", joined)
	}
}

func TestRepoAddCommand(t *testing.T) {
	cmd := RepoAddCommand("bitnami", "https://charts.bitnami.com/bitnami")
	if strings.Join(cmd, " ") != "helm repo add bitnami https://charts.bitnami.com/bitnami" {
		t.Fatalf("cmd=%v", cmd)
	}
}
