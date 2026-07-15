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

// DeployStub returns a Stub that classifies a deploy intent.
func DeployStub(name, namespace, image string, replicas int, port int) *Stub {
	params := map[string]any{
		"replicas": replicas,
	}
	if image != "" {
		params["image"] = image
	}
	if port > 0 {
		params["port"] = port
		params["createService"] = true
	}
	payload, _ := json.Marshal(map[string]any{
		"kind": "deploy",
		"target": map[string]any{
			"name":      name,
			"namespace": namespace,
			"kind":      "Deployment",
		},
		"params":     params,
		"confidence": 1.0,
	})
	return &Stub{Structured: payload}
}

// GetStub returns a Stub that classifies a get/list intent.
func GetStub(resourceKind, name, namespace, minMemory string) *Stub {
	params := map[string]any{}
	if minMemory != "" {
		params["minMemory"] = minMemory
	}
	payload, _ := json.Marshal(map[string]any{
		"kind": "get",
		"target": map[string]any{
			"name":      name,
			"namespace": namespace,
			"kind":      resourceKind,
		},
		"params":     params,
		"confidence": 1.0,
	})
	return &Stub{Structured: payload}
}

// ExplainStub returns a Stub that classifies an explain intent.
func ExplainStub(name, namespace, resourceKind string) *Stub {
	if resourceKind == "" {
		resourceKind = "Deployment"
	}
	payload, _ := json.Marshal(map[string]any{
		"kind": "explain",
		"target": map[string]any{
			"name":      name,
			"namespace": namespace,
			"kind":      resourceKind,
		},
		"confidence": 1.0,
	})
	return &Stub{Structured: payload}
}
