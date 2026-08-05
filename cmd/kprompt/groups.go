package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

const (
	groupDay0     = "day0"
	groupAdvanced = "advanced"
)

func registerCommandGroups(root *cobra.Command) {
	root.AddGroup(
		&cobra.Group{ID: groupDay0, Title: "Day-0 Commands:"},
		&cobra.Group{ID: groupAdvanced, Title: "Advanced Commands:"},
	)
}

func withGroup(cmd *cobra.Command, groupID string) *cobra.Command {
	cmd.GroupID = groupID
	return cmd
}

func newAdvancedHelpCmd(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "advanced",
		Short: "List advanced commands (agent, Team, setup, …)",
		Long:  "Advanced commands stay fully usable by name; this lists them when you want the full surface beyond Day-0.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return printAdvancedCommands(cmd.OutOrStdout(), root)
		},
	}
	cmd.GroupID = groupDay0
	return cmd
}

func printAdvancedCommands(w io.Writer, root *cobra.Command) error {
	fmt.Fprintln(w, "Advanced commands (also shown under Advanced in kprompt --help):")
	fmt.Fprintln(w, "")
	for _, c := range root.Commands() {
		if c.Hidden || c.GroupID != groupAdvanced {
			continue
		}
		fmt.Fprintf(w, "  %-14s %s\n", c.Name(), c.Short)
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Details: kprompt <command> --help")
	return nil
}
