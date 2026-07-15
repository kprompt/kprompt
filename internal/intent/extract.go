package intent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kprompt/kprompt/internal/llm"
)

const systemPrompt = `You convert Kubernetes ops requests into a single Intent JSON object.
Rules:
- kind must be one of: deploy, scale, get, explain, deny, unknown
- For scale: set target.name, target.kind (usually Deployment), target.namespace if mentioned, params.replicas as a number
- For deploy: set target.name (workload name), params.image when known (e.g. redis:7-alpine, nginx:1.27-alpine); for well-known apps like "redis" or "nginx" name alone is enough; optional params.replicas (default 1), params.port and/or params.createService=true for a ClusterIP Service
- For get/list/show: kind=get; set target.kind to Pod, Deployment, or Service; target.namespace if mentioned; target.name only for a single object; optional params.labelSelector; optional params.minMemory (e.g. "2Gi") when the user asks for pods using more than X memory (filter by memory requests)
- For clearly destructive wipe/delete-cluster requests: kind=deny
- Prefer Deployment as target.kind for named apps when unspecified
- Only emit JSON matching the schema`

// Extract uses an LLM provider to produce a structured Intent.
func Extract(ctx context.Context, provider llm.Provider, prompt, defaultNamespace string) (Intent, error) {
	schema := json.RawMessage(SchemaJSON)
	raw, err := provider.CompleteStructured(ctx, llm.CompletionRequest{
		System: systemPrompt,
		User:   prompt,
	}, schema)
	if err != nil {
		return Intent{}, fmt.Errorf("intent extract: %w", err)
	}
	in, err := ParseStructured(raw)
	if err != nil {
		return Intent{}, fmt.Errorf("intent parse: %w", err)
	}
	in.Raw = prompt
	if in.Target.Namespace == "" && defaultNamespace != "" {
		in.Target.Namespace = defaultNamespace
	}
	return in, nil
}
