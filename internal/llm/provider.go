package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
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

// New constructs a Provider from a preset name.
// baseURL overrides the preset default when non-empty (openai-compatible backends).
func New(name, apiKey, baseURL, model string) (Provider, error) {
	preset, ok := LookupPreset(name)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (supported: %s)", name, SupportedNames())
	}

	key := strings.TrimSpace(apiKey)
	if key == "" {
		key = firstEnv(preset.EnvKeys...)
	}
	if key == "" && preset.AllowEmptyKey {
		key = "ollama"
	}
	if key == "" {
		return nil, missingKeyError(preset)
	}

	mdl := model
	if mdl == "" {
		mdl = preset.DefaultModel
	}

	switch preset.Kind {
	case "openai":
		bu := strings.TrimRight(baseURL, "/")
		if bu == "" {
			bu = preset.BaseURL
		}
		if bu == "" {
			bu = strings.TrimRight(os.Getenv("KPROMPT_OPENAI_BASE_URL"), "/")
		}
		if bu == "" && preset.Name == "openai-compatible" {
			return nil, fmt.Errorf("provider openai-compatible requires base_url (config or KPROMPT_OPENAI_BASE_URL)")
		}
		if bu == "" {
			bu = "https://api.openai.com/v1"
		}
		return NewOpenAI(key, bu, mdl), nil
	case "anthropic":
		return NewAnthropic(key, mdl), nil
	case "gemini":
		return NewGemini(key, mdl), nil
	default:
		return nil, fmt.Errorf("internal: unhandled provider kind %q", preset.Kind)
	}
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
