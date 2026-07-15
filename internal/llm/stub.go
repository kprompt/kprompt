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

// RollbackStub returns a Stub that classifies a Deployment rollback intent.
// revision 0 means previous revision (omit params.revision).
func RollbackStub(name, namespace string, revision int64) *Stub {
	params := map[string]any{}
	if revision > 0 {
		params["revision"] = revision
	}
	payload, _ := json.Marshal(map[string]any{
		"kind": "rollback",
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

// LogsStub returns a Stub that classifies a logs intent.
func LogsStub(name, namespace, resourceKind string, tail int64, container string) *Stub {
	if resourceKind == "" {
		resourceKind = "Deployment"
	}
	params := map[string]any{}
	if tail > 0 {
		params["tail"] = tail
	}
	if container != "" {
		params["container"] = container
	}
	payload, _ := json.Marshal(map[string]any{
		"kind": "logs",
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

// DescribeStub returns a Stub that classifies a describe intent.
func DescribeStub(name, namespace, resourceKind string) *Stub {
	if resourceKind == "" {
		resourceKind = "Deployment"
	}
	payload, _ := json.Marshal(map[string]any{
		"kind": "describe",
		"target": map[string]any{
			"name":      name,
			"namespace": namespace,
			"kind":      resourceKind,
		},
		"confidence": 1.0,
	})
	return &Stub{Structured: payload}
}

// DeleteStub returns a Stub that classifies a named delete intent.
func DeleteStub(name, namespace, resourceKind string) *Stub {
	if resourceKind == "" {
		resourceKind = "Deployment"
	}
	payload, _ := json.Marshal(map[string]any{
		"kind": "delete",
		"target": map[string]any{
			"name":      name,
			"namespace": namespace,
			"kind":      resourceKind,
		},
		"confidence": 1.0,
	})
	return &Stub{Structured: payload}
}
