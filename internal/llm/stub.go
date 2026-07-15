package llm

import (
	"context"
	"encoding/json"
)

// Stub is a deterministic Provider for tests (no network).
type Stub struct {
	Structured json.RawMessage
	Text       string
}

func (s *Stub) Name() string { return "stub" }

func (s *Stub) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	return CompletionResponse{Text: s.Text}, nil
}

func (s *Stub) CompleteStructured(ctx context.Context, req CompletionRequest, schema json.RawMessage) (json.RawMessage, error) {
	if len(s.Structured) == 0 {
		return json.RawMessage(`{"kind":"unknown","target":{}}`), nil
	}
	return s.Structured, nil
}

// ScaleStub returns a Stub that always classifies a Deployment scale intent.
func ScaleStub(name, namespace string, replicas int) *Stub {
	payload, _ := json.Marshal(map[string]any{
		"kind": "scale",
		"target": map[string]any{
			"name":      name,
			"namespace": namespace,
			"kind":      "Deployment",
		},
		"params": map[string]any{
			"replicas": replicas,
		},
		"confidence": 1.0,
	})
	return &Stub{Structured: payload}
}
