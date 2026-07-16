package helm

import (
	"errors"
	"strings"
)

const previewMaxBytes = 8192

var errInvalidPreview = errors.New("invalid helm command for preview")

// PreviewInstallCommand converts helm install argv into helm template for plan preview.
func PreviewInstallCommand(installCmd []string, repoURL string) ([]string, error) {
	if len(installCmd) < 4 || installCmd[0] != "helm" || installCmd[1] != "install" {
		return nil, errInvalidPreview
	}
	cmd := []string{"helm", "template", installCmd[2], installCmd[3]}
	for i := 4; i < len(installCmd); i++ {
		arg := installCmd[i]
		if arg == "--create-namespace" {
			continue
		}
		cmd = append(cmd, arg)
	}
	if repoURL != "" {
		cmd = append(cmd, "--repo", repoURL)
	}
	return cmd, nil
}

// PreviewUpgradeCommand appends client dry-run flags to helm upgrade argv.
func PreviewUpgradeCommand(upgradeCmd []string) ([]string, error) {
	if len(upgradeCmd) < 4 || upgradeCmd[0] != "helm" || upgradeCmd[1] != "upgrade" {
		return nil, errInvalidPreview
	}
	cmd := append([]string{}, upgradeCmd...)
	for _, flag := range []string{"--dry-run=client", "--hide-notes"} {
		if !containsFlag(cmd, flag) {
			cmd = append(cmd, flag)
		}
	}
	return cmd, nil
}

// TruncatePreview limits manifest preview size for terminal output.
func TruncatePreview(body string) string {
	body = strings.TrimSpace(body)
	if len(body) <= previewMaxBytes {
		return body
	}
	return body[:previewMaxBytes] + "\n…(preview truncated)"
}

func containsFlag(argv []string, flag string) bool {
	for _, a := range argv {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}

// RepoURLFromCommand extracts the URL from `helm repo add NAME URL`.
func RepoURLFromCommand(repoCmd []string) string {
	if len(repoCmd) >= 5 && repoCmd[0] == "helm" && repoCmd[1] == "repo" && repoCmd[2] == "add" {
		return repoCmd[4]
	}
	return ""
}
