package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/team"
)

func newLoginCmd() *cobra.Command {
	var (
		apiURL string
		open   bool
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Enroll this CLI with a Team org (device login)",
		Long:  "Starts a device-code flow against the control plane. Approve the user code at app.kprompt.ai/connect, then a kp_… token is stored in ~/.kprompt/credentials.yaml (0600).",
		RunE: func(cmd *cobra.Command, args []string) error {
			url := strings.TrimSpace(apiURL)
			if url == "" {
				url = strings.TrimSpace(os.Getenv(team.EnvAPIURL))
			}
			if url == "" {
				url = team.DefaultAPIURL
			}
			creds, err := team.Login(cmd.Context(), team.LoginOptions{
				APIURL:      url,
				OpenBrowser: open,
				Stdout: func(s string) {
					fmt.Fprintln(cmd.OutOrStdout(), s)
				},
			})
			if err != nil {
				return err
			}
			name := creds.OrgName
			if name == "" {
				name = creds.OrgID
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Enrolled in %s", name)
			if creds.MemberEmail != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " as %s", creds.MemberEmail)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintf(cmd.OutOrStdout(), "Token saved to credentials file (%s).\n", creds.TokenHint)
			if pol, err := team.PullPolicy(cmd.Context()); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: policy pull failed: %v\n", err)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Org policy cached (version %d).\n", pol.Version)
			}
			if secrets, err := team.PullSecrets(cmd.Context()); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: secrets pull failed: %v\n", err)
			} else if len(secrets) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Org provider keys cached (%d).\n", len(secrets))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&apiURL, "api-url", "", "control plane base URL (default https://api.kprompt.ai or $KPROMPT_API_URL)")
	cmd.Flags().BoolVar(&open, "open", false, "open the verification URL in a browser")
	return cmd
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke the local Team API token and clear credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, _, err := team.LoadCredentials()
			if err != nil {
				return err
			}
			token := team.ResolveToken(creds)
			if token != "" {
				client := team.NewClient(team.ResolveAPIURL(creds), token)
				if err := client.Revoke(cmd.Context()); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: revoke failed: %v\n", err)
				}
			}
			if err := team.ClearCredentials(); err != nil {
				return err
			}
			_ = team.ClearPolicy()
			_ = team.ClearProviderSecrets()
			fmt.Fprintln(cmd.OutOrStdout(), "Logged out (credentials cleared).")
			return nil
		},
	}
}

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show Team enrollment status",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, ok, err := team.LoadCredentials()
			if err != nil {
				return err
			}
			token := team.ResolveToken(creds)
			if token == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "Not enrolled. Run: kprompt login")
				return nil
			}
			apiURL := team.ResolveAPIURL(creds)
			client := team.NewClient(apiURL, token)
			me, err := client.Me(cmd.Context())
			if err != nil {
				if ok {
					fmt.Fprintf(cmd.OutOrStdout(), "Local credentials present (%s) but /v1/me failed: %v\n", creds.TokenHint, err)
					return nil
				}
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "org:    %s (%s)\n", me.Org.Name, me.Org.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "member: %s (%s)\n", me.Member.Email, me.Member.Role)
			fmt.Fprintf(cmd.OutOrStdout(), "auth:   %s\n", me.Auth)
			fmt.Fprintf(cmd.OutOrStdout(), "api:    %s\n", apiURL)
			if me.Token != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "token:  %s…\n", me.Token.Prefix)
			}
			return nil
		},
	}
}
