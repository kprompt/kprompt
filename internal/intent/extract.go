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
