package main

import (
	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/onboard"
	"github.com/kprompt/kprompt/internal/ui"
)

func newInitCmd() *cobra.Command {
	var (
		ollama   bool
		provider string
		model    string
		kubeCtx  string
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Configure LLM provider for natural-language plans",
		Long: `Day-0 setup for natural-language prompts.

Writes non-secret prefs to ~/.kprompt/config.yaml (provider, model, optional context).
Does not create clusters, install Helm, or enroll Team.

$0 path:
  kprompt init --ollama

BYOK:
  kprompt init --provider openai
  export KPROMPT_OPENAI_API_KEY=...

Non-interactive shells require --ollama or --provider.`,
		Example: `  kprompt init --ollama
  kprompt init --provider openai
  kprompt init --ollama --dry-run
  kprompt init   # interactive TTY wizard`,
		RunE: func(cmd *cobra.Command, args []string) error {
			inter := ui.StdinIsTerminal()
			_, err := onboard.Run(cmd.Context(), onboard.Options{
				Provider:    provider,
				Ollama:      ollama,
				Model:       model,
				Context:     kubeCtx,
				DryRun:      dryRun,
				Interactive: &inter,
				In:          cmd.InOrStdin(),
				Out:         cmd.OutOrStdout(),
			})
			return err
		},
	}
	cmd.Flags().BoolVar(&ollama, "ollama", false, "configure local Ollama ($0, no API key)")
	cmd.Flags().StringVar(&provider, "provider", "", "LLM provider id (openai|anthropic|ollama|…)")
	cmd.Flags().StringVar(&model, "model", "", "model id (default: provider preset)")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "persist kubeconfig context")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be written without saving")
	return cmd
}
