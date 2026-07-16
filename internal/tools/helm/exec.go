package helm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Run executes a helm argv slice (must start with "helm").
func Run(ctx context.Context, argv []string) error {
	_, err := RunCapture(ctx, argv)
	return err
}

// RunCapture executes helm and returns combined stdout/stderr on success.
func RunCapture(ctx context.Context, argv []string) (string, error) {
	if len(argv) == 0 || argv[0] != "helm" {
		return "", fmt.Errorf("invalid helm command")
	}
	path, err := exec.LookPath("helm")
	if err != nil {
		return "", fmt.Errorf("helm not on PATH: %w", err)
	}
	cmd := exec.CommandContext(ctx, path, argv[1:]...)
	out, err := cmd.CombinedOutput()
	body := strings.TrimSpace(string(out))
	if err != nil {
		if body == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, body)
	}
	return body, nil
}
