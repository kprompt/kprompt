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
		Short: "Set a config value (provider|model|base_url|context|namespace|tools.*)",
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
	default:
		return ""
	}
}
