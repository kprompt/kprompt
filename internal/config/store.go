package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kprompt/kprompt/internal/llm"
)

// SaveFile writes File to ~/.kprompt/config.yaml (creates directory).
func SaveFile(f File) error {
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// SetField updates a non-secret config field and persists the file.
func SetField(key, value string) (File, error) {
	f, err := LoadFile()
	if err != nil {
		return File{}, err
	}
	k := strings.ToLower(strings.TrimSpace(key))
	v := strings.TrimSpace(value)
	switch k {
	case "provider":
		if v == "" {
			return File{}, fmt.Errorf("provider cannot be empty")
		}
		if _, ok := llm.LookupPreset(v); !ok {
			return File{}, fmt.Errorf("unknown provider %q (supported: %s)", v, llm.SupportedNames())
		}
		f.Provider = strings.ToLower(v)
	case "model":
		if v == "" {
			return File{}, fmt.Errorf("model cannot be empty")
		}
		f.Model = v
	case "base_url", "base-url", "baseurl":
		f.BaseURL = v
	case "context":
		f.Context = v
	case "namespace", "ns":
		f.Namespace = v
	case "theme":
		f.Theme = strings.ToLower(v)
	case "require_alias_match", "require-alias-match":
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			f.RequireAliasMatch = true
		case "false", "0", "no", "off", "":
			f.RequireAliasMatch = false
		default:
			return File{}, fmt.Errorf("require_alias_match must be true or false")
		}
	case "tools.prometheus.url", "tools.prometheus_url":
		f.Tools.Prometheus.URL = v
	case "tools.grafana.url", "tools.grafana_url":
		f.Tools.Grafana.URL = v
	case "tools.otel.endpoint", "tools.otel_endpoint":
		f.Tools.OTel.Endpoint = v
	case "tools.otel.backend", "tools.otel_backend":
		switch strings.ToLower(v) {
		case "", "auto", "jaeger", "tempo":
			f.Tools.OTel.Backend = strings.ToLower(v)
		default:
			return File{}, fmt.Errorf("tools.otel.backend must be auto, jaeger, or tempo")
		}
	default:
		return File{}, fmt.Errorf("unknown config key %q (allowed: provider, model, base_url, context, namespace, theme, require_alias_match, tools.prometheus.url, tools.grafana.url, tools.otel.endpoint, tools.otel.backend)", key)
	}
	if err := SaveFile(f); err != nil {
		return File{}, err
	}
	return f, nil
}

// View is a redacted snapshot for `kprompt config`.
type View struct {
	Path              string
	Provider          string
	Model             string
	BaseURL           string
	Context           string
	Namespace         string
	Theme             string
	Aliases           []string
	RequireAliasMatch bool
	APIKey            string // "set" | "unset" | "optional" — never the secret
	EnvHints          []string
}

// BuildView loads file + env status without exposing secrets.
func BuildView() (View, error) {
	path, err := DefaultPath()
	if err != nil {
		return View{}, err
	}
	f, err := LoadFile()
	if err != nil {
		return View{}, err
	}
	r := Merge(f, "", "", "", "", false, "")
	v := View{
		Path:              path,
		Provider:          r.Provider,
		Model:             r.Model,
		BaseURL:           r.BaseURL,
		Context:           dash(r.Context),
		Namespace:         r.Namespace,
		Theme:             themeOrAuto(r.Theme),
		Aliases:           AliasLines(f.Aliases),
		RequireAliasMatch: f.RequireAliasMatch,
	}
	preset, ok := llm.LookupPreset(r.Provider)
	if ok {
		v.EnvHints = append([]string{}, preset.EnvKeys...)
		key := APIKeyFor(r.Provider)
		switch {
		case preset.AllowEmptyKey:
			if key != "" && key != "ollama" {
				v.APIKey = "set"
			} else {
				v.APIKey = "optional (local)"
			}
		case key != "":
			v.APIKey = "set"
		default:
			v.APIKey = "unset"
		}
	} else {
		v.APIKey = "unknown provider"
	}
	return v, nil
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(default kube context)"
	}
	return s
}

func themeOrAuto(s string) string {
	if strings.TrimSpace(s) == "" {
		return "auto"
	}
	return s
}

// FormatView renders a human-readable redacted config.
func FormatView(v View) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Config file: %s\n", v.Path)
	fmt.Fprintf(&b, "provider:    %s\n", v.Provider)
	fmt.Fprintf(&b, "model:       %s\n", v.Model)
	fmt.Fprintf(&b, "base_url:    %s\n", emptyDash(v.BaseURL))
	fmt.Fprintf(&b, "namespace:   %s\n", v.Namespace)
	fmt.Fprintf(&b, "theme:       %s\n", v.Theme)
	fmt.Fprintf(&b, "context:     %s\n", v.Context)
	fmt.Fprintf(&b, "require_alias_match: %v\n", v.RequireAliasMatch)
	if len(v.Aliases) == 0 {
		fmt.Fprintf(&b, "aliases:     (none — kprompt config alias set <name> <kube-context>)\n")
	} else {
		fmt.Fprintf(&b, "aliases:\n")
		for _, line := range v.Aliases {
			fmt.Fprintf(&b, "  %s\n", line)
		}
	}
	fmt.Fprintf(&b, "api_key:     %s", v.APIKey)
	if len(v.EnvHints) > 0 {
		fmt.Fprintf(&b, "  (env: %s)", strings.Join(v.EnvHints, " | "))
	}
	b.WriteByte('\n')
	b.WriteString("Secrets are never stored in the config file.\n")
	return b.String()
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
