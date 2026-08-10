package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/doctor"
	"github.com/kprompt/kprompt/internal/pipeline"
	"github.com/kprompt/kprompt/internal/tools"
)

// registerTools wires the read/plan-only MCP tool set (ADR-0024, T-068).
// None of these tools apply a mutation to the cluster.
func (s *Server) registerTools() {
	s.register(toolDef{
		Name: "kprompt.read",
		Description: "Answer a natural-language, read-only Kubernetes question against the active kubeconfig " +
			"(e.g. \"list pods in payments\", \"how many nodes\", \"describe api\"). " +
			"Returns a typed PlanResult JSON. Never applies a mutation.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Natural-language read request.",
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
		Handler: s.handleRead,
	})

	s.register(toolDef{
		Name:        "kprompt.tools",
		Description: "List detected integrations (Kubernetes, Helm, Argo, Prometheus, OTel, Grafana, GitOps, …) as JSON. Read-only; does not call an LLM.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"context": map[string]any{
					"type":        "string",
					"description": "Optional kubeconfig context for cluster / CRD checks.",
				},
			},
		},
		Handler: s.handleTools,
	})

	s.register(toolDef{
		Name:        "kprompt.doctor",
		Description: "Run read-only environment health checks (kubeconfig, LLM provider, integrations, Team) and return a JSON report. Never prints API keys.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"context": map[string]any{
					"type":        "string",
					"description": "Optional kubeconfig context for cluster checks.",
				},
			},
		},
		Handler: s.handleDoctor,
	})

	s.registerSRETools() // T-069
	s.registerPlanTool() // T-070
}

// denyApply is the approval hook used for every MCP tool. Returning (false, nil)
// guarantees a mutating plan is never applied over the protocol (ADR-0024 §3).
func denyApply(io.Writer) (bool, error) { return false, nil }

func (s *Server) handleRead(ctx context.Context, args map[string]any) (string, error) {
	prompt := strArg(args, "prompt")
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	return s.runPromptJSON(ctx, prompt, strArg(args, "namespace"), strArg(args, "context"))
}

// runPromptJSON compiles a prompt through the standard pipeline in JSON mode and
// returns the PlanResult document. Confirm=denyApply and IsTerminal=false are
// forced on top of baseDeps, so no mutation is ever applied over MCP, regardless
// of the prompt's intent (ADR-0024 §3).
func (s *Server) runPromptJSON(ctx context.Context, prompt, ns, kctx string) (string, error) {
	file, err := config.LoadFile()
	if err != nil {
		return "", err
	}
	cfg := config.Merge(file, "", "", kctx, ns, false, prompt)
	cfg.Output = "json"
	cfg.NamespaceFromCLI = ns != ""
	cfg.ContextFromCLI = kctx != ""

	isTTY := false
	deps := s.baseDeps
	deps.Confirm = denyApply
	deps.IsTerminal = &isTTY

	var buf bytes.Buffer
	if err := pipeline.RunWith(ctx, cfg, &buf, deps); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (s *Server) handleTools(ctx context.Context, args map[string]any) (string, error) {
	file, err := config.LoadFile()
	if err != nil {
		return "", err
	}
	ctxName := strArg(args, "context")
	if ctxName == "" {
		ctxName = file.Context
	}
	reg, err := tools.Detect(ctx, tools.DetectOptions{Context: ctxName, File: file})
	if err != nil {
		return "", err
	}
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
	return marshalIndent(out)
}

func (s *Server) handleDoctor(ctx context.Context, args map[string]any) (string, error) {
	rep, err := doctor.Run(ctx, doctor.Options{Context: strArg(args, "context")})
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := doctor.FormatJSON(&buf, rep); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func strArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func marshalIndent(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
