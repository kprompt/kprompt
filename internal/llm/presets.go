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
		Name:         "azure",
		Kind:         "openai",
		BaseURL:      "", // must set base_url / KPROMPT_OPENAI_BASE_URL
		DefaultModel: "gpt-4o",
		EnvKeys:      []string{"KPROMPT_AZURE_API_KEY", "AZURE_OPENAI_API_KEY", "KPROMPT_OPENAI_API_KEY"},
		HelpURL:      "https://learn.microsoft.com/azure/ai-foundry/",
	},
	{
		Name:         "anthropic",
		Kind:         "anthropic",
		DefaultModel: "claude-sonnet-4-6",
		EnvKeys:      []string{"KPROMPT_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY"},
		HelpURL:      "https://console.anthropic.com/",
	},
	{
		Name:         "gemini",
		Kind:         "gemini",
		DefaultModel: "gemini-3.6-flash",
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
		Name:         "xai",
		Kind:         "openai",
		BaseURL:      "https://api.x.ai/v1",
		DefaultModel: "grok-4.5",
		EnvKeys:      []string{"KPROMPT_XAI_API_KEY", "XAI_API_KEY"},
		HelpURL:      "https://console.x.ai/",
	},
	{
		Name:         "cerebras",
		Kind:         "openai",
		BaseURL:      "https://api.cerebras.ai/v1",
		DefaultModel: "gpt-oss-120b",
		EnvKeys:      []string{"KPROMPT_CEREBRAS_API_KEY", "CEREBRAS_API_KEY"},
		HelpURL:      "https://cloud.cerebras.ai/",
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
		Name:         "moonshot",
		Kind:         "openai",
		BaseURL:      "https://api.moonshot.ai/v1",
		DefaultModel: "kimi-k3",
		EnvKeys:      []string{"KPROMPT_MOONSHOT_API_KEY", "MOONSHOT_API_KEY"},
		HelpURL:      "https://platform.kimi.ai/console/api-keys",
	},
	{
		Name:         "qwen",
		Kind:         "openai",
		BaseURL:      "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		DefaultModel: "qwen-plus",
		EnvKeys:      []string{"KPROMPT_QWEN_API_KEY", "DASHSCOPE_API_KEY", "QWEN_API_KEY"},
		HelpURL:      "https://www.alibabacloud.com/help/en/model-studio/",
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
		Name:          "lmstudio",
		Kind:          "openai",
		BaseURL:       "http://127.0.0.1:1234/v1",
		DefaultModel:  "local-model", // must match a model loaded in LM Studio
		EnvKeys:       []string{"KPROMPT_LMSTUDIO_API_KEY", "LMSTUDIO_API_KEY"},
		AllowEmptyKey: true,
		HelpURL:       "https://lmstudio.ai/",
	},
	{
		Name:         "together",
		Kind:         "openai",
		BaseURL:      "https://api.together.xyz/v1",
		DefaultModel: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
		EnvKeys:      []string{"KPROMPT_TOGETHER_API_KEY", "TOGETHER_API_KEY"},
		HelpURL:      "https://api.together.xyz/",
	},
	{
		Name:         "fireworks",
		Kind:         "openai",
		BaseURL:      "https://api.fireworks.ai/inference/v1",
		DefaultModel: "accounts/fireworks/models/llama-v3p3-70b-instruct",
		EnvKeys:      []string{"KPROMPT_FIREWORKS_API_KEY", "FIREWORKS_API_KEY"},
		HelpURL:      "https://fireworks.ai/account/api-keys",
	},
	{
		Name:         "hetzner",
		Kind:         "openai",
		BaseURL:      "https://inference.hetzner.com/api/v1",
		DefaultModel: "Qwen/Qwen3.6-35B-A3B-FP8",
		EnvKeys:      []string{"KPROMPT_HETZNER_API_KEY", "HETZNER_API_KEY"},
		HelpURL:      "https://experiments.hetzner.com/docs/inference",
	},
}

// LookupPreset finds a preset by name (case-insensitive).
// Empty name is unconfigured (OB-002) — does not silently map to openai.
func LookupPreset(name string) (Preset, bool) {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return Preset{}, false
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
	msg += fmt.Sprintf("\n  export %s=...", primary)
	msg += "\nOr $0 local LLM: kprompt init --ollama"
	msg += "\nDocs: https://kprompt.ai/docs/providers"
	if p.HelpURL != "" {
		msg += "\nKeys: " + p.HelpURL
	}
	return fmt.Errorf("%s", msg)
}

// ErrProviderUnconfigured is returned when no provider is set in config or flags.
func ErrProviderUnconfigured() error {
	return fmt.Errorf("no LLM provider configured — run: kprompt init --ollama\n  or: kprompt init --provider openai\n  or: kprompt config set provider ollama\nDocs: https://kprompt.ai/docs/providers")
}
