package llm

import (
	"context"
	"encoding/json"
	"fmt"
)

// CompletionRequest is a provider-agnostic chat completion request.
type CompletionRequest struct {
	System string
	User   string
	Model  string
}

// CompletionResponse is plain text output.
type CompletionResponse struct {
	Text string
}

// Provider is the multi-LLM boundary (ADR-0002).
type Provider interface {
	Name() string
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
	CompleteStructured(ctx context.Context, req CompletionRequest, schema json.RawMessage) (json.RawMessage, error)
}

// Registry maps provider names to constructors is handled by New.
func New(name, apiKey, baseURL, model string) (Provider, error) {
	switch name {
	case "openai", "openai-compatible", "":
		if apiKey == "" {
			return nil, fmt.Errorf("missing API key for openai — set KPROMPT_OPENAI_API_KEY (or OPENAI_API_KEY). Example:\n  export KPROMPT_OPENAI_API_KEY=sk-...\nUsage guide: https://kprompt-website.vercel.app/#usage")
		}
		return NewOpenAI(apiKey, baseURL, model), nil
	case "anthropic":
		if apiKey == "" {
			return nil, fmt.Errorf("missing API key for anthropic — set KPROMPT_ANTHROPIC_API_KEY (or ANTHROPIC_API_KEY). Example:\n  export KPROMPT_ANTHROPIC_API_KEY=sk-ant-...\nUsage guide: https://kprompt-website.vercel.app/#usage")
		}
		return NewAnthropic(apiKey, model), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: openai, anthropic)", name)
	}
}
