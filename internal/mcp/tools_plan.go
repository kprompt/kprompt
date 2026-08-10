package mcp

import (
	"context"
	"fmt"
)

// registerPlanTool wires kprompt.plan (ADR-0024, T-070): the explicit mutation
// surface. A natural-language mutation prompt is compiled into a typed
// PlanResult (actions, diff, risk, blast radius) and returned as JSON — it is
// NEVER applied. Hard-denied intents (wipe-class / namespace-delete) still
// return a denied PlanResult. Approval remains a human action out-of-band.
func (s *Server) registerPlanTool() {
	s.register(toolDef{
		Name: "kprompt.plan",
		Description: "Compile a natural-language mutation (e.g. \"scale api to 10\", \"rollback payment-api\", \"install redis\") " +
			"into a typed PlanResult JSON: actions, diff, risk, blast radius. Never applies the change — approval stays a human " +
			"action you run yourself with `kprompt \"…\" --approve`. Wipe-class intents are hard-denied.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Natural-language mutation request.",
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
			"required": []any{"prompt"},
		},
		Handler: s.handlePlan,
	})
}

func (s *Server) handlePlan(ctx context.Context, args map[string]any) (string, error) {
	prompt := strArg(args, "prompt")
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	return s.runPromptJSON(ctx, prompt, strArg(args, "namespace"), strArg(args, "context"))
}
