package llm

import (
	"fmt"
	"strings"
)

// Preset describes a named LLM backend.
type Preset struct {
	Name          string
	Kind          string // openai | anthropic | gemini
	BaseURL       string // for openai-compatible
	DefaultModel  string
	EnvKeys       []string // preferred env vars for API key
	AllowEmptyKey bool
	HelpURL       string
}

// Presets is the supported provider catalog.
var Presets = []Preset{
	{
		Name:         "openai",
		Kind:         "openai",
		BaseURL:      "https://api.openai.com/v1",
		DefaultModel: "gpt-4o-mini",
		EnvKeys:      []string{"KPROMPT_OPENAI_API_KEY", "OPENAI_API_KEY"},
		HelpURL:      "https://platform.openai.com/api-keys",
	},
	{
		Name:         "openai-compatible",
		Kind:         "openai",
		BaseURL:      "", // must set base_url / KPROMPT_OPENAI_BASE_URL
		DefaultModel: "gpt-4o-mini",
		EnvKeys:      []string{"KPROMPT_OPENAI_API_KEY", "OPENAI_API_KEY"},
	},
	{
		Name:         "anthropic",
		Kind:         "anthropic",
		DefaultModel: "claude-sonnet-4-20250514",
		EnvKeys:      []string{"KPROMPT_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY"},
		HelpURL:      "https://console.anthropic.com/",
	},
	{
		Name:         "gemini",
		Kind:         "gemini",
		DefaultModel: "gemini-2.0-flash",
		EnvKeys:      []string{"KPROMPT_GEMINI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY"},
		HelpURL:      "https://aistudio.google.com/apikey",
	},
	{
		Name:         "groq",
		Kind:         "openai",
		BaseURL:      "https://api.groq.com/openai/v1",
		DefaultModel: "llama-3.3-70b-versatile",
		EnvKeys:      []string{"KPROMPT_GROQ_API_KEY", "GROQ_API_KEY"},
		HelpURL:      "https://console.groq.com/keys",
	},
	{
		Name:         "mistral",
		Kind:         "openai",
		BaseURL:      "https://api.mistral.ai/v1",
		DefaultModel: "mistral-small-latest",
		EnvKeys:      []string{"KPROMPT_MISTRAL_API_KEY", "MISTRAL_API_KEY"},
		HelpURL:      "https://console.mistral.ai/",
	},
	{
		Name:         "deepseek",
		Kind:         "openai",
		BaseURL:      "https://api.deepseek.com/v1",
		DefaultModel: "deepseek-chat",
		EnvKeys:      []string{"KPROMPT_DEEPSEEK_API_KEY", "DEEPSEEK_API_KEY"},
		HelpURL:      "https://platform.deepseek.com/",
	},
	{
		Name:         "openrouter",
		Kind:         "openai",
		BaseURL:      "https://openrouter.ai/api/v1",
		DefaultModel: "openai/gpt-4o-mini",
		EnvKeys:      []string{"KPROMPT_OPENROUTER_API_KEY", "OPENROUTER_API_KEY"},
		HelpURL:      "https://openrouter.ai/keys",
	},
	{
		Name:          "ollama",
		Kind:          "openai",
		BaseURL:       "http://127.0.0.1:11434/v1",
		DefaultModel:  "llama3.2",
		EnvKeys:       []string{"KPROMPT_OLLAMA_API_KEY", "OLLAMA_API_KEY"},
		AllowEmptyKey: true,
		HelpURL:       "https://ollama.com/",
	},
	{
		Name:         "together",
		Kind:         "openai",
		BaseURL:      "https://api.together.xyz/v1",
		DefaultModel: "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
		EnvKeys:      []string{"KPROMPT_TOGETHER_API_KEY", "TOGETHER_API_KEY"},
		HelpURL:      "https://api.together.xyz/",
	},
}

// LookupPreset finds a preset by name (case-insensitive).
func LookupPreset(name string) (Preset, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		n = "openai"
	}
	for _, p := range Presets {
		if p.Name == n {
			return p, true
		}
	}
	return Preset{}, false
}

// SupportedNames returns comma-separated provider ids for error messages.
func SupportedNames() string {
	names := make([]string, 0, len(Presets))
	for _, p := range Presets {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}

func missingKeyError(p Preset) error {
	primary := "API key"
	if len(p.EnvKeys) > 0 {
		primary = p.EnvKeys[0]
	}
	msg := fmt.Sprintf("missing API key for %s — set %s", p.Name, primary)
	if len(p.EnvKeys) > 1 {
		msg += fmt.Sprintf(" (or %s)", strings.Join(p.EnvKeys[1:], " / "))
	}
	msg += fmt.Sprintf("\n  export %s=...\nUsage guide: https://kprompt-website.vercel.app/#usage", primary)
	if p.HelpURL != "" {
		msg += "\nKeys: " + p.HelpURL
	}
	return fmt.Errorf("%s", msg)
}
