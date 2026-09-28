package main

import (
	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/demo"
)

func newDemoCmd() *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Canonical AI Runtime walkthrough ($0, no LLM)",
		Long: `Print the $0 AI Runtime walkthrough (kind + kprompt-examples).

The walkthrough follows one failed rollout through Observe → proposal → explicit
human approval → apply → verify → Learn. It uses heuristic analysis, not an LLM.
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
