package main

import (
	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/demo"
)

func newDemoCmd() *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Observe walkthrough ($0, no LLM) — prerequisites + commands",
		Long: `Print the $0 Observe agent walkthrough (kind + kprompt-examples).

This is heuristic Observe Mode — not natural-language plan→approve.
Does not clone or mutate anything; prints exact commands after checking PATH tools.

Prerequisites only:
  kprompt demo --check`,
		Example: `  kprompt demo
  kprompt demo --check`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return demo.Run(demo.Options{
				CheckOnly: checkOnly,
				Out:       cmd.OutOrStdout(),
			})
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "only verify Docker/kind/kubectl/make/git/kprompt on PATH")
	return cmd
}
