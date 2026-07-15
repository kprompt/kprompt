package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	EnvOpenAIKey     = "KPROMPT_OPENAI_API_KEY"
	EnvAnthropicKey  = "KPROMPT_ANTHROPIC_API_KEY"
	EnvOpenAIBaseURL = "KPROMPT_OPENAI_BASE_URL"
)

// File holds non-secret preferences (~/.kprompt/config.yaml).
type File struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	BaseURL   string `yaml:"base_url,omitempty"`
	Context   string `yaml:"context,omitempty"`
	Namespace string `yaml:"namespace,omitempty"`
}

// Resolved is the effective runtime configuration.
type Resolved struct {
	Provider  string
	Model     string
	BaseURL   string
	Context   string
	Namespace string
	Approve   bool
	Prompt    string
}

// LoadFile reads ~/.kprompt/config.yaml if present.
func LoadFile() (File, error) {
	path, err := DefaultPath()
	if err != nil {
		return File{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, nil
		}
		return File{}, fmt.Errorf("read config: %w", err)
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return File{}, fmt.Errorf("parse config: %w", err)
	}
	return f, nil
}

// DefaultPath returns ~/.kprompt/config.yaml.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kprompt", "config.yaml"), nil
}

// APIKeyFor returns the env-sourced API key for a provider.
func APIKeyFor(provider string) string {
	switch provider {
	case "openai", "openai-compatible", "":
		if v := os.Getenv(EnvOpenAIKey); v != "" {
			return v
		}
		// Common fallbacks
		return os.Getenv("OPENAI_API_KEY")
	case "anthropic":
		if v := os.Getenv(EnvAnthropicKey); v != "" {
			return v
		}
		return os.Getenv("ANTHROPIC_API_KEY")
	default:
		return ""
	}
}

// OpenAIBaseURL returns the OpenAI-compatible base URL if set.
func OpenAIBaseURL(cfg string) string {
	if cfg != "" {
		return cfg
	}
	if v := os.Getenv(EnvOpenAIBaseURL); v != "" {
		return v
	}
	return "https://api.openai.com/v1"
}

// Merge builds Resolved from file defaults and CLI overrides.
func Merge(file File, provider, model, context, namespace string, approve bool, prompt string) Resolved {
	r := Resolved{
		Provider:  first(provider, file.Provider, "openai"),
		Model:     first(model, file.Model, defaultModel(first(provider, file.Provider, "openai"))),
		BaseURL:   first("", file.BaseURL),
		Context:   first(context, file.Context),
		Namespace: first(namespace, file.Namespace, "default"),
		Approve:   approve,
		Prompt:    prompt,
	}
	if r.BaseURL == "" {
		r.BaseURL = OpenAIBaseURL("")
	} else {
		r.BaseURL = OpenAIBaseURL(r.BaseURL)
	}
	return r
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func defaultModel(provider string) string {
	switch provider {
	case "anthropic":
		return "claude-sonnet-4-20250514"
	default:
		return "gpt-4o-mini"
	}
}
