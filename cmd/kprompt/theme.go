package main

import (
	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/ui"
)

func newThemeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "theme",
		Short: "Preview built-in terminal color themes",
		Long: `Lists every built-in palette with a short color sample.

Read-only — no cluster or LLM access. Does not change your saved theme;
use --theme, "kprompt config set theme <name>", or KPROMPT_THEME to select one.`,
		Example: `  # Preview every palette (same as theme preview)
  kprompt theme

  # Explicit preview subcommand
  kprompt theme preview

  # Use a theme for one command
  kprompt --theme nord "list pods"`,
		RunE: runThemePreview,
	}
	cmd.AddCommand(&cobra.Command{
		Use:     "preview",
		Short:   "Print a color sample for each built-in theme",
		Example: `  kprompt theme preview`,
		RunE:    runThemePreview,
	})
	return cmd
}

func runThemePreview(cmd *cobra.Command, _ []string) error {
	ui.PreviewThemes(cmd.OutOrStdout())
	return nil
}
