package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/tools"
)

func newToolsCmd() *cobra.Command {
	var kubeCtx string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "tools",
		Short: "Show detected integrations (Helm, Argo, Prometheus, …)",
		Long:  "Probes local binaries, configured URLs, and the active Kubernetes cluster. Read-only — does not call an LLM.",
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := config.LoadFile()
			if err != nil {
				return err
			}
			ctxName := kubeCtx
			if ctxName == "" {
				ctxName = file.Context
			}
			reg, err := tools.Detect(cmd.Context(), tools.DetectOptions{
				Context: ctxName,
				File:    file,
			})
			if err != nil {
				return err
			}
			if jsonOut {
				return encodeToolsJSON(cmd, reg)
			}
			return printToolsTable(cmd, reg)
		},
	}

	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context for cluster / CRD checks")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	return cmd
}

func printToolsTable(cmd *cobra.Command, reg *tools.Registry) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TOOL\tSTATUS\tDETAIL")
	for _, r := range reg.All() {
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.Status, sanitizeTab(r.Detail))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "\nConfigure URLs via env (KPROMPT_PROMETHEUS_URL, KPROMPT_GRAFANA_URL, KPROMPT_OTEL_ENDPOINT) or kprompt config set tools.prometheus.url …")
	return nil
}

func encodeToolsJSON(cmd *cobra.Command, reg *tools.Registry) error {
	type row struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		Status       string   `json:"status"`
		Detail       string   `json:"detail"`
		Hint         string   `json:"hint,omitempty"`
		Available    bool     `json:"available"`
		Capabilities []string `json:"capabilities,omitempty"`
	}
	out := make([]row, 0, len(reg.All()))
	for _, r := range reg.All() {
		caps := make([]string, len(r.Capabilities))
		for i, c := range r.Capabilities {
			caps[i] = string(c)
		}
		out = append(out, row{
			ID:           string(r.ID),
			Name:         r.Name,
			Status:       string(r.Status),
			Detail:       r.Detail,
			Hint:         r.Hint,
			Available:    r.Available(),
			Capabilities: caps,
		})
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func sanitizeTab(s string) string {
	return strings.ReplaceAll(s, "\t", " ")
}
