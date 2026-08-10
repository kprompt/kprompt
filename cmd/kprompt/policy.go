package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/team"
)

func newPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Show or pull Team org policy",
		Long:  "Cached at ~/.kprompt/policy.yaml. Org rules only tighten local hard-denies.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return showPolicy(cmd)
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:     "pull",
		Short:   "Fetch org policy from the control plane and cache it",
		Example: `  kprompt policy pull`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pol, err := team.PullPolicy(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Pulled policy for org %s (version %d)\n", pol.OrgID, pol.Version)
			fmt.Fprint(cmd.OutOrStdout(), formatPolicy(pol))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:     "show",
		Short:   "Show the cached org policy",
		Example: `  kprompt policy show`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return showPolicy(cmd)
		},
	})
	return cmd
}

func showPolicy(cmd *cobra.Command) error {
	pol, ok, err := team.LoadPolicy()
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(cmd.OutOrStdout(), "No cached policy. Run: kprompt policy pull")
		return nil
	}
	fmt.Fprint(cmd.OutOrStdout(), formatPolicy(pol))
	return nil
}

func formatPolicy(p team.Policy) string {
	var b strings.Builder
	fmt.Fprintf(&b, "org_id:            %s\n", p.OrgID)
	fmt.Fprintf(&b, "version:           %d\n", p.Version)
	fmt.Fprintf(&b, "updated_at:        %s\n", p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"))
	fmt.Fprintf(&b, "max_risk:          %s\n", p.MaxRisk)
	fmt.Fprintf(&b, "deny_intents:      %s\n", strings.Join(p.DenyIntents, ", "))
	fmt.Fprintf(&b, "allow_namespaces:  %s\n", strings.Join(p.AllowNamespaces, ", "))
	fmt.Fprintf(&b, "deny_namespaces:   %s\n", strings.Join(p.DenyNamespaces, ", "))
	fmt.Fprintf(&b, "require_approve:   %v\n", p.RequireApprove)
	if len(p.ChangeWindows) > 0 {
		fmt.Fprintf(&b, "change_windows:    %d\n", len(p.ChangeWindows))
	}
	if len(p.ApproveByRole) > 0 {
		fmt.Fprintf(&b, "approve_by_role:\n")
		for _, role := range []string{"viewer", "operator", "admin"} {
			if risks, ok := p.ApproveByRole[role]; ok {
				fmt.Fprintf(&b, "  %s: [%s]\n", role, strings.Join(risks, ", "))
			}
		}
	}
	if len(p.ContextAliases) > 0 {
		fmt.Fprintf(&b, "context_aliases:\n")
		for k, v := range p.ContextAliases {
			fmt.Fprintf(&b, "  %s → %s\n", k, v)
		}
	}
	if p.RequireAliasMatch {
		fmt.Fprintf(&b, "require_alias_match: true\n")
	}
	return b.String()
}
