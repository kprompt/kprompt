package mcp

import (
	"context"
	"fmt"
	"strings"
)

// registerSRETools wires the read-only AI SRE reasoning tools (ADR-0024, T-069).
// Each compiles a canonical prompt onto its intent kind and returns an
// InvestigationDoc inside the PlanResult JSON. None apply a mutation.
func (s *Server) registerSRETools() {
	s.register(toolDef{
		Name:        "kprompt.investigate",
		Description: "Root-cause a workload with a multi-hop Service→Endpoints→Pods→Events→Logs walk. Returns an investigation PlanResult. Read-only.",
		InputSchema: sreSchema("Workload to investigate (e.g. \"payment-api\")."),
		Handler:     s.sreHandler("investigate %s"),
	})
	s.register(toolDef{
		Name:        "kprompt.why",
		Description: "Explain why a workload is failing/pending/crashing (causal analysis). Returns an investigation PlanResult. Read-only.",
		InputSchema: sreSchema("Workload to explain (e.g. \"ledger\")."),
		Handler:     s.sreHandler("why is %s failing"),
	})
	s.register(toolDef{
		Name:        "kprompt.timeline",
		Description: "Build a chronological timeline of what happened to a workload. Returns an investigation PlanResult. Read-only.",
		InputSchema: sreSchema("Workload to build a timeline for (e.g. \"api\")."),
		Handler:     s.sreHandler("timeline for %s"),
	})
	s.register(toolDef{
		Name:        "kprompt.impact",
		Description: "Compute reverse dependencies / blast radius — who consumes or depends on a live object. Returns an impact PlanResult. Read-only.",
		InputSchema: sreSchema("Service or Deployment to assess impact for (e.g. \"redis\")."),
		Handler:     s.sreHandler("impact of %s"),
	})
}

// sreSchema builds the shared input schema for target-based SRE tools.
func sreSchema(targetDesc string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"target": map[string]any{
				"type":        "string",
				"description": targetDesc,
			},
			"namespace": map[string]any{
				"type":        "string",
				"description": "Optional namespace override.",
			},
			"context": map[string]any{
				"type":        "string",
				"description": "Optional kubeconfig context override.",
			},
		},
		"required": []any{"target"},
	}
}

// sreHandler returns a handler that formats promptFmt with the target argument
// and runs it through the read-only pipeline.
func (s *Server) sreHandler(promptFmt string) func(context.Context, map[string]any) (string, error) {
	return func(ctx context.Context, args map[string]any) (string, error) {
		target := strings.TrimSpace(strArg(args, "target"))
		if target == "" {
			return "", fmt.Errorf("target is required")
		}
		prompt := fmt.Sprintf(promptFmt, target)
		return s.runPromptJSON(ctx, prompt, strArg(args, "namespace"), strArg(args, "context"))
	}
}
