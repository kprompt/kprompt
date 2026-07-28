package main

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/learn"
)

func newLearnCmd() *cobra.Command {
	var kubeCtx string
	var jsonOut bool
	var showOnly bool

	cmd := &cobra.Command{
		Use:   "learn",
		Short: "Detect cluster tools and save a local profile",
		Long: `Probes Helm, Linkerd, Prometheus, Gateway API, cert-manager, Argo CD/Flux, and other
integrations via tools.Detect, then writes ~/.kprompt/profiles/<context>.json.

Read-only — never mutates the cluster. The profile biases later intent routing
toward the detected stack. Re-run after installing new controllers.

  kprompt learn
  kprompt learn --context kind-dev
  kprompt learn --show
  kprompt learn --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := config.LoadFile()
			if err != nil {
				return err
			}
			ctxName := kubeCtx
			if ctxName == "" {
				ctxName = file.Context
			}
			if showOnly {
				p, ok := learn.LoadBestEffort(ctxName)
				if !ok {
					fmt.Fprintf(cmd.OutOrStdout(), "No learned profile for context %q.\nRun: kprompt learn\n", displayCtx(ctxName))
					return nil
				}
				if jsonOut {
					return encodeLearnJSON(cmd, p)
				}
				return printLearnProfile(cmd, p, false)
			}
			p, err := learn.Run(cmd.Context(), learn.Options{
				Context: ctxName,
				File:    file,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return encodeLearnJSON(cmd, p)
			}
			return printLearnProfile(cmd, p, true)
		},
	}

	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context for cluster / CRD checks")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON profile")
	cmd.Flags().BoolVar(&showOnly, "show", false, "print the saved profile without re-detecting")

	return cmd
}

func displayCtx(name string) string {
	if name == "" {
		return "(default)"
	}
	return name
}

func printLearnProfile(cmd *cobra.Command, p learn.Profile, saved bool) error {
	out := cmd.OutOrStdout()
	if saved {
		fmt.Fprintf(out,
			"Learned and saved cluster tool profile → %s\n\n",
			learn.MustPath(p.Context))
	} else {
		fmt.Fprintf(out,
			"Showing saved cluster tool profile → %s\n\n",
			learn.MustPath(p.Context))
	}
	fmt.Fprintln(out, p.Summary())
	fmt.Fprintln(out)
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TOOL\tSTATUS\tDETAIL")
	for _, t := range p.Tools {
		fmt.Fprintf(w, "%s\t%s\t%s\n", t.Name, t.Status, sanitizeTab(t.Detail))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(out, "\nRe-run after installing controllers. Doctor shows this profile: kprompt doctor")
	return nil
}

func encodeLearnJSON(cmd *cobra.Command, p learn.Profile) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(p)
}
