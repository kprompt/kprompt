package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/team"
)

func newSecretsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Pull org LLM provider keys from Team",
		Long:  "Caches keys at ~/.kprompt/provider-secrets.yaml (0600). Env vars always override pulled keys (ADR-0005). Does not print secret values.",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "pull",
		Short: "Fetch org provider keys and cache them locally",
		RunE: func(cmd *cobra.Command, args []string) error {
			secrets, err := team.PullSecrets(cmd.Context())
			if err != nil {
				return err
			}
			path, _ := team.ProviderSecretsPath()
			fmt.Fprintf(cmd.OutOrStdout(), "Pulled %d provider key(s) → %s\n", len(secrets), path)
			fmt.Fprintf(cmd.OutOrStdout(), "providers: %s\n", team.FormatSecretProviders(secrets))
			return nil
		},
	})
	return cmd
}
