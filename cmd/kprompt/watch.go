package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/watchassist"
)

func newWatchCmd() *cobra.Command {
	var (
		once     bool
		interval time.Duration
		jsonOut  bool
		ns       string
		watchCtx string
	)
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Opt-in local proactive scan (suggest investigate; never mutate)",
		Long: `Laptop watcher (S-014 · ADR-0022): scans Pods and recent Warning Events in one
namespace and prints suggested kprompt investigate/why prompts.

Opt-in foreground process only — not a required daemon. Always-on Observe stays
on kprompt agent / Helm (ADR-0013). This command never applies changes.`,
		Example: `  kprompt watch -n payments --once
  kprompt watch -n payments --interval 30s
  kprompt watch -n payments --once --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if ns == "" {
				ns = namespace
			}
			if ns == "" {
				return fmt.Errorf("watch requires -n/--namespace")
			}
			file, err := config.LoadFile()
			if err != nil {
				return err
			}
			ctxName := resolveKubeContext(watchCtx, kubeCtx, file.Context)
			clients, err := cluster.Connect(ctxName)
			if err != nil {
				return err
			}
			run := func() error {
				rep, err := (&watchassist.Analyzer{Client: clients.Clientset}).Run(cmd.Context(), watchassist.Request{
					Namespace: ns,
				})
				if err != nil {
					return err
				}
				if jsonOut || outputFmt == "json" {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(rep)
				}
				fmt.Fprintln(cmd.OutOrStdout(), rep.Summary)
				if len(rep.Suggestions) == 0 {
					return nil
				}
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
				fmt.Fprintln(tw, "SEV\tCODE\tTITLE\tSUGGEST")
				for _, s := range rep.Suggestions {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Severity, s.Code, s.Title, s.Command)
				}
				_ = tw.Flush()
				for _, d := range rep.Degraded {
					fmt.Fprintf(cmd.OutOrStdout(), "degraded: %s\n", d)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "\nNever auto-applies — copy a suggest command to investigate.")
				return nil
			}
			if once || interval <= 0 {
				return run()
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Watching namespace %s every %s (Ctrl-C to stop)\n", ns, interval)
			if err := run(); err != nil {
				return err
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-cmd.Context().Done():
					return nil
				case <-ticker.C:
					fmt.Fprintln(cmd.OutOrStdout(), "---")
					if err := run(); err != nil {
						fmt.Fprintln(os.Stderr, err)
					}
				}
			}
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "run a single scan and exit (default when --interval unset)")
	cmd.Flags().DurationVar(&interval, "interval", 0, "repeat scan every duration (e.g. 30s); implies loop")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON WatchReport")
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "namespace to watch (required)")
	cmd.Flags().StringVar(&watchCtx, "context", "", "kubeconfig context")
	return cmd
}
