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
	Provider          string            `yaml:"provider"`
	Model             string            `yaml:"model"`
	BaseURL           string            `yaml:"base_url,omitempty"`
	Context           string            `yaml:"context,omitempty"`
	Namespace         string            `yaml:"namespace,omitempty"`
	Theme             string            `yaml:"theme,omitempty"`
	Aliases           map[string]string `yaml:"aliases,omitempty"`
	RequireAliasMatch bool              `yaml:"require_alias_match,omitempty"`
	Tools             ToolsFile         `yaml:"tools,omitempty"`
	GitOps            GitOpsFile        `yaml:"gitops,omitempty"`
}

// GitOpsFile configures T-072 PR-mode apply (distinct from tools.gitops enable/disable).
type GitOpsFile struct {
	Mode       string `yaml:"mode,omitempty"`        // apply (default) | pr
	Repo       string `yaml:"repo,omitempty"`        // owner/name
	Path       string `yaml:"path,omitempty"`        // path prefix in the repo
	BaseBranch string `yaml:"base_branch,omitempty"` // default main
}

// ToolsFile holds integration endpoints and opt-outs (no secrets).
type ToolsFile struct {
	Helm          ToolToggle     `yaml:"helm,omitempty"`
	ArgoWorkflows ToolToggle     `yaml:"argo_workflows,omitempty"`
	Tekton        ToolToggle     `yaml:"tekton,omitempty"`
	KEDA          ToolToggle     `yaml:"keda,omitempty"`
	Istio         ToolToggle     `yaml:"istio,omitempty"`
	Crossplane    ToolToggle     `yaml:"crossplane,omitempty"`
	GitOps        ToolToggle     `yaml:"gitops,omitempty"`
	Prometheus    PrometheusTool `yaml:"prometheus,omitempty"`
	Grafana       GrafanaTool    `yaml:"grafana,omitempty"`
	OTel          OTelTool       `yaml:"otel,omitempty"`
}

// ToolToggle can disable a backend (enabled defaults to true).
type ToolToggle struct {
	Enabled *bool `yaml:"enabled,omitempty"`
}

// PrometheusTool configures Prometheus query integration.
type PrometheusTool struct {
	Enabled *bool  `yaml:"enabled,omitempty"`
	URL     string `yaml:"url,omitempty"`
}

// GrafanaTool configures Grafana integration.
type GrafanaTool struct {
	Enabled *bool  `yaml:"enabled,omitempty"`
	URL     string `yaml:"url,omitempty"`
}

// OTelTool configures trace backend endpoints.
type OTelTool struct {
	Enabled  *bool  `yaml:"enabled,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty"`
	Backend  string `yaml:"backend,omitempty"`
}

// Resolved is the effective runtime configuration.
type Resolved struct {
	Provider  string
	Model     string
	BaseURL   string
	Context   string
	Namespace string
	Theme     string
	Tools     ToolsFile
	GitOps    GitOpsFile
	Approve   bool
	Wait      bool
	Timeout   time.Duration // used with Wait; 0 means default (5m)
	Output    string        // "", "text", or "json"
	Prompt    string
	// GitOpsPR forces PR mode for this run (CLI --gitops), overriding file/env mode.
	GitOpsPR bool
	// GitOpsRepo overrides gitops.repo for this run.
	GitOpsRepo string
	// GitOpsPath overrides gitops.path for this run.
	GitOpsPath string
	// GitOpsBaseBranch overrides gitops.base_branch for this run.
	GitOpsBaseBranch string

	// Aliases maps short names (prod) → kubeconfig context names.
	Aliases map[string]string
	// RequireAliasMatch refuses mutating apply when active kube context ≠ resolved Context.
	RequireAliasMatch bool
	// ContextAlias is the alias key used to resolve Context, if any.
	ContextAlias string
	// Contexts is an explicit multi-context fan-out list (resolved kube context names).
	Contexts []string
	// ApproveEachContext allows non-interactive multi-context mutate (explicit; not plain --approve).
	ApproveEachContext bool
	// FanOutChild skips plan/header noise when executing one section of a fan-out.
	FanOutChild bool

	// Set when the corresponding CLI flag was explicitly passed.
	NamespaceFromCLI bool
	ContextFromCLI   bool
}

// JSONOutput reports whether machine-readable JSON should be emitted.
func (r Resolved) JSONOutput() bool {
	return strings.EqualFold(strings.TrimSpace(r.Output), "json")
}

// EffectiveGitOps returns PR-mode settings with CLI overrides applied.
func (r Resolved) EffectiveGitOps() GitOpsFile {
	g := r.GitOps
	if r.GitOpsPR {
		g.Mode = "pr"
	}
	if v := strings.TrimSpace(r.GitOpsRepo); v != "" {
		g.Repo = v
	}
	if v := strings.TrimSpace(r.GitOpsPath); v != "" {
		g.Path = v
	}
	if v := strings.TrimSpace(r.GitOpsBaseBranch); v != "" {
		g.BaseBranch = v
	}
	return g
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

// DefaultPath returns ~/.kprompt/config.yaml (or $KPROMPT_HOME/config.yaml).
func DefaultPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Dir returns the kprompt config directory (~/.kprompt or $KPROMPT_HOME).
func Dir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("KPROMPT_HOME")); v != "" {
		return filepath.Clean(v), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kprompt"), nil
}

// APIKeyFor returns the API key for a provider preset.
// Order: process env → Team-pulled ~/.kprompt/provider-secrets.yaml → empty
// (or "ollama" when AllowEmptyKey). Env always wins over pulled secrets (ADR-0005).
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
	if v := PulledAPIKey(provider); v != "" {
		return v
	}
	if preset.AllowEmptyKey {
		return "ollama"
	}
	return ""
}

// Merge builds Resolved from file defaults and CLI overrides.
// Empty provider (no CLI flag, no config) stays unconfigured — no silent openai default (OB-002).
func Merge(file File, provider, model, context, namespace string, approve bool, prompt string) Resolved {
	prov := first(provider, file.Provider)
	preset, ok := llm.LookupPreset(prov)
	defModel := ""
	if ok {
		defModel = preset.DefaultModel
	}

	r := Resolved{
		Provider:          strings.ToLower(prov),
		Model:             first(model, file.Model, defModel),
		BaseURL:           first(file.BaseURL, os.Getenv(EnvOpenAIBaseURL), preset.BaseURL),
		Context:           first(context, file.Context),
		Namespace:         first(namespace, file.Namespace, "default"),
		Theme:             strings.ToLower(strings.TrimSpace(file.Theme)),
		Tools:             file.Tools,
		GitOps:            file.GitOps,
		Aliases:           file.Aliases,
		RequireAliasMatch: file.RequireAliasMatch,
		Approve:           approve,
		Prompt:            prompt,
	}
	resolved, alias := ResolveContext(r.Context, r.Aliases)
	r.Context = resolved
	r.ContextAlias = alias
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
