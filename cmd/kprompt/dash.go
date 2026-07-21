package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newDashCmd() *cobra.Command {
	var (
		addr    string
		kubeCtx string
		openUI  bool
	)
	cmd := &cobra.Command{
		Use:   "dash",
		Short: "Open the local read-only cluster dashboard",
		Long:  "Starts kprompt-dash on localhost (kubeconfig stays on this machine). Install: go install github.com/kprompt/kprompt-dash/cmd/kprompt-dash@latest",
		RunE: func(cmd *cobra.Command, args []string) error {
			bin, err := lookDashBinary()
			if err != nil {
				return err
			}
			dashArgs := []string{"-addr", addr}
			if kubeCtx != "" {
				dashArgs = append(dashArgs, "-context", kubeCtx)
			} else if strings.TrimSpace(os.Getenv("KPROMPT_CONTEXT")) != "" {
				// no-op; use flag from parent if set via --context on root... we use local flag
			}
			if openUI {
				dashArgs = append(dashArgs, "-open")
			}
			c := exec.CommandContext(cmd.Context(), bin, dashArgs...)
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.ErrOrStderr()
			c.Stdin = os.Stdin
			fmt.Fprintf(cmd.ErrOrStderr(), "Starting %s → http://%s\n", bin, addr)
			if err := c.Run(); err != nil {
				return fmt.Errorf("kprompt-dash: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:7474", "dashboard listen address")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context for the dashboard")
	cmd.Flags().BoolVar(&openUI, "open", true, "print Open: URL (kprompt-dash -open)")
	return cmd
}

func lookDashBinary() (string, error) {
	if v := strings.TrimSpace(os.Getenv("KPROMPT_DASH_BIN")); v != "" {
		if st, err := os.Stat(v); err == nil && !st.IsDir() {
			return v, nil
		}
		return "", fmt.Errorf("KPROMPT_DASH_BIN=%s not found", v)
	}
	if p, err := exec.LookPath("kprompt-dash"); err == nil {
		return p, nil
	}
	// Common go install location
	if home, err := os.UserHomeDir(); err == nil {
		cand := filepath.Join(home, "go", "bin", "kprompt-dash")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, nil
		}
	}
	return "", fmt.Errorf(`kprompt-dash not found on PATH

Install the OSS dashboard:
  go install github.com/kprompt/kprompt-dash/cmd/kprompt-dash@latest

Or set KPROMPT_DASH_BIN to the binary path. See https://github.com/kprompt/kprompt-dash`)
}
