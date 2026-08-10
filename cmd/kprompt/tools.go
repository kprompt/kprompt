package main

import (
	"encoding/json"
	"fmt"
	"io"
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
		Example: `  # Show integrations using the default context
  kprompt tools

  # Show integrations for a specific kube context
  kprompt tools --context kind-dev

  # Output details in JSON format
  kprompt tools --json`,
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
			return printToolsTable(cmd.OutOrStdout(), reg)
		},
	}

	cmd.Flags().StringVar(&kubeCtx, "context", "", "kubeconfig context for cluster / CRD checks")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	return cmd
}

func printToolsTable(out io.Writer, reg *tools.Registry) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TOOL\tSTATUS\tDETAIL")
	for _, r := range reg.All() {
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.Status, sanitizeTab(r.Detail))
	}
	if err := w.Flush(); err != nil {
		return err
	}

	var next []tools.Result
	setupGap := false
	urlGap := false
	for _, r := range reg.All() {
		if r.Status != tools.StatusUnavailable {
			continue
		}
		hint := strings.TrimSpace(r.Hint)
		if hint == "" {
			continue
		}
		next = append(next, r)
		switch r.ID {
		case tools.IDHelm, tools.IDArgoWorkflows, tools.IDPrometheus, tools.IDGrafana, tools.IDOpenTelemetry:
			setupGap = true
		}
		switch r.ID {
		case tools.IDPrometheus, tools.IDGrafana, tools.IDOpenTelemetry:
			urlGap = true
		}
	}

	if len(next) > 0 {
		fmt.Fprintln(out, "\nNext steps (unavailable):")
		for _, r := range next {
			fmt.Fprintf(out, "  - %s: %s\n", r.Name, strings.TrimSpace(r.Hint))
		}
	}
	if setupGap {
		fmt.Fprintln(out, "\nTry: kprompt setup   # dry-run plan for Helm / Argo Workflows / Prometheus (approve-gated)")
	}
	if urlGap {
		fmt.Fprintln(out, "Configure URLs via env (KPROMPT_PROMETHEUS_URL, KPROMPT_GRAFANA_URL, KPROMPT_OTEL_ENDPOINT) or kprompt config set tools.prometheus.url …")
	}
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
