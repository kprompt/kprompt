package llm

import (
	"strings"
	"testing"
)

func TestLookupPresetDefaults(t *testing.T) {
	if _, ok := LookupPreset(""); ok {
		t.Fatal("empty name must be unconfigured, not openai")
	}
	g, ok := LookupPreset("Gemini")
	if !ok || g.Kind != "gemini" {
		t.Fatalf("gemini: %+v", g)
	}
	o, ok := LookupPreset("ollama")
	if !ok || !o.AllowEmptyKey {
		t.Fatal("ollama should allow empty key")
	}
	p, ok := LookupPreset("openai")
	if !ok || p.Name != "openai" {
		t.Fatalf("openai: %+v ok=%v", p, ok)
	}
}

func TestLookupPresetXAI(t *testing.T) {
	p, ok := LookupPreset("xai")
	if !ok {
		t.Fatal("xai preset not found")
	}
	if p.Kind != "openai" {
		t.Fatalf("xai kind = %q, want openai", p.Kind)
	}
	if p.BaseURL != "https://api.x.ai/v1" {
		t.Fatalf("xai base URL = %q", p.BaseURL)
	}
	if p.DefaultModel != "grok-4.5" {
		t.Fatalf("xai default model = %q", p.DefaultModel)
	}
	if len(p.EnvKeys) != 2 || p.EnvKeys[0] != "KPROMPT_XAI_API_KEY" || p.EnvKeys[1] != "XAI_API_KEY" {
		t.Fatalf("xai env keys = %v", p.EnvKeys)
	}
}

func TestLookupPresetCerebras(t *testing.T) {
	p, ok := LookupPreset("cerebras")
	if !ok {
		t.Fatal("cerebras preset not found")
	}
	if p.Kind != "openai" {
		t.Fatalf("cerebras kind = %q, want openai", p.Kind)
	}
	if p.BaseURL != "https://api.cerebras.ai/v1" {
		t.Fatalf("cerebras base URL = %q", p.BaseURL)
	}
	if p.DefaultModel != "gpt-oss-120b" {
		t.Fatalf("cerebras default model = %q", p.DefaultModel)
	}
	if len(p.EnvKeys) != 2 || p.EnvKeys[0] != "KPROMPT_CEREBRAS_API_KEY" || p.EnvKeys[1] != "CEREBRAS_API_KEY" {
		t.Fatalf("cerebras env keys = %v", p.EnvKeys)
	}
}

func TestSupportedNamesIncludesNewProviders(t *testing.T) {
	s := SupportedNames()
	for _, want := range []string{"openai", "anthropic", "gemini", "groq", "mistral", "deepseek", "moonshot", "ollama", "openrouter", "together", "xai", "cerebras", "azure"} {
		if !contains(s, want) {
			t.Fatalf("%q missing from %s", want, s)
		}
	}
}

// Smoke tests: pin DefaultModel for the three presets updated in P-007 (#58).
// These will fail in CI immediately if a preset drifts back to a retired model ID.

func TestLookupPresetGemini(t *testing.T) {
	p, ok := LookupPreset("gemini")
	if !ok {
		t.Fatal("gemini preset not found")
	}
	if p.Kind != "gemini" {
		t.Fatalf("gemini kind = %q, want gemini", p.Kind)
	}
	const want = "gemini-3.6-flash"
	if p.DefaultModel != want {
		t.Fatalf("gemini default model = %q, want %q (update if GA model changes)", p.DefaultModel, want)
	}
}

func TestLookupPresetAnthropic(t *testing.T) {
	p, ok := LookupPreset("anthropic")
	if !ok {
		t.Fatal("anthropic preset not found")
	}
	if p.Kind != "anthropic" {
		t.Fatalf("anthropic kind = %q, want anthropic", p.Kind)
	}
	const want = "claude-sonnet-4-6"
	if p.DefaultModel != want {
		t.Fatalf("anthropic default model = %q, want %q (update if GA model changes)", p.DefaultModel, want)
	}
}

func TestLookupPresetTogether(t *testing.T) {
	p, ok := LookupPreset("together")
	if !ok {
		t.Fatal("together preset not found")
	}
	if p.Kind != "openai" {
		t.Fatalf("together kind = %q, want openai", p.Kind)
	}
	const want = "meta-llama/Llama-3.3-70B-Instruct-Turbo"
	if p.DefaultModel != want {
		t.Fatalf("together default model = %q, want %q (update if model is delisted)", p.DefaultModel, want)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestLookupPresetAzure(t *testing.T) {
	p, ok := LookupPreset("azure")
	if !ok {
		t.Fatal("azure preset not found")
	}
	if p.Kind != "openai" {
		t.Fatalf("azure kind = %q, want openai", p.Kind)
	}
	if p.BaseURL != "" {
		t.Fatalf("azure base URL = %q, want empty", p.BaseURL)
	}
	if p.DefaultModel != "gpt-4o" {
		t.Fatalf("azure default model = %q, want gpt-4o", p.DefaultModel)
	}
	if len(p.EnvKeys) != 3 || p.EnvKeys[0] != "KPROMPT_AZURE_API_KEY" || p.EnvKeys[1] != "AZURE_OPENAI_API_KEY" || p.EnvKeys[2] != "KPROMPT_OPENAI_API_KEY" {
		t.Fatalf("azure env keys = %v", p.EnvKeys)
	}
}

func TestAzureRequiresBaseURL(t *testing.T) {
	t.Setenv("KPROMPT_OPENAI_BASE_URL", "")
	_, err := New("azure", "fake-key", "", "")
	if err == nil || !strings.Contains(err.Error(), "provider azure requires base_url") {
		t.Fatalf("expected error requiring base_url, got %v", err)
	}
}

// Azure routes on the deployment name, so --model must reach the adapter verbatim.
func TestAzureUsesResourceBaseURLAndDeploymentModel(t *testing.T) {
	t.Setenv("KPROMPT_OPENAI_BASE_URL", "https://r.openai.azure.com/openai/v1/")
	p, err := New("azure", "fake-key", "", "my-gpt4o-deploy")
	if err != nil {
		t.Fatalf("New(azure) = %v", err)
	}
	o, ok := p.(*OpenAI)
	if !ok {
		t.Fatalf("azure provider type = %T, want *OpenAI", p)
	}
	if o.baseURL != "https://r.openai.azure.com/openai/v1" {
		t.Fatalf("azure base URL = %q, want trailing slash trimmed", o.baseURL)
	}
	if o.model != "my-gpt4o-deploy" {
		t.Fatalf("azure model = %q, want my-gpt4o-deploy", o.model)
	}
}
