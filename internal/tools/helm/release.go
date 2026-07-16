package helm

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type helmListRow struct {
	Name  string `json:"name"`
	Chart string `json:"chart"`
}

// CurrentChartLabel returns the deployed chart label (e.g. nginx-15.3.2) for a release.
func CurrentChartLabel(release, namespace, kubeContext string) (string, error) {
	release = strings.TrimSpace(release)
	if release == "" {
		return "", fmt.Errorf("release name required")
	}
	if namespace == "" {
		namespace = "default"
	}
	path, err := exec.LookPath("helm")
	if err != nil {
		return "", err
	}
	args := []string{
		"list", "-n", namespace,
		"-f", fmt.Sprintf("^%s$", release),
		"-o", "json",
	}
	if kubeContext != "" {
		args = append(args, "--kube-context", kubeContext)
	}
	out, err := exec.Command(path, args...).Output()
	if err != nil {
		return "", err
	}
	var rows []helmListRow
	if err := json.Unmarshal(out, &rows); err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("release %q not found in namespace %s", release, namespace)
	}
	return strings.TrimSpace(rows[0].Chart), nil
}

// UpgradeDiffLine formats a before→after chart version summary for the plan.
func UpgradeDiffLine(currentChart, chartRef, targetVersion string) string {
	target := chartRef
	if targetVersion != "" {
		target = fmt.Sprintf("%s --version %s", chartRef, targetVersion)
	}
	if currentChart == "" {
		return fmt.Sprintf("chart → %s", target)
	}
	return fmt.Sprintf("chart %s → %s", currentChart, target)
}
