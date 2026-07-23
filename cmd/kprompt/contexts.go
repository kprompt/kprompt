package main

import (
	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/contexts"
)

func newContextsCmd() *cobra.Command {
	var (
		jsonOut bool
		check   bool
	)
	cmd := &cobra.Command{
		Use:     "contexts",
		Aliases: []string{"context", "ctx"},
		Short:   "List kubeconfig contexts and local aliases",
		Long:    "Shows kubeconfig contexts, which are current, and aliases from ~/.kprompt/config.yaml. Optional --check probes API reachability per context.",
		RunE: func(cmd *cobra.Command, args []string) error {
			rep, err := contexts.List(cmd.Context(), contexts.Options{
				CheckReachability: check,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return contexts.FormatJSON(cmd.OutOrStdout(), rep)
			}
			return contexts.FormatText(cmd.OutOrStdout(), rep)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON inventory")
	cmd.Flags().BoolVar(&check, "check", false, "probe API reachability for each context")
	return cmd
}
