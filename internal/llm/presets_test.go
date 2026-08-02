package llm

import "testing"

func TestLookupPresetDefaults(t *testing.T) {
	p, ok := LookupPreset("")
	if !ok || p.Name != "openai" {
		t.Fatalf("empty -> openai, got %+v ok=%v", p, ok)
	}
	g, ok := LookupPreset("Gemini")
	if !ok || g.Kind != "gemini" {
		t.Fatalf("gemini: %+v", g)
	}
	o, ok := LookupPreset("ollama")
	if !ok || !o.AllowEmptyKey {
		t.Fatal("ollama should allow empty key")
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
	for _, want := range []string{"openai", "anthropic", "gemini", "groq", "mistral", "deepseek", "moonshot", "ollama", "openrouter", "together", "xai", "cerebras"} {
		if !contains(s, want) {
			t.Fatalf("%q missing from %s", want, s)
		}
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
