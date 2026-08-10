package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/runworker"
	"github.com/kprompt/kprompt/internal/team"
)

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Team app-initiated runs (CLI bridge)",
		Long:  "Poll the Team control plane for browser-queued prompt jobs and execute them locally with your kubeconfig (ADR-0021 / A-052). Cluster credentials never leave this machine.",
	}
	cmd.AddCommand(newRunListenCmd())
	return cmd
}

func newRunListenCmd() *cobra.Command {
	var (
		interval    time.Duration
		workerLabel string
	)
	cmd := &cobra.Command{
		Use:   "listen",
		Short: "Poll for Team /run jobs and execute plans locally",
		Long: `Enrolled CLI bridge worker: claims jobs from POST /v1/runs/claim, runs the
local plan pipeline (never auto-applies), and posts PlanResult to
POST /v1/runs/{id}/result.

Requires kprompt login (operator or admin). Empty state in the app:
"Enroll a CLI worker (kprompt login + kprompt run listen)".`,
		Example: `  kprompt run listen
  kprompt run listen --interval 5s
  kprompt run listen --worker-label my-worker-123`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, ok, err := team.LoadCredentials()
			if err != nil {
				return err
			}
			token := team.ResolveToken(creds)
			if !ok || strings.TrimSpace(token) == "" {
				return fmt.Errorf("not enrolled — run: kprompt login")
			}
			client := team.NewClient(team.ResolveAPIURL(creds), token)
			// Longer timeout for plan LLM calls via result posting is separate;
			// claim poll stays short on the default client.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return team.Listen(ctx, client, team.BridgeOptions{
				WorkerLabel:  workerLabel,
				Interval:     interval,
				Execute:      runworker.Execute,
				ExecuteApply: runworker.ExecuteApply,
				Stdout: func(s string) {
					fmt.Fprintln(cmd.OutOrStdout(), s)
				},
			})
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 3*time.Second, "poll interval when the queue is empty")
	cmd.Flags().StringVar(&workerLabel, "worker-label", "", "label reported on claim (default: bridge-<hostname>)")
	return cmd
}
