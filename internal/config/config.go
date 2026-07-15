package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/kprompt/kprompt/internal/llm"
)

const (
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
	Wait      bool
	Timeout   time.Duration // used with Wait; 0 means default (5m)
	Prompt    string

	// Set when the corresponding CLI flag was explicitly passed.
	NamespaceFromCLI bool
	ContextFromCLI   bool
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

// APIKeyFor returns the env-sourced API key for a provider preset.
func APIKeyFor(provider string) string {
	preset, ok := llm.LookupPreset(provider)
	if !ok {
		return ""
	}
	for _, k := range preset.EnvKeys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	if preset.AllowEmptyKey {
		return "ollama"
	}
	return ""
}

// Merge builds Resolved from file defaults and CLI overrides.
func Merge(file File, provider, model, context, namespace string, approve bool, prompt string) Resolved {
	prov := first(provider, file.Provider, "openai")
	preset, _ := llm.LookupPreset(prov)
	defModel := "gpt-4o-mini"
	if preset.Name != "" {
		defModel = preset.DefaultModel
	}

	r := Resolved{
		Provider:  strings.ToLower(prov),
		Model:     first(model, file.Model, defModel),
		BaseURL:   first(file.BaseURL, os.Getenv(EnvOpenAIBaseURL), preset.BaseURL),
		Context:   first(context, file.Context),
		Namespace: first(namespace, file.Namespace, "default"),
		Approve:   approve,
		Prompt:    prompt,
	}
	return r
}

func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
