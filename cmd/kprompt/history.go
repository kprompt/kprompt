package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/history"
	"github.com/kprompt/kprompt/internal/pipeline"
	"github.com/kprompt/kprompt/internal/team"
	"github.com/kprompt/kprompt/internal/ui"
)

func newHistoryCmd() *cobra.Command {
	var (
		limit      int
		filterNs   string
		filterKind string
	)
	cmd := &cobra.Command{
		Use:   "history",
		Short: "List recent prompts and plans",
		Long:  "Reads append-only ~/.kprompt/history.jsonl (no secrets or manifests).",
		RunE: func(cmd *cobra.Command, args []string) error {
			filterNs = ""
			root := cmd.Root()
			if root.PersistentFlags().Changed("namespace") {
				filterNs = namespace // uses global namespace variable
			}
			entries, err := history.ListFiltered(limit, history.FilterOptions{
				Namespace: filterNs,
				Kind:      filterKind,
			})
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), history.FormatList(entries))
			path, _ := history.DefaultPath()
			if len(entries) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nRe-run: kprompt history rerun [n]   (1 = newest)\nFile:   %s\n", path)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "number of entries to show")
	cmd.Flags().StringVar(&filterKind, "kind", "", "filter history by kind")
	cmd.AddCommand(&cobra.Command{
		Use:   "rerun [index]",
		Short: "Re-run a history prompt (default: newest)",
		Long:  "Replays the stored prompt through the normal pipeline. Use --approve for mutations.",
		Example: `  kprompt history rerun
  kprompt history rerun 3 --approve`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			idx := 1
			if len(args) == 1 {
				n, err := strconv.Atoi(args[0])
				if err != nil || n < 1 {
					return fmt.Errorf("index must be a positive integer")
				}
				idx = n
			}
			entry, err := history.Get(idx, 100)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Re-running #%d: %q\n", idx, entry.Prompt)

			file, err := config.LoadFile()
			if err != nil {
				return err
			}
			root := cmd.Root()
			if root.PersistentFlags().Changed("context") && root.PersistentFlags().Changed("contexts") {
				return fmt.Errorf("use either --context or --contexts, not both")
			}
			cfg := config.Merge(file, provider, model, kubeCtx, namespace, approve, entry.Prompt)
			cfg.Wait = waitFlag
			cfg.Timeout = timeout
			cfg.Output = outputFmt
			cfg.ApproveEachContext = approveEachContext
			cfg.NamespaceFromCLI = root.PersistentFlags().Changed("namespace")
			cfg.ContextFromCLI = root.PersistentFlags().Changed("context") || root.PersistentFlags().Changed("contexts")
			if root.PersistentFlags().Changed("theme") {
				cfg.Theme = theme
			}
			team.ApplyOrgContextPolicy(&cfg)
			if raw := strings.TrimSpace(kubeCtxs); raw != "" {
				names := config.ParseContextsFlag(raw)
				resolved, err := config.ResolveContextList(names, cfg.Aliases)
				if err != nil {
					return err
				}
				cfg.Contexts = resolved
				if len(resolved) == 1 {
					cfg.Context = resolved[0]
				}
			}
			ui.SetTheme(cfg.Theme)
			return pipeline.Run(cmd.Context(), cfg, cmd.OutOrStdout())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "show [index]",
		Short: "Show detailed information for a single history entry",
		Long:  "Displays the prompt text, summary, namespace, and kind/intent for a specific history entry by index (1 = newest).",
		Example: `  kprompt history show 1
  kprompt history show 3`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			idx, err := strconv.Atoi(args[0])
			if err != nil || idx < 1 {
				return fmt.Errorf("index must be a positive integer")
			}
			entry, err := history.Get(idx, 100)
			if err != nil {
				return err
			}

			fmt.Fprint(cmd.OutOrStdout(), history.FormatEntry(idx, entry))
			return nil
		},
	})

	var (
		skipConfirmApprove bool
		skipConfirmYes     bool
	)
	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear all prompt history safely",
		Long:  "Safely truncates/clears the local history file (~/.kprompt/history.jsonl). Requires confirmation unless --approve is passed (--yes is an alias).",
		Example: `  kprompt history clear
  kprompt history clear --approve`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			skip := skipConfirmApprove || skipConfirmYes
			if !skip {
				ok, err := ui.ConfirmClearHistory(cmd.InOrStdin(), cmd.OutOrStdout())
				if err != nil {
					return err
				}
				if !ok {
					ui.PrintAborted(cmd.OutOrStdout())
					return nil
				}
			}
			if err := history.Clear(); err != nil {
				return fmt.Errorf("failed to clear history: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "History cleared.")
			return nil
		},
	}
	clearCmd.Flags().BoolVar(&skipConfirmApprove, "approve", false, "skip confirmation (same vocabulary as mutate apply)")
	clearCmd.Flags().BoolVarP(&skipConfirmYes, "yes", "y", false, "alias for --approve")
	cmd.AddCommand(clearCmd)

	return cmd
}
