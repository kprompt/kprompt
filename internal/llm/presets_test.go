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

func TestSupportedNamesIncludesNewProviders(t *testing.T) {
	s := SupportedNames()
	for _, want := range []string{"openai", "anthropic", "gemini", "groq", "mistral", "deepseek", "ollama", "openrouter", "together"} {
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
