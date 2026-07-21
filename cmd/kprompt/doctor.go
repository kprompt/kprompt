package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/doctor"
)

func newDoctorCmd() *cobra.Command {
	var (
		jsonOut bool
		kubeCtx string
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check local setup (kube, LLM key, tools, Team)",
		Long:  "Runs read-only health checks. Does not print API keys. Exit code 1 if a required check fails.",
		RunE: func(cmd *cobra.Command, args []string) error {
			rep, err := doctor.Run(cmd.Context(), doctor.Options{Context: kubeCtx})
			if err != nil {
				return err
			}
			if jsonOut {
				if err := doctor.FormatJSON(cmd.OutOrStdout(), rep); err != nil {
					return err
				}
			} else {
				if err := doctor.FormatText(cmd.OutOrStdout(), rep); err != nil {
					return err
				}
			}
			if !rep.OK {
				return fmt.Errorf("doctor: required checks failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON report")
	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context for cluster checks")
	return cmd
}
