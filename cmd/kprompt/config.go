package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/config"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or update non-secret kprompt settings",
		Long:  "Reads/writes ~/.kprompt/config.yaml. API keys are never stored or printed — only env status (set/unset).",
		RunE: func(cmd *cobra.Command, args []string) error {
			view, err := config.BuildView()
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), config.FormatView(view))
			return nil
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value (provider|model|base_url|context|namespace|require_alias_match|tools.*)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.SetField(args[0], args[1])
			if err != nil {
				return err
			}
			path, _ := config.DefaultPath()
			fmt.Fprintf(cmd.OutOrStdout(), "Updated %s\n  %s = %s\n", path, args[0], displaySet(args[0], f))
			return nil
		},
	})

	cmd.AddCommand(newConfigAliasCmd())
	return cmd
}

func newConfigAliasCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias",
		Short: "Manage cluster context aliases (prod → kubeconfig context)",
		Long:  "Aliases let --context prod (or prompt phrases) resolve to a real kubeconfig context. Optional require_alias_match refuses mutate when kubectl current-context differs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.LoadFile()
			if err != nil {
				return err
			}
			lines := config.AliasLines(f.Aliases)
			if len(lines) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No aliases. Set one with:\n  kprompt config alias set prod <kube-context>")
				return nil
			}
			for _, line := range lines {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "set <name> <kube-context>",
		Short: "Map an alias to a kubeconfig context",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := config.SetAlias(args[0], args[1])
			if err != nil {
				return err
			}
			path, _ := config.DefaultPath()
			fmt.Fprintf(cmd.OutOrStdout(), "Updated %s\n  alias %s → %s\n", path, args[0], f.Aliases[args[0]])
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "unset <name>",
		Short: "Remove a context alias",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := config.UnsetAlias(args[0]); err != nil {
				return err
			}
			path, _ := config.DefaultPath()
			fmt.Fprintf(cmd.OutOrStdout(), "Removed alias %q from %s\n", args[0], path)
			return nil
		},
	})

	return cmd
}

func displaySet(key string, f config.File) string {
	switch key {
	case "provider":
		return f.Provider
	case "model":
		return f.Model
	case "base_url", "base-url", "baseurl":
		if f.BaseURL == "" {
			return "(cleared)"
		}
		return f.BaseURL
	case "context":
		if f.Context == "" {
			return "(cleared)"
		}
		return f.Context
	case "namespace", "ns":
		return f.Namespace
	case "require_alias_match", "require-alias-match":
		return fmt.Sprintf("%v", f.RequireAliasMatch)
	default:
		return ""
	}
}
