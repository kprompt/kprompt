package helm

import "fmt"

// RepoAddCommand returns argv for helm repo add.
func RepoAddCommand(repoName, repoURL string) []string {
	return []string{"helm", "repo", "add", repoName, repoURL}
}

// InstallCommand returns argv for helm install.
func InstallCommand(release, chartRef, namespace, kubeContext string, replicaCount int32) []string {
	cmd := []string{
		"helm", "install", release, chartRef,
		"-n", namespace,
		"--create-namespace",
	}
	if kubeContext != "" {
		cmd = append(cmd, "--kube-context", kubeContext)
	}
	if replicaCount > 0 {
		cmd = append(cmd, "--set", fmt.Sprintf("replicaCount=%d", replicaCount))
	}
	return cmd
}

// RepoUpdateCommand returns argv for helm repo update.
func RepoUpdateCommand(repoName string) []string {
	return []string{"helm", "repo", "update", repoName}
}

// UpgradeCommand returns argv for helm upgrade.
func UpgradeCommand(release, chartRef, namespace, kubeContext, chartVersion string) []string {
	cmd := []string{
		"helm", "upgrade", release, chartRef,
		"-n", namespace,
	}
	if kubeContext != "" {
		cmd = append(cmd, "--kube-context", kubeContext)
	}
	if chartVersion != "" {
		cmd = append(cmd, "--version", chartVersion)
	}
	return cmd
}
