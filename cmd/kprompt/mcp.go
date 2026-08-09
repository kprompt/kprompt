package main

import (
	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Expose kprompt to IDE assistants over MCP (read/plan-only)",
		Long: `kprompt as an MCP (Model Context Protocol) tool provider.

Assistants like Cursor and Claude Desktop can call kprompt's read/plan-only
tools. Mutating prompts return a typed PlanResult and stop — the MCP surface
never applies a change to the cluster (ADR-0024).`,
	}
	cmd.AddCommand(newMCPServeCmd())
	return cmd
}

func newMCPServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the MCP server over stdio",
		Long:  "Serves MCP over stdin/stdout (newline-delimited JSON-RPC 2.0). Wire this into your editor's MCP config.",
		Example: `  # Editor MCP config command
  kprompt mcp serve`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := mcp.NewServer(cmd.InOrStdin(), cmd.OutOrStdout(), version)
			return srv.Serve(cmd.Context())
		},
	}
}
