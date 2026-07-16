package helm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Run executes a helm argv slice (must start with "helm").
func Run(ctx context.Context, argv []string) error {
	if len(argv) == 0 || argv[0] != "helm" {
		return fmt.Errorf("invalid helm command")
	}
	path, err := exec.LookPath("helm")
	if err != nil {
		return fmt.Errorf("helm not on PATH: %w", err)
	}
	cmd := exec.CommandContext(ctx, path, argv[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}
